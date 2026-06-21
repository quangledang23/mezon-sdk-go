package mezonlightsdk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

// wsTestServer is a fake Mezon realtime server speaking the protobuf envelope
// protocol over websocket.
type wsTestServer struct {
	host    string
	port    string
	queries chan url.Values
}

// newWSTestServer starts a websocket server. For every envelope received,
// handle returns the envelopes to write back; a nil element closes the
// connection.
func newWSTestServer(t *testing.T, handle func(env *proto.Envelope) []*proto.Envelope) *wsTestServer {
	t.Helper()
	ws := &wsTestServer{queries: make(chan url.Values, 4)}
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}
		select {
		case ws.queries <- r.URL.Query():
		default:
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			env := &proto.Envelope{}
			if err := env.Unmarshal(data); err != nil {
				continue
			}
			if handle == nil {
				continue
			}
			for _, resp := range handle(env) {
				if resp == nil {
					return // defer closes the connection
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, resp.Marshal()); err != nil {
					return
				}
			}
		}
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ws.host = u.Hostname()
	ws.port = u.Port()
	return ws
}

// pongReply answers heartbeat pings; tests compose it into their handlers.
func pongReply(env *proto.Envelope) []*proto.Envelope {
	if env.Ping != nil {
		return []*proto.Envelope{{Cid: env.Cid, Pong: &proto.Pong{}}}
	}
	return nil
}

func connectSocket(t *testing.T, ws *wsTestServer) *DefaultSocket {
	t.Helper()
	s := NewDefaultSocket(ws.host, ws.port, false, false)
	if err := s.Connect(context.Background(), &Session{Token: "test-token"}, true, "0"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { s.Disconnect(false) })
	return s
}

func TestSocketConnectAndJoinChat(t *testing.T) {
	joins := make(chan *proto.ChannelJoin, 1)
	ws := newWSTestServer(t, func(env *proto.Envelope) []*proto.Envelope {
		if env.ChannelJoin != nil {
			joins <- env.ChannelJoin
			return []*proto.Envelope{{Cid: env.Cid, Channel: &proto.Channel{ID: "123", ChanelLabel: "general"}}}
		}
		if env.ChannelLeave != nil {
			return []*proto.Envelope{{Cid: env.Cid}}
		}
		return pongReply(env)
	})

	s := connectSocket(t, ws)
	if !s.IsOpen() {
		t.Fatal("IsOpen() = false after Connect")
	}

	// Connect must pass session and protocol parameters in the URL.
	query := <-ws.queries
	if query.Get("token") != "test-token" || query.Get("format") != "protobuf" || query.Get("status") != "true" {
		t.Errorf("connect query = %v", query)
	}

	ch, err := s.JoinChat(context.Background(), "555", "123", ChannelTypeDM, false)
	if err != nil {
		t.Fatalf("JoinChat: %v", err)
	}
	if ch == nil || ch.ID != "123" || ch.ChanelLabel != "general" {
		t.Errorf("JoinChat returned %+v", ch)
	}

	join := <-joins
	if join.ClanID != "555" || join.ChannelID != "123" || join.ChannelType != ChannelTypeDM {
		t.Errorf("server received join = %+v", join)
	}

	if err := s.LeaveChat(context.Background(), "555", "123", ChannelTypeDM, false); err != nil {
		t.Errorf("LeaveChat: %v", err)
	}
}

func TestSocketJoinClanChat(t *testing.T) {
	joins := make(chan *proto.ClanJoin, 1)
	ws := newWSTestServer(t, func(env *proto.Envelope) []*proto.Envelope {
		if env.ClanJoin != nil {
			joins <- env.ClanJoin
			return []*proto.Envelope{{Cid: env.Cid}}
		}
		return pongReply(env)
	})

	s := connectSocket(t, ws)
	if err := s.JoinClanChat(context.Background(), "555"); err != nil {
		t.Fatalf("JoinClanChat: %v", err)
	}
	if join := <-joins; join.ClanID != "555" {
		t.Errorf("server received clan join = %+v", join)
	}
}

func TestSocketWriteChatMessage(t *testing.T) {
	sends := make(chan *proto.ChannelMessageSend, 1)
	ws := newWSTestServer(t, func(env *proto.Envelope) []*proto.Envelope {
		if env.ChannelMessageSend != nil {
			sends <- env.ChannelMessageSend
			return []*proto.Envelope{{
				Cid:               env.Cid,
				ChannelMessageAck: &proto.ChannelMessageAck{ChannelID: env.ChannelMessageSend.ChannelID, MessageID: "789"},
			}}
		}
		return pongReply(env)
	})

	s := connectSocket(t, ws)
	ack, err := s.WriteChatMessage(context.Background(), "555", "123", StreamModeDM, false,
		map[string]string{"t": "hello"},
		&ChatMessageOptions{Code: 1, Attachments: []*proto.MessageAttachment{{Filename: "f.png"}}})
	if err != nil {
		t.Fatalf("WriteChatMessage: %v", err)
	}
	if ack == nil || ack.MessageID != "789" || ack.ChannelID != "123" {
		t.Errorf("ack = %+v", ack)
	}

	sent := <-sends
	if sent.Content != `{"t":"hello"}` {
		t.Errorf("Content = %q, want JSON-encoded payload", sent.Content)
	}
	if sent.Mode != StreamModeDM || sent.Code != 1 || len(sent.Attachments) != 1 {
		t.Errorf("send envelope = %+v", sent)
	}
}

func TestSocketWriteChatMessageUnencodableContent(t *testing.T) {
	ws := newWSTestServer(t, pongReply)
	s := connectSocket(t, ws)

	if _, err := s.WriteChatMessage(context.Background(), "1", "2", StreamModeDM, false, func() {}, nil); err == nil {
		t.Error("expected JSON encoding error for func content")
	}
}

func TestSocketServerErrorEnvelope(t *testing.T) {
	ws := newWSTestServer(t, func(env *proto.Envelope) []*proto.Envelope {
		if env.ChannelJoin != nil {
			return []*proto.Envelope{{
				Cid:   env.Cid,
				Error: &proto.Error{Code: 3, Message: "permission denied", Context: map[string]string{"channel": "123"}},
			}}
		}
		return pongReply(env)
	})

	s := connectSocket(t, ws)
	_, err := s.JoinChat(context.Background(), "555", "123", ChannelTypeDM, false)

	var sockErr *SocketError
	if !errors.As(err, &sockErr) {
		t.Fatalf("error = %v, want *SocketError", err)
	}
	if sockErr.Code != 3 || sockErr.Message != "permission denied" || sockErr.Context["channel"] != "123" {
		t.Errorf("SocketError = %+v", sockErr)
	}
}

func TestSocketSendTimeout(t *testing.T) {
	ws := newWSTestServer(t, pongReply) // never answers joins
	s := connectSocket(t, ws)
	s.SendTimeout = 50 * time.Millisecond

	_, err := s.JoinChat(context.Background(), "555", "123", ChannelTypeDM, false)
	var sockErr *SocketError
	if !errors.As(err, &sockErr) || !strings.Contains(sockErr.Message, "timed out") {
		t.Fatalf("error = %v, want socket timeout", err)
	}
}

func TestSocketSendContextCanceled(t *testing.T) {
	ws := newWSTestServer(t, pongReply) // never answers sends
	s := connectSocket(t, ws)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.WriteChatMessage(ctx, "1", "2", StreamModeDM, false, map[string]string{"t": "x"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestSocketSendBeforeConnect(t *testing.T) {
	s := NewDefaultSocket("localhost", "0", false, false)
	_, err := s.JoinChat(context.Background(), "1", "2", ChannelTypeDM, false)
	var sockErr *SocketError
	if !errors.As(err, &sockErr) {
		t.Fatalf("error = %v, want *SocketError", err)
	}
}

func TestSocketConnectTwice(t *testing.T) {
	ws := newWSTestServer(t, pongReply)
	s := connectSocket(t, ws)

	// A second Connect on an open socket is a no-op.
	if err := s.Connect(context.Background(), &Session{Token: "test-token"}, true, "0"); err != nil {
		t.Fatalf("second Connect: %v", err)
	}
}

func TestSocketConnectFailure(t *testing.T) {
	// Plain HTTP server: the websocket upgrade fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no websocket here", http.StatusBadRequest)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	var gotErr error
	s := NewDefaultSocket(u.Hostname(), u.Port(), false, false)
	s.OnError = func(err error) { gotErr = err }

	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err == nil {
		t.Fatal("Connect to non-websocket server expected error")
	}
	if gotErr == nil {
		t.Error("OnError callback not invoked")
	}
	if s.IsOpen() {
		t.Error("IsOpen() = true after failed connect")
	}

	// The socket must be reusable for another attempt after a failure.
	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err == nil {
		t.Fatal("second Connect expected error too")
	}
}

func TestSocketOnChannelMessage(t *testing.T) {
	attachments := &proto.MessageAttachmentList{
		Attachments: []*proto.MessageAttachment{{Filename: "pic.png", URL: "https://cdn.test/pic.png"}},
	}
	ws := newWSTestServer(t, func(env *proto.Envelope) []*proto.Envelope {
		if env.ChannelJoin != nil {
			return []*proto.Envelope{
				{Cid: env.Cid, Channel: &proto.Channel{ID: "123"}},
				// Unsolicited push (cid 0).
				{ChannelMessage: &proto.ChannelMessage{
					ChannelID:   "123",
					MessageID:   "456",
					SenderID:    "789",
					Content:     `{"t":"hi there"}`,
					Attachments: attachments.Marshal(),
				}},
			}
		}
		return pongReply(env)
	})

	received := make(chan *ChannelMessage, 1)
	s := NewDefaultSocket(ws.host, ws.port, false, false)
	s.OnChannelMessage = func(m *ChannelMessage) { received <- m }
	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer s.Disconnect(false)

	if _, err := s.JoinChat(context.Background(), "555", "123", ChannelTypeDM, false); err != nil {
		t.Fatalf("JoinChat: %v", err)
	}

	select {
	case m := <-received:
		if m.ChannelID != "123" || m.ID != "456" || m.SenderID != "789" {
			t.Errorf("message = %+v", m)
		}
		content, ok := m.Content.(map[string]any)
		if !ok || content["t"] != "hi there" {
			t.Errorf("Content = %#v, want decoded JSON", m.Content)
		}
		if len(m.Attachments) != 1 || m.Attachments[0].Filename != "pic.png" {
			t.Errorf("Attachments = %+v, want decoded protobuf list", m.Attachments)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnChannelMessage")
	}
}

func TestSocketDisconnectFiresOnce(t *testing.T) {
	ws := newWSTestServer(t, pongReply)

	events := make(chan struct{}, 4)
	s := NewDefaultSocket(ws.host, ws.port, false, false)
	s.OnDisconnect = func() { events <- struct{}{} }
	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	s.Disconnect(true)

	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("OnDisconnect not fired")
	}
	// The read-loop teardown must not fire OnDisconnect a second time.
	select {
	case <-events:
		t.Fatal("OnDisconnect fired twice")
	case <-time.After(100 * time.Millisecond):
	}
	if s.IsOpen() {
		t.Error("IsOpen() = true after Disconnect")
	}
}

func TestSocketServerClosesConnection(t *testing.T) {
	ws := newWSTestServer(t, func(env *proto.Envelope) []*proto.Envelope {
		if env.ChannelJoin != nil {
			return []*proto.Envelope{nil} // close without answering
		}
		return pongReply(env)
	})

	disconnected := make(chan struct{})
	s := NewDefaultSocket(ws.host, ws.port, false, false)
	s.OnDisconnect = func() { close(disconnected) }
	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// The pending request must fail when the connection drops.
	_, err := s.JoinChat(context.Background(), "555", "123", ChannelTypeDM, false)
	var sockErr *SocketError
	if !errors.As(err, &sockErr) {
		t.Fatalf("error = %v, want *SocketError", err)
	}

	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("OnDisconnect not fired after server closed the connection")
	}
}

func TestSocketReconnectCallback(t *testing.T) {
	ws := newWSTestServer(t, pongReply)

	reconnected := make(chan struct{}, 1)
	s := NewDefaultSocket(ws.host, ws.port, false, false)
	s.OnReconnect = func() { reconnected <- struct{}{} }

	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	select {
	case <-reconnected:
		t.Fatal("OnReconnect fired on the first connect")
	default:
	}

	s.Disconnect(false)
	// Give the read loop time to finish tearing down the old connection
	// before dialing again.
	time.Sleep(50 * time.Millisecond)
	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	defer s.Disconnect(false)

	select {
	case <-reconnected:
	case <-time.After(time.Second):
		t.Fatal("OnReconnect not fired on the second connect")
	}
}

func TestSocketHeartbeat(t *testing.T) {
	ws := newWSTestServer(t, pongReply)

	// The heartbeat loop reads the timeout at the start of each cycle, so it
	// must be configured before Connect for the test to exercise it.
	s := NewDefaultSocket(ws.host, ws.port, false, false)
	s.SetHeartbeatTimeout(30 * time.Millisecond)
	if s.HeartbeatTimeout() != 30*time.Millisecond {
		t.Fatalf("HeartbeatTimeout() = %v", s.HeartbeatTimeout())
	}
	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer s.Disconnect(false)

	// With the server answering pings the connection stays up across several
	// heartbeat intervals.
	time.Sleep(150 * time.Millisecond)
	if !s.IsOpen() {
		t.Error("connection dropped despite answered heartbeats")
	}
}

func TestSocketHeartbeatTimeout(t *testing.T) {
	ws := newWSTestServer(t, nil) // never answers pings

	timedOut := make(chan struct{})
	s := NewDefaultSocket(ws.host, ws.port, false, false)
	s.OnHeartbeatTimeout = func() { close(timedOut) }
	s.SetHeartbeatTimeout(30 * time.Millisecond)
	if err := s.Connect(context.Background(), &Session{Token: "t"}, true, "0"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer s.Disconnect(false)

	select {
	case <-timedOut:
	case <-time.After(2 * time.Second):
		t.Fatal("OnHeartbeatTimeout not fired")
	}

	// The heartbeat loop closes the connection after the timeout.
	deadline := time.Now().Add(time.Second)
	for s.IsOpen() {
		if time.Now().After(deadline) {
			t.Fatal("connection still open after heartbeat timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
