package mezon

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
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
	// eventQueueSize buffers server-pushed events between the read loop and the
	// dispatch goroutine. Big enough to absorb command bursts; if it ever fills
	// (a handler wedged far beyond the send timeout), the read loop applies
	// backpressure instead of dropping events.
	eventQueueSize = 1024
)

// Raw API-response frame layout, port of the PREFIX_RAW handling in
// web_socket_adapter_pb.ts. Instead of an Envelope, the server answers an
// ApiRequestEvent with one or more binary frames:
//
//	[0]    0xFF prefix
//	[1:3]  cid, big-endian uint16
//	[3:7]  big-endian uint32: high 16 bits = response code, low 16 bits = fin flag
//	[7:]   payload chunk
//
// Chunks for a cid are buffered until a frame with fin flag 0xFF arrives.
const (
	rawFramePrefix  = 0xff
	rawHeaderLength = 7
	rawCodeFin      = 0xff
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
	ClanID        string
	ChannelID     string
	ChannelType   int
	Mode          int
	IsPublic      bool
	MessageID     string
	TopicID       string
	HasAttachment bool
}

// socketResponse is what a cid-correlated wait receives: either an Envelope
// reply or a reassembled raw API response (port of the api_response message
// shape produced by web_socket_adapter_pb.ts).
type socketResponse struct {
	env         *rtapi.Envelope
	apiResponse bool
	code        uint32
	body        []byte
}

// DefaultSocket is a realtime connection to the Mezon server, port of
// DefaultSocket + the transport adapters in the TS SDK. It encodes/decodes
// rtapi.Envelope frames over a transportConn (abridged TCP by default,
// WebSocket opt-in) and correlates request/response pairs by cid.
type DefaultSocket struct {
	wsURL   string
	host    string
	port    string
	useSSL  bool
	Verbose bool

	// Transport selects the wire transport for the next Connect; empty means
	// TransportTCP. TLSInsecureSkipVerify disables certificate verification for
	// the TCP transport (dev gateways use self-signed certs).
	Transport             TransportKind
	TLSInsecureSkipVerify bool

	// TcpURL is the dedicated abridged-TCP endpoint from Session.tcp_url. When
	// set, the TCP transport dials it instead of deriving an address from the
	// websocket URL / host.
	TcpURL string

	conn    transportConn
	writeMu sync.Mutex

	cidMu   sync.Mutex
	cids    map[int32]chan *socketResponse
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
		cids:             make(map[int32]chan *socketResponse),
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

// tcpAddr resolves the TCP transport endpoint: the server-provided TcpURL
// (Session.tcp_url) first, then the same fields buildURL uses — session WsURL,
// host[:port] otherwise; port defaults to 443.
func (s *DefaultSocket) tcpAddr() (host, port string) {
	if addr := s.TcpURL; addr != "" {
		// Tolerate a scheme prefix (e.g. "tcp://host:port").
		if i := strings.Index(addr, "://"); i >= 0 {
			addr = addr[i+3:]
		}
		if h, p, err := net.SplitHostPort(addr); err == nil {
			return h, p
		}
		return addr, "443"
	}
	if s.wsURL != "" {
		if h, p, err := net.SplitHostPort(s.wsURL); err == nil {
			return h, p
		}
		return s.wsURL, "443"
	}
	if s.port != "" {
		return s.host, s.port
	}
	return s.host, "443"
}

func (s *DefaultSocket) logf(format string, args ...any) {
	if s.Verbose {
		fmt.Printf(format+"\n", args...)
	}
}

// dial establishes the wire connection for the configured transport.
func (s *DefaultSocket) dial(token string, createStatus bool) (transportConn, error) {
	if s.Transport == TransportWebSocket {
		dialer := websocket.Dialer{HandshakeTimeout: defaultConnectTimeout}
		conn, _, err := dialer.Dial(s.buildURL(token, createStatus), nil)
		if err != nil {
			return nil, err
		}
		return &wsConn{conn: conn, logf: s.logf}, nil
	}
	host, port := s.tcpAddr()
	return dialTCP(host, port, s.useSSL, s.TLSInsecureSkipVerify, token)
}

// Connect establishes the connection using the session token.
func (s *DefaultSocket) Connect(session *Session, createStatus bool) error {
	if s.IsOpen() {
		return nil
	}
	conn, err := s.dial(session.Token, createStatus)
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
	// Events are dispatched on their own goroutine, NOT on the read loop. A
	// handler that performs a send-and-wait (e.g. a bot replying from inside
	// OnChannelMessage) parks until the cid response arrives — if dispatch ran
	// inline in readLoop, nobody could read that response and every reply would
	// stall for the full send timeout (and starve heartbeat pongs into a fake
	// "heartbeat timeout" disconnect). The TS SDK gets this for free from the JS
	// event loop; here the buffered channel plays that role while preserving
	// event order.
	events := make(chan *rtapi.Envelope, eventQueueSize)
	go s.dispatchLoop(events, done)
	go s.readLoop(conn, events, done)
	go s.heartbeatLoop(conn, done)
	return nil
}

// dispatchLoop delivers server-pushed envelopes to handlers in arrival order,
// decoupled from the read loop so handlers may send-and-wait safely.
func (s *DefaultSocket) dispatchLoop(events <-chan *rtapi.Envelope, done chan struct{}) {
	for {
		select {
		case env := <-events:
			s.dispatch(env)
		case <-done:
			return
		}
	}
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
		_ = conn.close()
	}
}

func (s *DefaultSocket) generateCid() int32 {
	return atomic.AddInt32(&s.nextCid, 1) - 1
}

// send writes an envelope and waits for the cid-correlated Envelope response.
func (s *DefaultSocket) send(env *rtapi.Envelope, timeout time.Duration) (*rtapi.Envelope, error) {
	resp, err := s.sendResponse(env, timeout)
	if err != nil {
		return nil, err
	}
	if resp.apiResponse {
		return nil, fmt.Errorf("mezon: unexpected raw API response for cid %d", env.Cid)
	}
	return resp.env, nil
}

// sendApiRequest invokes a server API over the socket, port of
// socket.sendApiRequest. The request rides in an ApiRequestEvent envelope and
// the response comes back as raw 0xFF-prefixed frames carrying the encoded
// response proto.
func (s *DefaultSocket) sendApiRequest(apiName string, body []byte) ([]byte, error) {
	apiIndex, ok := apiIndexFromName(apiName)
	if !ok {
		return nil, fmt.Errorf("mezon: unknown API %q", apiName)
	}
	resp, err := s.sendResponse(&rtapi.Envelope{ApiRequestEvent: &rtapi.ApiRequestEvent{
		ApiIndex: apiIndex,
		ApiName:  apiName,
		Body:     body,
	}}, 0)
	if err != nil {
		return nil, err
	}
	if !resp.apiResponse {
		return nil, fmt.Errorf("mezon: expected raw API response for %s, got envelope", apiName)
	}
	if resp.code != 0 {
		return nil, fmt.Errorf("mezon: API %s failed with code %d", apiName, resp.code)
	}
	return resp.body, nil
}

// sendResponse writes an envelope and waits for the cid-correlated response,
// which is either an Envelope or a reassembled raw API response.
func (s *DefaultSocket) sendResponse(env *rtapi.Envelope, timeout time.Duration) (*socketResponse, error) {
	if !s.IsOpen() {
		return nil, ErrSocketClosed
	}
	cid := s.generateCid()
	env.Cid = cid
	ch := make(chan *socketResponse, 1)

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
	if env.Ping != nil && conn.nativePing() {
		// The TCP transport carries heartbeats as a dedicated 3-byte frame; the
		// pong comes back as framePong and resolves this cid.
		err = conn.writePing(cid)
	} else {
		err = conn.writeEnvelope(data)
	}
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
		if resp.env != nil && resp.env.Error != nil {
			return nil, fmt.Errorf("mezon socket error %d: %s", resp.env.Error.Code, resp.env.Error.Message)
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

func (s *DefaultSocket) readLoop(conn transportConn, events chan<- *rtapi.Envelope, done chan struct{}) {
	// Per-connection chunk buffers for raw API responses, keyed by cid, port of
	// the _streams map in the TS adapters (cleared there on close; here they die
	// with this goroutine).
	streams := make(map[int32][][]byte)
	for {
		f, err := conn.readFrame()
		if err != nil {
			s.markDisconnected(done, err.Error())
			return
		}
		switch f.kind {
		case frameAPIChunk:
			s.deliverAPIChunk(f, streams)
		case framePong:
			// TCP heartbeat reply: resolve the waiting ping sender with a
			// synthetic pong envelope.
			if ch, ok := s.takeRawCid(f.cid); ok {
				ch <- &socketResponse{env: &rtapi.Envelope{Cid: f.cid, Pong: &rtapi.Pong{}}}
			}
		default: // frameEnvelope
			env, err := unmarshalEnvelope(f)
			if err != nil {
				s.logf("mezon: failed to decode envelope: %v", err)
				continue
			}
			// cid-correlated responses unblock waiting senders and MUST be
			// handled here, never queued behind event handlers.
			if env.Cid != 0 {
				if ch, ok := s.takeCid(env.Cid); ok {
					ch <- &socketResponse{env: env}
				}
				continue
			}
			select {
			case events <- env:
			case <-done:
				return
			}
		}
	}
}

// unmarshalEnvelope decodes a frame's envelope. An abridged frame still
// carries its 0-3 zero padding bytes, indistinguishable from proto content
// that really ends in 0x00; a padded parse fails on the stray zeros, so
// decoding retries with one fewer trailing zero until it parses.
func unmarshalEnvelope(f *wireFrame) (*rtapi.Envelope, error) {
	data := f.payload
	env := &rtapi.Envelope{}
	err := proto.Unmarshal(data, env)
	if !f.padded {
		return env, err
	}
	for i := 0; err != nil && i < 3 && len(data) > 0 && data[len(data)-1] == 0x00; i++ {
		data = data[:len(data)-1]
		env = &rtapi.Envelope{}
		err = proto.Unmarshal(data, env)
	}
	return env, err
}

// deliverAPIChunk buffers a raw API-response chunk and, on the fin frame,
// delivers the reassembled body to the waiting sender, port of the PREFIX_RAW
// branch in the TS adapters plus the api_response branch of
// DefaultSocket.onmessage.
func (s *DefaultSocket) deliverAPIChunk(f *wireFrame, streams map[int32][][]byte) {
	if !f.fin {
		streams[f.cid] = append(streams[f.cid], f.payload)
		return
	}

	chunks := streams[f.cid]
	delete(streams, f.cid)
	if len(f.payload) > 0 {
		chunks = append(chunks, f.payload)
	}
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	body := make([]byte, 0, total)
	for _, c := range chunks {
		body = append(body, c...)
	}

	ch, ok := s.takeRawCid(f.cid)
	if !ok {
		s.logf("mezon: no pending request for API response cid %d", f.cid)
		return
	}
	ch <- &socketResponse{apiResponse: true, code: f.code, body: body}
}

// takeCid removes and returns the response channel registered for cid.
func (s *DefaultSocket) takeCid(cid int32) (chan *socketResponse, bool) {
	s.cidMu.Lock()
	defer s.cidMu.Unlock()
	ch, ok := s.cids[cid]
	if ok {
		delete(s.cids, cid)
	}
	return ch, ok
}

// takeRawCid is takeCid for raw API frames, which only carry the low 16 bits
// of the cid: when the exact key is missing (the counter has passed 65535) it
// falls back to the pending request whose truncated cid matches.
func (s *DefaultSocket) takeRawCid(cid int32) (chan *socketResponse, bool) {
	s.cidMu.Lock()
	defer s.cidMu.Unlock()
	if ch, ok := s.cids[cid]; ok {
		delete(s.cids, cid)
		return ch, true
	}
	for k, ch := range s.cids {
		if int32(uint16(k)) == cid {
			delete(s.cids, k)
			return ch, true
		}
	}
	return nil, false
}

// markDisconnected tears down connection state and fires OnDisconnect once.
// done identifies the caller's connection: when it is no longer the current
// one (a reconnect already replaced the socket), the call is a stale no-op —
// the Go equivalent of the connect-attempt-sequence guard added in mezon-js
// PR #1128.
func (s *DefaultSocket) markDisconnected(done chan struct{}, reason string) {
	s.writeMu.Lock()
	current, closeDone := s.done, s.closeDone
	s.writeMu.Unlock()
	if current != done {
		return
	}
	if !s.connected.Swap(false) {
		return
	}
	if closeDone != nil {
		closeDone()
	}
	if s.OnDisconnect != nil {
		s.OnDisconnect(reason)
	}
}

// heartbeatLoop pings the server for liveness. conn/done belong to the
// connection the loop was started for, so a stale loop can never tear down a
// socket that a reconnect has since installed (port of mezon-js PR #1128).
func (s *DefaultSocket) heartbeatLoop(conn transportConn, done chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-time.After(s.heartbeatTimeout):
			if !s.IsOpen() {
				return
			}
			_, err := s.send(&rtapi.Envelope{Ping: &rtapi.Ping{}}, s.heartbeatTimeout)
			if err == nil {
				continue
			}
			select {
			case <-done:
				// This connection was already torn down while the ping was in
				// flight; whatever replaced it has its own heartbeat.
				return
			default:
			}
			if s.OnHeartbeatTimeout != nil {
				s.OnHeartbeatTimeout()
			}
			// markDisconnected (not Close) so OnDisconnect fires and the client
			// can reconnect — Close would swallow it by flipping connected
			// first, leaving the socket dead forever. It runs before conn.close
			// so the readLoop (which unblocks on the close) finds connected
			// already false and cannot report a bogus reason; closing the
			// captured conn — never the live s.conn — means a socket installed
			// by a reconnecting OnDisconnect consumer is not at risk (the
			// hazard mezon-js PR #1128 fixed by closing before the callback).
			s.markDisconnected(done, "heartbeat timeout: "+err.Error())
			_ = conn.close()
			return
		}
	}
}
