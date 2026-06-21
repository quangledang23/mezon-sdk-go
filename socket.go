package mezonlightsdk

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

// Default socket timeouts, mirroring DefaultSocket in socket.gen.ts.
const (
	DefaultHeartbeatTimeout = 10 * time.Second
	DefaultSendTimeout      = 10 * time.Second
	DefaultConnectTimeout   = 30 * time.Second
)

// ConnectionState describes the socket connection lifecycle.
type ConnectionState int32

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
)

// ChatMessageOptions holds the optional parameters of WriteChatMessage.
type ChatMessageOptions struct {
	Mentions         []*proto.MessageMention
	Attachments      []*proto.MessageAttachment
	References       []*proto.MessageRef
	AnonymousMessage bool
	MentionEveryone  bool
	Avatar           string
	Code             int32
	TopicID          string
	ID               string
}

type envelopeResult struct {
	env *proto.Envelope
	err error
}

// DefaultSocket is a socket connection to the Mezon server speaking the
// protobuf realtime protocol, the Go counterpart of DefaultSocket +
// WebSocketAdapterPb in the TypeScript SDK.
//
// Callback fields must be set before Connect and not mutated afterwards.
type DefaultSocket struct {
	Host    string
	Port    string
	UseSSL  bool
	Verbose bool

	// SendTimeout bounds request/response exchanges such as JoinChat.
	SendTimeout time.Duration

	// OnReconnect is called when the socket connects again after having
	// connected at least once before.
	OnReconnect func()
	// OnDisconnect is called when the connection is lost or closed.
	OnDisconnect func()
	// OnError is called for transport or decode errors.
	OnError func(err error)
	// OnHeartbeatTimeout is called when the server stops answering
	// application-level pings.
	OnHeartbeatTimeout func()
	// OnChannelMessage receives incoming channel messages.
	OnChannelMessage func(message *ChannelMessage)

	mu                 sync.Mutex
	conn               *websocket.Conn
	state              ConnectionState
	done               chan struct{}
	pending            map[int32]chan envelopeResult
	nextCid            int32
	hasConnectedOnce   bool
	suppressDisconnect bool

	writeMu sync.Mutex

	heartbeatMu      sync.Mutex
	heartbeatTimeout time.Duration
}

// NewDefaultSocket creates a socket for the given host/port. The socket is
// not connected until Connect is called.
func NewDefaultSocket(host, port string, useSSL, verbose bool) *DefaultSocket {
	return &DefaultSocket{
		Host:             host,
		Port:             port,
		UseSSL:           useSSL,
		Verbose:          verbose,
		SendTimeout:      DefaultSendTimeout,
		heartbeatTimeout: DefaultHeartbeatTimeout,
	}
}

// IsOpen reports whether the connection is established.
func (s *DefaultSocket) IsOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == StateConnected
}

// SetHeartbeatTimeout sets the heartbeat interval/timeout used to detect a
// lost connection.
func (s *DefaultSocket) SetHeartbeatTimeout(d time.Duration) {
	s.heartbeatMu.Lock()
	defer s.heartbeatMu.Unlock()
	s.heartbeatTimeout = d
}

// HeartbeatTimeout returns the heartbeat interval/timeout.
func (s *DefaultSocket) HeartbeatTimeout() time.Duration {
	s.heartbeatMu.Lock()
	defer s.heartbeatMu.Unlock()
	return s.heartbeatTimeout
}

// Connect dials the realtime endpoint and starts the read and heartbeat
// loops. If the socket is already connected it returns immediately. If ctx
// carries no deadline, DefaultConnectTimeout applies to the handshake.
func (s *DefaultSocket) Connect(ctx context.Context, session *Session, createStatus bool, platform string) error {
	s.mu.Lock()
	if s.state == StateConnected {
		s.mu.Unlock()
		return nil
	}
	if s.state == StateConnecting {
		s.mu.Unlock()
		return &SocketError{Message: "socket connection already in progress"}
	}
	s.state = StateConnecting
	s.mu.Unlock()

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultConnectTimeout)
		defer cancel()
	}

	scheme := "ws"
	if s.UseSSL {
		scheme = "wss"
	}
	hostPort := s.Host
	if s.Port != "" {
		hostPort += ":" + s.Port
	}
	wsURL := scheme + "://" + hostPort + "/ws?lang=en&status=" + url.QueryEscape(strconv.FormatBool(createStatus)) +
		"&token=" + url.QueryEscape(session.Token) +
		"&format=protobuf&platform=" + url.QueryEscape(platform)

	dialer := *websocket.DefaultDialer
	conn, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		s.mu.Lock()
		s.state = StateDisconnected
		s.mu.Unlock()
		if s.OnError != nil {
			s.OnError(err)
		}
		return err
	}

	s.mu.Lock()
	s.conn = conn
	s.done = make(chan struct{})
	s.pending = make(map[int32]chan envelopeResult)
	s.state = StateConnected
	isReconnect := s.hasConnectedOnce
	s.hasConnectedOnce = true
	done := s.done
	s.mu.Unlock()

	go s.readLoop(conn)
	go s.heartbeatLoop(done)

	if isReconnect && s.OnReconnect != nil {
		s.OnReconnect()
	}
	return nil
}

// Disconnect closes the connection. When fireDisconnectEvent is true the
// OnDisconnect callback is invoked, matching the TypeScript SDK behavior.
func (s *DefaultSocket) Disconnect(fireDisconnectEvent bool) {
	s.mu.Lock()
	conn := s.conn
	if conn != nil {
		// The read loop will tear the connection down; keep it from firing
		// OnDisconnect a second time.
		s.suppressDisconnect = true
	}
	s.state = StateDisconnected
	s.mu.Unlock()

	if conn != nil {
		conn.Close()
	}
	if fireDisconnectEvent && s.OnDisconnect != nil {
		s.OnDisconnect()
	}
}

// readLoop receives envelopes until the connection drops, dispatching
// incoming channel messages and resolving request/response exchanges.
func (s *DefaultSocket) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			s.teardown()
			return
		}

		env := &proto.Envelope{}
		if uerr := env.Unmarshal(data); uerr != nil {
			if s.OnError != nil {
				s.OnError(uerr)
			}
			continue
		}
		if s.Verbose {
			raw, _ := json.Marshal(env)
			log.Printf("mezonlightsdk: response: %s", raw)
		}

		// Inbound message from the server (no cid).
		if env.Cid == 0 {
			if env.ChannelMessage != nil {
				if handler := s.OnChannelMessage; handler != nil {
					handler(newChannelMessageFromProto(env.ChannelMessage))
				}
			} else if s.Verbose {
				log.Printf("mezonlightsdk: unrecognized message received: %+v", env)
			}
			continue
		}

		s.mu.Lock()
		ch := s.pending[env.Cid]
		delete(s.pending, env.Cid)
		s.mu.Unlock()
		if ch == nil {
			if s.Verbose {
				log.Printf("mezonlightsdk: no pending request for cid %d", env.Cid)
			}
			continue
		}
		if env.Error != nil {
			ch <- envelopeResult{err: &SocketError{Code: env.Error.Code, Message: env.Error.Message, Context: env.Error.Context}}
		} else {
			ch <- envelopeResult{env: env}
		}
	}
}

// teardown finalizes a dropped connection: it stops the heartbeat loop,
// fails all pending requests, and fires OnDisconnect unless suppressed.
func (s *DefaultSocket) teardown() {
	s.mu.Lock()
	if s.conn == nil {
		s.mu.Unlock()
		return
	}
	s.conn = nil
	s.state = StateDisconnected
	if s.done != nil {
		close(s.done)
		s.done = nil
	}
	pending := s.pending
	s.pending = nil
	suppress := s.suppressDisconnect
	s.suppressDisconnect = false
	s.mu.Unlock()

	for _, ch := range pending {
		ch <- envelopeResult{err: &SocketError{Message: "socket connection closed"}}
	}
	if !suppress && s.OnDisconnect != nil {
		s.OnDisconnect()
	}
}

// heartbeatLoop sends application-level pings and closes the connection when
// the server stops answering.
func (s *DefaultSocket) heartbeatLoop(done chan struct{}) {
	for {
		timeout := s.HeartbeatTimeout()
		select {
		case <-done:
			return
		case <-time.After(timeout):
			if !s.IsOpen() {
				return
			}
			if _, err := s.send(context.Background(), &proto.Envelope{Ping: &proto.Ping{}}, timeout); err != nil {
				if !s.IsOpen() {
					return
				}
				if s.Verbose {
					log.Println("mezonlightsdk: server unreachable from heartbeat")
				}
				if s.OnHeartbeatTimeout != nil {
					s.OnHeartbeatTimeout()
				}
				s.mu.Lock()
				conn := s.conn
				s.mu.Unlock()
				if conn != nil {
					conn.Close()
				}
				return
			}
		}
	}
}

// send writes an envelope tagged with a fresh cid and waits for the matching
// response. A zero timeout waits indefinitely (bounded only by ctx).
func (s *DefaultSocket) send(ctx context.Context, env *proto.Envelope, timeout time.Duration) (*proto.Envelope, error) {
	s.mu.Lock()
	if s.state != StateConnected || s.conn == nil {
		s.mu.Unlock()
		return nil, &SocketError{Message: "socket connection has not been established yet"}
	}
	s.nextCid++
	cid := s.nextCid
	env.Cid = cid
	ch := make(chan envelopeResult, 1)
	s.pending[cid] = ch
	conn := s.conn
	done := s.done
	s.mu.Unlock()

	data := env.Marshal()
	s.writeMu.Lock()
	err := conn.WriteMessage(websocket.BinaryMessage, data)
	s.writeMu.Unlock()
	if err != nil {
		s.removePending(cid)
		return nil, err
	}

	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}

	select {
	case res := <-ch:
		return res.env, res.err
	case <-timer:
		s.removePending(cid)
		return nil, &SocketError{Message: "the socket timed out while waiting for a response"}
	case <-ctx.Done():
		s.removePending(cid)
		return nil, ctx.Err()
	case <-done:
		return nil, &SocketError{Message: "socket connection closed"}
	}
}

func (s *DefaultSocket) removePending(cid int32) {
	s.mu.Lock()
	delete(s.pending, cid)
	s.mu.Unlock()
}

// JoinClanChat joins clan-level realtime events on the server, which also
// registers the connection as an active clan member.
func (s *DefaultSocket) JoinClanChat(ctx context.Context, clanID string) error {
	_, err := s.send(ctx, &proto.Envelope{
		ClanJoin: &proto.ClanJoin{ClanID: clanID},
	}, s.SendTimeout)
	return err
}

// JoinChat joins a chat channel on the server.
func (s *DefaultSocket) JoinChat(ctx context.Context, clanID, channelID string, channelType int32, isPublic bool) (*proto.Channel, error) {
	res, err := s.send(ctx, &proto.Envelope{
		ChannelJoin: &proto.ChannelJoin{
			ClanID:      clanID,
			ChannelID:   channelID,
			ChannelType: channelType,
			IsPublic:    isPublic,
		},
	}, s.SendTimeout)
	if err != nil {
		return nil, err
	}
	return res.Channel, nil
}

// LeaveChat leaves a chat channel on the server.
func (s *DefaultSocket) LeaveChat(ctx context.Context, clanID, channelID string, channelType int32, isPublic bool) error {
	_, err := s.send(ctx, &proto.Envelope{
		ChannelLeave: &proto.ChannelLeave{
			ClanID:      clanID,
			ChannelID:   channelID,
			ChannelType: channelType,
			IsPublic:    isPublic,
		},
	}, s.SendTimeout)
	return err
}

// WriteChatMessage sends a chat message to a channel on the server. The
// content is JSON-encoded, matching the TypeScript SDK. Like the original,
// the wait for the acknowledgement is unbounded except by ctx.
func (s *DefaultSocket) WriteChatMessage(ctx context.Context, clanID, channelID string, mode int32, isPublic bool, content any, opts *ChatMessageOptions) (*proto.ChannelMessageAck, error) {
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &ChatMessageOptions{}
	}

	res, err := s.send(ctx, &proto.Envelope{
		ChannelMessageSend: &proto.ChannelMessageSend{
			ClanID:           clanID,
			ChannelID:        channelID,
			Mode:             mode,
			IsPublic:         isPublic,
			Content:          string(contentJSON),
			Mentions:         opts.Mentions,
			Attachments:      opts.Attachments,
			References:       opts.References,
			AnonymousMessage: opts.AnonymousMessage,
			MentionEveryone:  opts.MentionEveryone,
			Avatar:           opts.Avatar,
			Code:             opts.Code,
			TopicID:          opts.TopicID,
			ID:               opts.ID,
		},
	}, 0)
	if err != nil {
		return nil, err
	}
	return res.ChannelMessageAck, nil
}
