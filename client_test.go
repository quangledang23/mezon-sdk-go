package mezon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

func makeTestJWT(t *testing.T, exp int64, tag string) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{"exp": exp, "tag": tag})
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(claims)
	return fmt.Sprintf("%s.%s.sig", hdr, payload)
}

// TestRetryConnectReauthenticatesExpiredSession reproduces the exact production
// bug where a bot reconnects after its initial 1h JWT expired: retryConnect
// must call MezonAuthenticate to obtain a fresh JWT instead of looping forever
// with 403 on the stale token.
func TestRetryConnectReauthenticatesExpiredSession(t *testing.T) {
	origInit, origMax := reconnectInitialDelay, reconnectMaxDelay
	reconnectInitialDelay = 15 * time.Millisecond
	reconnectMaxDelay = 30 * time.Millisecond
	t.Cleanup(func() {
		reconnectInitialDelay = origInit
		reconnectMaxDelay = origMax
	})

	tokenV1 := makeTestJWT(t, time.Now().Add(-time.Hour).Unix(), "v1")
	tokenV2 := makeTestJWT(t, time.Now().Add(time.Hour).Unix(), "v2")

	var authCount atomic.Int32
	var tokenV1Revoked atomic.Bool
	var v2Connected atomic.Bool

	var mu sync.Mutex
	var firstWS *websocket.Conn
	webrtcCh := make(chan *rtapi.WebrtcSignalingFwd, 1)

	upgrader := websocket.Upgrader{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/apps/authenticate/token":
			n := authCount.Add(1)
			tok := tokenV1
			if n >= 2 {
				tok = tokenV2
			}
			u, _ := url.Parse(srv.URL)
			out, _ := proto.Marshal(&api.Session{
				Token:  tok,
				UserId: 12345,
				ApiUrl: srv.URL,
				WsUrl:  u.Host,
			})
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(out)

		case "/mezon.api.Mezon/ListClanDescs":
			if tokenV1Revoked.Load() && r.Header.Get("Authorization") != "Bearer "+tokenV2 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			out, _ := proto.Marshal(&api.ClanDescList{})
			_, _ = w.Write(out)

		case "/mezon.api.Mezon/ListChannelDescs":
			out, _ := proto.Marshal(&api.ChannelDescList{})
			_, _ = w.Write(out)

		case "/ws":
			tok := r.URL.Query().Get("token")
			if tokenV1Revoked.Load() && tok != tokenV2 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			c, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer c.Close()

			if tok == tokenV1 {
				mu.Lock()
				firstWS = c
				mu.Unlock()
			} else if tok == tokenV2 {
				v2Connected.Store(true)
			}

			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}
				env := &rtapi.Envelope{}
				if proto.Unmarshal(data, env) != nil {
					continue
				}
				if env.ApiRequestEvent != nil {
					var body []byte
					switch env.ApiRequestEvent.ApiName {
					case "ListClanDescs":
						body, _ = proto.Marshal(&api.ClanDescList{})
					case "ListChannelDescs":
						body, _ = proto.Marshal(&api.ChannelDescList{})
					}
					_ = c.WriteMessage(websocket.BinaryMessage, rawFrame(uint16(env.Cid), 0, rawCodeFin, body))
					continue
				}
				if env.Cid != 0 {
					ack, _ := proto.Marshal(&rtapi.Envelope{Cid: env.Cid})
					_ = c.WriteMessage(websocket.BinaryMessage, ack)
					// Once reconnected with v2 and joined pseudo-clan 0, send a WebRTC signaling event.
					if tok == tokenV2 && env.ClanJoin != nil {
						sig, _ := proto.Marshal(&rtapi.Envelope{
							WebrtcSignalingFwd: &rtapi.WebrtcSignalingFwd{
								CallerId:   999,
								ReceiverId: 12345,
								ChannelId:  777,
								DataType:   1,
								JsonData:   `{"type":"offer","sdp":"v=0"}`,
							},
						})
						_ = c.WriteMessage(websocket.BinaryMessage, sig)
					}
				}
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
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
	client.On(EventWebrtcSignalingFwd, func(p any) {
		if sig, ok := p.(*rtapi.WebrtcSignalingFwd); ok {
			select {
			case webrtcCh <- sig:
			default:
			}
		}
	})

	if err := client.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}
	defer client.Close()

	if authCount.Load() != 1 {
		t.Fatalf("initial authCount = %d, want 1", authCount.Load())
	}

	// Simulate token expiry and gateway disconnect.
	tokenV1Revoked.Store(true)
	mu.Lock()
	ws := firstWS
	mu.Unlock()
	if ws == nil {
		t.Fatal("firstWS was nil")
	}
	_ = ws.Close()

	select {
	case sig := <-webrtcCh:
		if sig.CallerId != 999 {
			t.Fatalf("unexpected caller id: %d", sig.CallerId)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("did not reconnect and receive signaling after token expiry (authCount=%d, v2Connected=%v)",
			authCount.Load(), v2Connected.Load())
	}

	if authCount.Load() < 2 {
		t.Fatalf("expected re-authentication on reconnect, got authCount=%d", authCount.Load())
	}
	client.mu.Lock()
	gotToken := client.session.Token
	client.mu.Unlock()
	if gotToken != tokenV2 {
		t.Fatalf("session token not updated after reconnect")
	}
}
