package mezon

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

// rawFrame builds a 0xFF-prefixed API response frame as produced by the
// server: cid uint16 BE, then code uint32 BE (high 16 bits response code, low
// 16 bits fin flag), then payload.
func rawFrame(cid uint16, responseCode, finFlag uint16, payload []byte) []byte {
	frame := make([]byte, rawHeaderLength, rawHeaderLength+len(payload))
	frame[0] = rawFramePrefix
	frame[1] = byte(cid >> 8)
	frame[2] = byte(cid)
	frame[3] = byte(responseCode >> 8)
	frame[4] = byte(responseCode)
	frame[5] = byte(finFlag >> 8)
	frame[6] = byte(finFlag)
	return append(frame, payload...)
}

func newTestSocket() *DefaultSocket {
	return NewDefaultSocket("", "example.com", "", true, func(string, any) {})
}

// wsRawFrame parses data as a WebSocket raw API frame and feeds it to the
// socket's chunk delivery, the path readLoop takes for 0xFF messages.
func (s *DefaultSocket) wsRawFrame(t *testing.T, data []byte, streams map[int32][][]byte) {
	t.Helper()
	f, ok := parseWSRawFrame(data)
	if !ok {
		t.Fatal("frame did not parse")
	}
	s.deliverAPIChunk(f, streams)
}

func TestHandleRawFrameSingle(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	s.cids[7] = ch
	streams := make(map[int32][][]byte)

	s.wsRawFrame(t, rawFrame(7, 0, rawCodeFin, []byte("hello")), streams)

	select {
	case resp := <-ch:
		if !resp.apiResponse {
			t.Fatal("expected apiResponse")
		}
		if resp.code != 0 {
			t.Fatalf("code = %d, want 0", resp.code)
		}
		if !bytes.Equal(resp.body, []byte("hello")) {
			t.Fatalf("body = %q, want %q", resp.body, "hello")
		}
	default:
		t.Fatal("no response delivered")
	}
	if len(streams) != 0 {
		t.Fatal("stream buffer not cleaned up")
	}
}

func TestHandleRawFrameChunked(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	s.cids[9] = ch
	streams := make(map[int32][][]byte)

	s.wsRawFrame(t, rawFrame(9, 0, 0, []byte("foo")), streams)
	s.wsRawFrame(t, rawFrame(9, 0, 0, []byte("bar")), streams)
	if len(streams[9]) != 2 {
		t.Fatalf("buffered chunks = %d, want 2", len(streams[9]))
	}
	s.wsRawFrame(t, rawFrame(9, 0, rawCodeFin, []byte("baz")), streams)

	resp := <-ch
	if !bytes.Equal(resp.body, []byte("foobarbaz")) {
		t.Fatalf("body = %q, want %q", resp.body, "foobarbaz")
	}
	if len(streams) != 0 {
		t.Fatal("stream buffer not cleaned up")
	}
}

func TestHandleRawFrameErrorCode(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	s.cids[3] = ch
	streams := make(map[int32][][]byte)

	s.wsRawFrame(t, rawFrame(3, 13, rawCodeFin, nil), streams)

	resp := <-ch
	if resp.code != 13 {
		t.Fatalf("code = %d, want 13", resp.code)
	}
}

func TestHandleRawFrameTooShort(t *testing.T) {
	// A frame shorter than the header must be rejected by the parser.
	if _, ok := parseWSRawFrame([]byte{rawFramePrefix, 0, 1}); ok {
		t.Fatal("short frame should be rejected")
	}
}

// A server that accepts the websocket but never answers pings must surface a
// heartbeat timeout as OnDisconnect so the client can reconnect; Close()ing
// silently would leave the socket dead forever.
func TestHeartbeatTimeoutFiresOnDisconnect(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	s := NewDefaultSocket("", u.Hostname(), u.Port(), false, func(string, any) {})
	s.Transport = TransportWebSocket
	s.heartbeatTimeout = 50 * time.Millisecond

	disconnected := make(chan string, 1)
	s.OnDisconnect = func(reason string) { disconnected <- reason }

	if err := s.Connect(&Session{Token: "test"}, false); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	select {
	case reason := <-disconnected:
		if !strings.Contains(reason, "heartbeat timeout") {
			t.Fatalf("reason = %q, want heartbeat timeout", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnDisconnect never fired after heartbeat timeout")
	}
	if s.IsOpen() {
		t.Fatal("socket should be marked disconnected")
	}
}

// A handler that performs a send-and-wait from inside an event callback (the
// normal shape of a bot replying in OnChannelMessage) must complete in ~one
// round trip. If events were dispatched inline on the read loop, the cid
// response could never be read: every reply would burn the full send timeout
// and heartbeat pongs would starve into a fake disconnect.
func TestDispatchDoesNotBlockReadLoop(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		// Push one server event to trigger the client's handler…
		env := &rtapi.Envelope{ChannelMessage: &api.ChannelMessage{ChannelId: 1}}
		data, _ := proto.Marshal(env)
		_ = c.WriteMessage(websocket.BinaryMessage, data)
		// …then echo back any cid-correlated request (the handler's send).
		for {
			_, in, err := c.ReadMessage()
			if err != nil {
				return
			}
			req := &rtapi.Envelope{}
			if proto.Unmarshal(in, req) == nil && req.Cid != 0 {
				out, _ := proto.Marshal(&rtapi.Envelope{Cid: req.Cid})
				_ = c.WriteMessage(websocket.BinaryMessage, out)
			}
		}
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	handlerDone := make(chan error, 1)
	var s *DefaultSocket
	s = NewDefaultSocket("", u.Hostname(), u.Port(), false, func(event string, _ any) {
		if event != EventChannelMessage {
			return
		}
		_, err := s.send(&rtapi.Envelope{Ping: &rtapi.Ping{}}, 2*time.Second)
		handlerDone <- err
	})
	s.Transport = TransportWebSocket
	s.heartbeatTimeout = time.Minute // keep the heartbeat out of this test
	if err := s.Connect(&Session{Token: "test"}, false); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer s.Close()

	select {
	case err := <-handlerDone:
		if err != nil {
			t.Fatalf("send from inside a handler: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("handler still blocked — events are being dispatched on the read loop")
	}
}

func TestTakeRawCidTruncatedFallback(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	// cid counter past 65535: raw frames only carry the low 16 bits.
	s.cids[65536+42] = ch

	got, ok := s.takeRawCid(42)
	if !ok || got != ch {
		t.Fatal("expected truncated-cid fallback to find the pending request")
	}
	if len(s.cids) != 0 {
		t.Fatal("pending cid not removed")
	}
}

func TestTcpAddrPrefersTcpURL(t *testing.T) {
	cases := []struct {
		tcpURL, wsURL, host, port string
		wantHost, wantPort        string
	}{
		{"sock.mezon.ai:8443", "gw.mezon.ai:443", "example.com", "80", "sock.mezon.ai", "8443"},
		{"tcp://sock.mezon.ai:8443", "", "example.com", "", "sock.mezon.ai", "8443"},
		{"sock.mezon.ai", "", "example.com", "", "sock.mezon.ai", "443"},
		{"", "gw.mezon.ai:9000", "example.com", "", "gw.mezon.ai", "9000"},
		{"", "", "example.com", "8080", "example.com", "8080"},
		{"", "", "example.com", "", "example.com", "443"},
	}
	for _, c := range cases {
		s := NewDefaultSocket(c.wsURL, c.host, c.port, true, func(string, any) {})
		s.TcpURL = c.tcpURL
		h, p := s.tcpAddr()
		if h != c.wantHost || p != c.wantPort {
			t.Errorf("tcpAddr(tcp=%q ws=%q host=%q port=%q) = %q:%q, want %q:%q",
				c.tcpURL, c.wsURL, c.host, c.port, h, p, c.wantHost, c.wantPort)
		}
	}
}
