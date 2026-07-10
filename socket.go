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

func (s *DefaultSocket) readLoop(conn *websocket.Conn) {
	// Per-connection chunk buffers for raw API responses, keyed by cid, port of
	// the _streams map in web_socket_adapter_pb.ts (cleared there on close; here
	// they die with this goroutine).
	streams := make(map[int32][][]byte)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			s.markDisconnected(err.Error())
			return
		}
		if len(data) > 0 && data[0] == rawFramePrefix {
			s.handleRawFrame(data, streams)
			continue
		}
		env := &rtapi.Envelope{}
		if err := proto.Unmarshal(data, env); err != nil {
			if s.Verbose {
				fmt.Println("mezon: failed to decode envelope:", err)
			}
			continue
		}
		if env.Cid != 0 {
			if ch, ok := s.takeCid(env.Cid); ok {
				ch <- &socketResponse{env: env}
			}
			continue
		}
		s.dispatch(env)
	}
}

// handleRawFrame buffers a raw API-response chunk and, on the fin frame,
// delivers the reassembled body to the waiting sender, port of the PREFIX_RAW
// branch in web_socket_adapter_pb.ts plus the api_response branch of
// DefaultSocket.onmessage.
func (s *DefaultSocket) handleRawFrame(data []byte, streams map[int32][][]byte) {
	if len(data) < rawHeaderLength {
		if s.Verbose {
			fmt.Println("mezon: raw frame too small to contain headers")
		}
		return
	}
	cid := int32(uint16(data[1])<<8 | uint16(data[2]))
	code := uint32(data[3])<<24 | uint32(data[4])<<16 | uint32(data[5])<<8 | uint32(data[6])
	payload := data[rawHeaderLength:]

	responseCode := (code >> 16) & 0xffff
	finFlag := code & 0xffff
	if finFlag != rawCodeFin {
		streams[cid] = append(streams[cid], payload)
		return
	}

	chunks := streams[cid]
	delete(streams, cid)
	if len(payload) > 0 {
		chunks = append(chunks, payload)
	}
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	body := make([]byte, 0, total)
	for _, c := range chunks {
		body = append(body, c...)
	}

	ch, ok := s.takeRawCid(cid)
	if !ok {
		if s.Verbose {
			fmt.Printf("mezon: no pending request for API response cid %d\n", cid)
		}
		return
	}
	ch <- &socketResponse{apiResponse: true, code: responseCode, body: body}
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
