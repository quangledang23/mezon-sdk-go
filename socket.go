package mezon

import (
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
	"google.golang.org/protobuf/proto"
)

const (
	defaultHeartbeatTimeout = 10 * time.Second
	defaultSendTimeout      = 10 * time.Second
	defaultConnectTimeout   = 30 * time.Second
)

// ReplyMessageData is the payload for writing/replying a chat message.
type ReplyMessageData struct {
	ClanID           string
	ChannelID        string
	ChannelType      int
	Mode             int
	IsPublic         bool
	Content          Content
	Mentions         []Mention
	Attachments      []Attachment
	References       []MessageRef
	AnonymousMessage bool
	MentionEveryone  bool
	Avatar           string
	Code             int
	TopicID          string
}

// EphemeralMessageData is the payload for an ephemeral message.
type EphemeralMessageData struct {
	ReceiverIDs      []string
	ClanID           string
	ChannelID        string
	ChannelType      int
	Mode             int
	IsPublic         bool
	Content          Content
	Mentions         []Mention
	Attachments      []Attachment
	References       []MessageRef
	AnonymousMessage bool
	MentionEveryone  bool
	Avatar           string
	Code             int
	TopicID          string
	MessageID        string
}

// UpdateMessageData is the payload for editing a message.
type UpdateMessageData struct {
	ClanID            string
	ChannelID         string
	ChannelType       int
	Mode              int
	IsPublic          bool
	MessageID         string
	Content           Content
	Mentions          []Mention
	Attachments       []Attachment
	CreateTimeSeconds uint32
	HideEditted       bool
	TopicID           string
	IsUpdateMsgTopic  bool
}

// ReactMessageData is the payload for reacting to a message.
type ReactMessageData struct {
	ID              string
	ClanID          string
	ChannelID       string
	ChannelType     int
	Mode            int
	IsPublic        bool
	MessageID       string
	EmojiID         string
	Emoji           string
	Count           int
	MessageSenderID string
	ActionDelete    bool
}

// RemoveMessageData is the payload for deleting a message.
type RemoveMessageData struct {
	ClanID      string
	ChannelID   string
	ChannelType int
	Mode        int
	IsPublic    bool
	MessageID   string
	TopicID     string
}

// DefaultSocket is a protobuf WebSocket connection to the Mezon server, port of
// DefaultSocket + WebSocketAdapterPb in the TS SDK. It encodes/decodes
// rtapi.Envelope frames over a binary WebSocket and correlates request/response
// pairs by cid.
type DefaultSocket struct {
	wsURL   string
	host    string
	port    string
	useSSL  bool
	Verbose bool

	conn    *websocket.Conn
	writeMu sync.Mutex

	cidMu   sync.Mutex
	cids    map[int32]chan *rtapi.Envelope
	nextCid int32

	heartbeatTimeout time.Duration
	sendTimeout      time.Duration

	connected atomic.Bool
	// done is closed when the current connection is torn down; closeDone closes
	// it at most once. Both are replaced (under writeMu) on every Connect so each
	// connection has its own teardown signal, and read under writeMu by
	// send/Close/markDisconnected.
	done      chan struct{}
	closeDone func()

	emit func(event string, payload any)

	// optional lifecycle callbacks
	OnDisconnect       func(reason string)
	OnError            func(err error)
	OnHeartbeatTimeout func()
}

// NewDefaultSocket creates a socket. wsURL is the host portion used to build the
// websocket URL (typically session.WsURL); when empty, host[:port] is used.
func NewDefaultSocket(wsURL, host, port string, useSSL bool, emit func(string, any)) *DefaultSocket {
	return &DefaultSocket{
		wsURL:            wsURL,
		host:             host,
		port:             port,
		useSSL:           useSSL,
		cids:             make(map[int32]chan *rtapi.Envelope),
		nextCid:          1,
		heartbeatTimeout: defaultHeartbeatTimeout,
		sendTimeout:      defaultSendTimeout,
		emit:             emit,
		done:             make(chan struct{}),
	}
}

// IsOpen reports whether the socket is connected.
func (s *DefaultSocket) IsOpen() bool { return s.connected.Load() }

func (s *DefaultSocket) buildURL(token string, createStatus bool) string {
	scheme := "ws"
	if s.useSSL {
		scheme = "wss"
	}
	wsHost := s.wsURL
	if wsHost == "" {
		wsHost = s.host
		if s.port != "" && s.port != "443" && s.port != "80" {
			wsHost = s.host + ":" + s.port
		}
	}
	q := url.Values{}
	q.Set("lang", "en")
	q.Set("status", strconv.FormatBool(createStatus))
	q.Set("token", token)
	q.Set("format", "protobuf")
	return fmt.Sprintf("%s://%s/ws?%s", scheme, wsHost, q.Encode())
}

// Connect establishes the websocket connection using the session token.
func (s *DefaultSocket) Connect(session *Session, createStatus bool) error {
	if s.IsOpen() {
		return nil
	}
	dialer := websocket.Dialer{HandshakeTimeout: defaultConnectTimeout}
	conn, _, err := dialer.Dial(s.buildURL(session.Token, createStatus), nil)
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		}
		return err
	}
	// Publish the new connection state under writeMu (read by send/Close/
	// markDisconnected) and hand the fresh conn/done to the loops as locals, so a
	// later reconnect that reassigns these fields cannot race the goroutines
	// started here.
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }
	s.writeMu.Lock()
	s.conn = conn
	s.done = done
	s.closeDone = closeDone
	s.writeMu.Unlock()
	s.connected.Store(true)
	go s.readLoop(conn)
	go s.heartbeatLoop(done)
	return nil
}

// Close shuts down the connection.
func (s *DefaultSocket) Close() {
	s.connected.Store(false)
	s.writeMu.Lock()
	closeDone, conn := s.closeDone, s.conn
	s.writeMu.Unlock()
	if closeDone != nil {
		closeDone()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

func (s *DefaultSocket) generateCid() int32 {
	return atomic.AddInt32(&s.nextCid, 1) - 1
}

// send writes an envelope and waits for the cid-correlated response.
func (s *DefaultSocket) send(env *rtapi.Envelope, timeout time.Duration) (*rtapi.Envelope, error) {
	if !s.IsOpen() {
		return nil, ErrSocketClosed
	}
	cid := s.generateCid()
	env.Cid = cid
	ch := make(chan *rtapi.Envelope, 1)

	s.cidMu.Lock()
	s.cids[cid] = ch
	s.cidMu.Unlock()

	data, err := proto.Marshal(env)
	if err != nil {
		s.clearCid(cid)
		return nil, err
	}

	s.writeMu.Lock()
	conn, done := s.conn, s.done
	err = conn.WriteMessage(websocket.BinaryMessage, data)
	s.writeMu.Unlock()
	if err != nil {
		s.clearCid(cid)
		return nil, err
	}

	if timeout <= 0 {
		timeout = s.sendTimeout
	}
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("mezon socket error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-time.After(timeout):
		s.clearCid(cid)
		return nil, ErrSendTimeout
	case <-done:
		return nil, ErrSocketClosed
	}
}

func (s *DefaultSocket) clearCid(cid int32) {
	s.cidMu.Lock()
	delete(s.cids, cid)
	s.cidMu.Unlock()
}

func (s *DefaultSocket) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			s.markDisconnected(err.Error())
			return
		}
		env := &rtapi.Envelope{}
		if err := proto.Unmarshal(data, env); err != nil {
			if s.Verbose {
				fmt.Println("mezon: failed to decode envelope:", err)
			}
			continue
		}
		if env.Cid != 0 {
			s.cidMu.Lock()
			ch, ok := s.cids[env.Cid]
			if ok {
				delete(s.cids, env.Cid)
			}
			s.cidMu.Unlock()
			if ok {
				ch <- env
			}
			continue
		}
		s.dispatch(env)
	}
}

func (s *DefaultSocket) markDisconnected(reason string) {
	if !s.connected.Swap(false) {
		return
	}
	s.writeMu.Lock()
	closeDone := s.closeDone
	s.writeMu.Unlock()
	if closeDone != nil {
		closeDone()
	}
	if s.OnDisconnect != nil {
		s.OnDisconnect(reason)
	}
}

func (s *DefaultSocket) heartbeatLoop(done chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-time.After(s.heartbeatTimeout):
			if !s.IsOpen() {
				return
			}
			if _, err := s.send(&rtapi.Envelope{Ping: &rtapi.Ping{}}, s.heartbeatTimeout); err != nil {
				if s.OnHeartbeatTimeout != nil {
					s.OnHeartbeatTimeout()
				}
				s.Close()
				return
			}
		}
	}
}
