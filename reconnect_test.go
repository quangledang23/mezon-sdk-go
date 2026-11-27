package mezon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

// testJWT builds an unsigned JWT whose payload carries exp; tag makes tokens
// with the same exp distinct.
func testJWT(t *testing.T, exp time.Time, tag string) string {
	t.Helper()
	claims, err := json.Marshal(map[string]any{"exp": exp.Unix(), "tag": tag})
	if err != nil {
		t.Fatal(err)
	}
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	return fmt.Sprintf("%s.%s.sig", hdr, base64.RawURLEncoding.EncodeToString(claims))
}

// fakeNode is a Mezon gateway/node stand-in for the WebSocket transport: it
// authenticates, serves the realtime socket (answering API requests and
// cid'd envelopes) and optionally the healthy-endpoint lookup. Hooks are set
// before the server starts and must be safe for concurrent use.
type fakeNode struct {
	srv *httptest.Server

	// session returns the session issued by the n-th authenticate call.
	session func(n int32) *api.Session
	// authDown makes the n-th authenticate call fail with a 503.
	authDown func(n int32) bool
	// rejectWS refuses the socket upgrade for a token.
	rejectWS func(token string) bool
	// apiCode is the response code for an API request over the socket.
	apiCode func(token, apiName string) uint16
	// healthy answers GET /v2/healthy/endpoint; nil => 404.
	healthy func(w http.ResponseWriter, r *http.Request)

	authCount   atomic.Int32
	healthyAuth atomic.Value // Authorization header of the last healthy lookup
	joined      chan string  // token of each connection that joined clan "0"

	mu    sync.Mutex
	conns []*websocket.Conn
}

func newFakeNode(t *testing.T) *fakeNode {
	t.Helper()
	n := &fakeNode{joined: make(chan string, 16)}
	n.srv = httptest.NewServer(n)
	t.Cleanup(n.srv.Close)
	return n
}

func (n *fakeNode) host() string {
	u, _ := url.Parse(n.srv.URL)
	return u.Host
}

// dropConns closes every open socket from the server side.
func (n *fakeNode) dropConns() {
	n.mu.Lock()
	conns := n.conns
	n.conns = nil
	n.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

func (n *fakeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v2/apps/authenticate/token":
		count := n.authCount.Add(1)
		if n.authDown != nil && n.authDown(count) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		out, _ := proto.Marshal(n.session(count))
		_, _ = w.Write(out)
	case r.URL.Path == "/v2/healthy/endpoint":
		n.healthyAuth.Store(r.Header.Get("Authorization"))
		if n.healthy == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		n.healthy(w, r)
	case r.URL.Path == "/ws":
		n.serveSocket(w, r)
	case strings.HasPrefix(r.URL.Path, "/mezon.api.Mezon/"):
		// HTTP fallback while the socket is down: an empty proto body.
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (n *fakeNode) serveSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if n.rejectWS != nil && n.rejectWS(token) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	n.mu.Lock()
	n.conns = append(n.conns, conn)
	n.mu.Unlock()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		env := &rtapi.Envelope{}
		if proto.Unmarshal(data, env) != nil {
			continue
		}
		if ev := env.ApiRequestEvent; ev != nil {
			code := uint16(0)
			if n.apiCode != nil {
				code = n.apiCode(token, ev.ApiName)
			}
			_ = conn.WriteMessage(websocket.BinaryMessage, rawFrame(uint16(env.Cid), code, rawCodeFin, nil))
			continue
		}
		if env.Cid == 0 {
			continue
		}
		ack, _ := proto.Marshal(&rtapi.Envelope{Cid: env.Cid})
		_ = conn.WriteMessage(websocket.BinaryMessage, ack)
		if env.ClanJoin != nil && env.ClanJoin.ClanId == 0 {
			select {
			case n.joined <- token:
			default:
			}
		}
	}
}

// waitJoined waits for the next connection to finish its init (the DM clan
// join is the last step of postConnectInit) and returns its token.
func (n *fakeNode) waitJoined(t *testing.T) string {
	t.Helper()
	select {
	case token := <-n.joined:
		return token
	case <-time.After(15 * time.Second):
		t.Fatalf("no connection finished init (authCount=%d)", n.authCount.Load())
		return ""
	}
}

// waitReconnected waits for the client's reconnect loop to finish with the
// socket up, so assertions do not race the tail of the post-connect init.
func waitReconnected(t *testing.T, client *MezonClient) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		client.mu.Lock()
		reconnecting := client.reconnecting
		client.mu.Unlock()
		if !reconnecting && client.socket.IsOpen() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("reconnect loop did not settle with an open socket")
}

// loginTestClient logs a WebSocket-transport client in against gateway with
// short reconnect delays.
func loginTestClient(t *testing.T, gateway *fakeNode) *MezonClient {
	t.Helper()
	origInit, origMax := reconnectInitialDelay, reconnectMaxDelay
	reconnectInitialDelay, reconnectMaxDelay = 10*time.Millisecond, 40*time.Millisecond
	t.Cleanup(func() { reconnectInitialDelay, reconnectMaxDelay = origInit, origMax })

	u, err := url.Parse(gateway.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	useSSL := false
	client, err := NewMezonClient(ClientConfig{
		BotID:     "12345",
		Token:     "bot-secret",
		Host:      u.Hostname(),
		Port:      u.Port(),
		UseSSL:    &useSSL,
		Transport: TransportWebSocket,
	})
	if err != nil {
		t.Fatalf("NewMezonClient: %v", err)
	}
	if err := client.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}
	t.Cleanup(client.Close)
	gateway.waitJoined(t)
	return client
}

// A bot that reconnects after its session JWT expired must authenticate again
// instead of retrying forever with the dead token.
func TestReconnectReauthenticatesExpiredSession(t *testing.T) {
	tokenV1 := testJWT(t, time.Now().Add(-time.Hour), "v1")
	tokenV2 := testJWT(t, time.Now().Add(time.Hour), "v2")
	var expired atomic.Bool

	node := newFakeNode(t)
	node.session = func(n int32) *api.Session {
		token := tokenV1
		if n >= 2 {
			token = tokenV2
		}
		return &api.Session{Token: token, UserId: 12345, ApiUrl: node.srv.URL, WsUrl: node.host()}
	}
	node.rejectWS = func(token string) bool { return expired.Load() && token != tokenV2 }

	client := loginTestClient(t, node)
	if got := node.authCount.Load(); got != 1 {
		t.Fatalf("authenticate calls after login = %d, want 1", got)
	}

	expired.Store(true)
	node.dropConns()

	if token := node.waitJoined(t); token != tokenV2 {
		t.Fatalf("reconnected with the stale token")
	}
	waitReconnected(t, client)
	if got := node.authCount.Load(); got != 2 {
		t.Errorf("authenticate calls after reconnect = %d, want 2", got)
	}
	if got := client.SessionToken(); got != tokenV2 {
		t.Errorf("session token not renewed")
	}
	if clan, ok := client.Clans.Get("0"); !ok || clan.SessionToken != tokenV2 {
		t.Errorf("clan session token not renewed")
	}
	if client.IsSessionExpired(sessionExpiryBuffer) {
		t.Errorf("renewed session still reported expired")
	}
}

// When the node the session points at stops accepting connections, the client
// asks the gateway for a healthy node and moves both the socket and the REST
// client there.
func TestReconnectMovesToHealthyEndpoint(t *testing.T) {
	token := testJWT(t, time.Now().Add(time.Hour), "v1")
	var nodeADown atomic.Bool

	nodeB := newFakeNode(t)
	gateway := newFakeNode(t)
	gateway.session = func(int32) *api.Session {
		return &api.Session{Token: token, UserId: 12345, ApiUrl: gateway.srv.URL, WsUrl: gateway.host(), SessionId: "sess-1"}
	}
	gateway.rejectWS = func(string) bool { return nodeADown.Load() }
	gateway.healthy = func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_url":    nodeB.srv.URL,
			"ws_url":     nodeB.host(),
			"session_id": "sess-2",
			"user_id":    12345,
		})
	}

	client := loginTestClient(t, gateway)

	nodeADown.Store(true)
	gateway.dropConns()

	if got := nodeB.waitJoined(t); got != token {
		t.Fatalf("healthy node saw another token")
	}
	waitReconnected(t, client)
	if got := gateway.authCount.Load(); got != 2 {
		t.Errorf("authenticate calls = %d, want 2 (login, then the reconnect)", got)
	}
	if got, _ := gateway.healthyAuth.Load().(string); got != "Bearer sess-1" {
		t.Errorf("healthy lookup Authorization = %q, want the session id", got)
	}
	if got := client.Api().basePath(); got != nodeB.srv.URL {
		t.Errorf("REST base path = %q, want %q", got, nodeB.srv.URL)
	}
	if got := client.currentSession().SessionID; got != "sess-2" {
		t.Errorf("session id = %q, want sess-2", got)
	}
}

// The reconnect could not renew the session (authenticate was down for one
// call) and fell back to the current one, whose JWT has not expired. A gateway
// that answers the healthy-endpoint lookup with 401 no longer accepts it:
// renew and redial.
func TestReconnectRenewsSessionRejectedByGateway(t *testing.T) {
	tokenV1 := testJWT(t, time.Now().Add(time.Hour), "v1")
	tokenV2 := testJWT(t, time.Now().Add(time.Hour), "v2")
	var revoked atomic.Bool

	node := newFakeNode(t)
	node.authDown = func(n int32) bool { return n == 2 }
	node.session = func(n int32) *api.Session {
		token := tokenV1
		if n >= 2 {
			token = tokenV2
		}
		return &api.Session{Token: token, UserId: 12345, ApiUrl: node.srv.URL, WsUrl: node.host()}
	}
	node.rejectWS = func(token string) bool { return revoked.Load() && token != tokenV2 }
	node.healthy = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }

	client := loginTestClient(t, node)

	revoked.Store(true)
	node.dropConns()

	if token := node.waitJoined(t); token != tokenV2 {
		t.Fatalf("reconnected with the revoked token")
	}
	waitReconnected(t, client)
	if got, _ := node.healthyAuth.Load().(string); got != "Bearer "+tokenV1 {
		t.Errorf("healthy lookup must fall back to the session token when there is no session id")
	}
	if got := node.authCount.Load(); got != 3 {
		t.Errorf("authenticate calls = %d, want 3 (login, the failed one, the renewal)", got)
	}
	if got := client.SessionToken(); got != tokenV2 {
		t.Errorf("session token not renewed")
	}
}

// Same fallback to the current session, but the node accepts the connection
// and fails the post-connect init instead: the connection is dropped and the
// next attempt comes with a fresh session.
func TestReconnectRenewsSessionAfterInitFailure(t *testing.T) {
	tokenV1 := testJWT(t, time.Now().Add(time.Hour), "v1")
	tokenV2 := testJWT(t, time.Now().Add(time.Hour), "v2")
	var revoked atomic.Bool

	node := newFakeNode(t)
	node.authDown = func(n int32) bool { return n == 2 }
	node.session = func(n int32) *api.Session {
		token := tokenV1
		if n >= 2 {
			token = tokenV2
		}
		return &api.Session{Token: token, UserId: 12345, ApiUrl: node.srv.URL, WsUrl: node.host()}
	}
	node.apiCode = func(token, _ string) uint16 {
		if revoked.Load() && token != tokenV2 {
			return 16 // unauthenticated
		}
		return 0
	}

	client := loginTestClient(t, node)

	revoked.Store(true)
	node.dropConns()

	if token := node.waitJoined(t); token != tokenV2 {
		t.Fatalf("init completed on the revoked token")
	}
	waitReconnected(t, client)
	if got := node.authCount.Load(); got != 3 {
		t.Errorf("authenticate calls = %d, want 3 (login, the failed one, the renewal)", got)
	}
	if got := client.SessionToken(); got != tokenV2 {
		t.Errorf("session token not renewed")
	}
}

// A session the server revoked while its JWT still looks valid, on a gateway
// with no healthy-endpoint lookup to say so: the reconnect must not keep
// dialing with it.
func TestReconnectReauthenticatesRevokedSession(t *testing.T) {
	tokenV1 := testJWT(t, time.Now().Add(time.Hour), "v1")
	tokenV2 := testJWT(t, time.Now().Add(time.Hour), "v2")
	var revoked atomic.Bool

	node := newFakeNode(t)
	node.session = func(n int32) *api.Session {
		token := tokenV1
		if n >= 2 {
			token = tokenV2
		}
		return &api.Session{Token: token, UserId: 12345, ApiUrl: node.srv.URL, WsUrl: node.host()}
	}
	node.rejectWS = func(token string) bool { return revoked.Load() && token != tokenV2 }

	client := loginTestClient(t, node)

	revoked.Store(true)
	node.dropConns()

	if token := node.waitJoined(t); token != tokenV2 {
		t.Fatalf("reconnected with the revoked token")
	}
	waitReconnected(t, client)
	if got := node.authCount.Load(); got != 2 {
		t.Errorf("authenticate calls = %d, want 2", got)
	}
}

// A RefreshSessionEvent pushed by the server replaces the session: REST calls
// and cached clans use the new token, and what the event leaves blank is kept.
func TestRefreshSessionEventAdoptsSession(t *testing.T) {
	tokenV1 := testJWT(t, time.Now().Add(time.Hour), "v1")
	tokenV2 := testJWT(t, time.Now().Add(2*time.Hour), "v2")

	node := newFakeNode(t)
	node.session = func(int32) *api.Session {
		return &api.Session{Token: tokenV1, UserId: 12345, ApiUrl: node.srv.URL, WsUrl: node.host()}
	}
	client := loginTestClient(t, node)
	before := client.currentSession()

	// Through the socket dispatcher, as a frame from the server would arrive.
	client.socket.dispatch(&rtapi.Envelope{RefreshSessionEvent: &api.Session{Token: tokenV2}})

	after := client.currentSession()
	if after.Token != tokenV2 {
		t.Fatalf("session token not replaced by the event")
	}
	if after.APIURL != before.APIURL || after.WsURL != before.WsURL || after.UserID != before.UserID {
		t.Errorf("event session dropped fields of the old one: %+v", after)
	}
	if before.Token != tokenV1 {
		t.Errorf("the previous session value was mutated in place")
	}
	if clan, ok := client.Clans.Get("0"); !ok || clan.SessionToken != tokenV2 {
		t.Errorf("clan session token not updated")
	}

	// A session that cannot be parsed is ignored.
	client.socket.dispatch(&rtapi.Envelope{RefreshSessionEvent: &api.Session{Token: "not-a-jwt"}})
	if got := client.SessionToken(); got != tokenV2 {
		t.Errorf("an invalid event session replaced the current one")
	}
	if got := node.authCount.Load(); got != 1 {
		t.Errorf("authenticate calls = %d, want 1", got)
	}
}

// Concurrent callers that find the session expired share one authenticate
// round trip, and a valid session is returned without one.
func TestEnsureValidSessionSingleFlight(t *testing.T) {
	expired := testJWT(t, time.Now().Add(-time.Minute), "old")
	fresh := testJWT(t, time.Now().Add(time.Hour), "new")

	node := newFakeNode(t)
	release := make(chan struct{})
	node.session = func(int32) *api.Session {
		<-release
		return &api.Session{Token: fresh, UserId: 9}
	}

	u, _ := url.Parse(node.srv.URL)
	useSSL := false
	client, err := NewMezonClient(ClientConfig{BotID: "9", Token: "bot-secret", Host: u.Hostname(), Port: u.Port(), UseSSL: &useSSL})
	if err != nil {
		t.Fatal(err)
	}
	old, err := NewSession(expired, "", "9", "http://api.test:80", "id-token", "ws.test:443")
	if err != nil {
		t.Fatal(err)
	}
	old.TcpURL = "tcp.test:443"
	client.setSession(old)

	const callers = 8
	var wg sync.WaitGroup
	tokens := make([]string, callers)
	for i := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s, err := client.EnsureValidSession(); err == nil {
				tokens[i] = s.Token
			}
		}()
	}
	// Let every caller reach the in-flight re-authentication before it ends.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, token := range tokens {
		if token != fresh {
			t.Fatalf("caller %d got token %q", i, token)
		}
	}
	if got := node.authCount.Load(); got != 1 {
		t.Errorf("authenticate calls = %d, want 1", got)
	}
	// The server left the endpoints blank: they carry over from the old session.
	s := client.currentSession()
	if s.APIURL != old.APIURL || s.WsURL != old.WsURL || s.TcpURL != old.TcpURL || s.IDToken != old.IDToken || s.UserID != "9" {
		t.Errorf("renewed session dropped fields of the old one: %+v", s)
	}
	if _, err := client.EnsureValidSession(); err != nil || node.authCount.Load() != 1 {
		t.Errorf("valid session must not re-authenticate (err=%v, calls=%d)", err, node.authCount.Load())
	}
}
