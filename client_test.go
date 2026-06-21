package mezonlightsdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

// newTestClient builds a LightClient whose API base path points at the given
// server URL, with freshly minted JWT tokens.
func newTestClient(t *testing.T, apiURL string) *LightClient {
	t.Helper()
	token := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "uid": "42", "usn": "alice"})
	refresh := makeJWT(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
	client, err := InitClient(ClientInitConfig{
		Token:        token,
		RefreshToken: refresh,
		APIURL:       apiURL,
		WSURL:        "gw.test",
		UserID:       "42",
	})
	if err != nil {
		t.Fatalf("InitClient: %v", err)
	}
	return client
}

func TestParseBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://api.mezon.ai/some/path?x=1", "https://api.mezon.ai"},
		{"https://api.test:8443/path", "https://api.test:8443"},
		{"http://api.test:8080", "http://api.test:8080"},
		// Non-https schemes fall back to http, mirroring the TypeScript SDK.
		{"ftp://files.test", "http://files.test"},
	}
	for _, tt := range tests {
		got, err := parseBaseURL(tt.in)
		if err != nil {
			t.Errorf("parseBaseURL(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	if _, err := parseBaseURL("http://bad host/"); err == nil {
		t.Error("parseBaseURL with invalid host expected error")
	}
}

func TestInitClientMissingFields(t *testing.T) {
	configs := []ClientInitConfig{
		{},
		{Token: "t", RefreshToken: "r", APIURL: "https://a", WSURL: "w"},  // missing UserID
		{Token: "t", RefreshToken: "r", APIURL: "https://a", UserID: "u"}, // missing WSURL
		{Token: "t", RefreshToken: "r", WSURL: "w", UserID: "u"},          // missing APIURL
		{Token: "t", APIURL: "https://a", WSURL: "w", UserID: "u"},        // missing RefreshToken
		{RefreshToken: "r", APIURL: "https://a", WSURL: "w", UserID: "u"}, // missing Token
	}
	for i, cfg := range configs {
		_, err := InitClient(cfg)
		var sessErr *SessionError
		if !errors.As(err, &sessErr) {
			t.Errorf("config %d: error = %v, want *SessionError", i, err)
		}
	}
}

func TestInitClientInvalidToken(t *testing.T) {
	_, err := InitClient(ClientInitConfig{
		Token:        "not-a-jwt",
		RefreshToken: "also-not-a-jwt",
		APIURL:       "https://api.test",
		WSURL:        "gw.test",
		UserID:       "42",
	})
	var sessErr *SessionError
	if !errors.As(err, &sessErr) {
		t.Fatalf("error = %v, want *SessionError", err)
	}
}

func TestInitClientAndExportSession(t *testing.T) {
	client := newTestClient(t, "https://api.test/with/path")

	if client.UserID() != "42" {
		t.Errorf("UserID() = %q, want %q", client.UserID(), "42")
	}
	if client.Session().Username != "alice" {
		t.Errorf("Session().Username = %q, want %q", client.Session().Username, "alice")
	}
	if client.Token() != client.Session().Token || client.RefreshToken() != client.Session().RefreshToken {
		t.Error("Token()/RefreshToken() must mirror the session")
	}
	if got := client.Client().BasePath(); got != "https://api.test" {
		t.Errorf("BasePath() = %q, want path stripped", got)
	}
	if client.Client().ServerKey != DefaultServerKey {
		t.Errorf("ServerKey = %q, want default", client.Client().ServerKey)
	}
	if client.IsSessionExpired() || client.IsRefreshSessionExpired() {
		t.Error("fresh tokens must not be expired")
	}

	exported := client.ExportSession()
	want := ClientInitConfig{
		Token:        client.Token(),
		RefreshToken: client.RefreshToken(),
		APIURL:       "https://api.test/with/path",
		WSURL:        "gw.test",
		UserID:       "42",
	}
	if exported != want {
		t.Errorf("ExportSession() = %+v, want %+v", exported, want)
	}

	// The exported config must round-trip through InitClient.
	if _, err := InitClient(exported); err != nil {
		t.Errorf("InitClient(ExportSession()): %v", err)
	}
}

func TestInitClientCustomServerKey(t *testing.T) {
	token := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	refresh := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	client, err := InitClient(ClientInitConfig{
		Token:        token,
		RefreshToken: refresh,
		APIURL:       "https://api.test",
		WSURL:        "gw.test",
		UserID:       "42",
		ServerKey:    "custom-key",
	})
	if err != nil {
		t.Fatalf("InitClient: %v", err)
	}
	if client.Client().ServerKey != "custom-key" {
		t.Errorf("ServerKey = %q, want %q", client.Client().ServerKey, "custom-key")
	}
}

func TestAuthenticate(t *testing.T) {
	token := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "uid": "42"})
	refresh := makeJWT(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})

	var gotBody ApiAuthenticationIdToken
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(AuthenticationIdTokenResponse{
			Token:        token,
			RefreshToken: refresh,
			APIURL:       "https://user-api.test/path",
			WSURL:        "user-gw.test",
			UserID:       "42",
		})
	}))
	defer srv.Close()

	client, err := Authenticate(context.Background(), AuthenticateConfig{
		IDToken:    "id-token",
		UserID:     "42",
		Username:   "alice",
		GatewayURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	if gotBody.IDToken != "id-token" || gotBody.Username != "alice" {
		t.Errorf("request body = %+v", gotBody)
	}
	if client.UserID() != "42" {
		t.Errorf("UserID() = %q", client.UserID())
	}
	// The base path must switch to the user-specific endpoint.
	if got := client.Client().BasePath(); got != "https://user-api.test" {
		t.Errorf("BasePath() = %q, want %q", got, "https://user-api.test")
	}
	if client.Session().WSURL != "user-gw.test" {
		t.Errorf("WSURL = %q", client.Session().WSURL)
	}
}

func TestAuthenticateIncompleteResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(AuthenticationIdTokenResponse{Token: "only-token"})
	}))
	defer srv.Close()

	_, err := Authenticate(context.Background(), AuthenticateConfig{GatewayURL: srv.URL})
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %v, want *AuthenticationError", err)
	}
}

func TestAuthenticateHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := Authenticate(context.Background(), AuthenticateConfig{GatewayURL: srv.URL})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
}

func TestAuthenticateBot(t *testing.T) {
	token := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "uid": "2028"})
	refresh := makeJWT(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})

	var gotBody ApiAuthenticateAppRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token":         token,
			"refresh_token": refresh,
			"user_id":       "2028",
			"api_url":       "https://user-api.test/path",
			"ws_url":        "sock.test",
		})
	}))
	defer srv.Close()

	client, err := AuthenticateBot(context.Background(), AuthenticateBotConfig{
		BotID:      "2028",
		APIKey:     "api-key",
		GatewayURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("AuthenticateBot: %v", err)
	}

	if gotBody.Account.AppID != "2028" || gotBody.Account.Token != "api-key" {
		t.Errorf("request body = %+v", gotBody)
	}
	if client.UserID() != "2028" {
		t.Errorf("UserID() = %q", client.UserID())
	}
	if got := client.Client().BasePath(); got != "https://user-api.test" {
		t.Errorf("BasePath() = %q, want user api endpoint", got)
	}
	if client.Session().WSURL != "sock.test" {
		t.Errorf("WSURL = %q, want %q", client.Session().WSURL, "sock.test")
	}
	// The API key doubles as the server key for session refreshes.
	if client.Client().ServerKey != "api-key" {
		t.Errorf("ServerKey = %q, want the API key", client.Client().ServerKey)
	}
}

func TestAuthenticateBotMissingFields(t *testing.T) {
	var authErr *AuthenticationError
	if _, err := AuthenticateBot(context.Background(), AuthenticateBotConfig{APIKey: "k"}); !errors.As(err, &authErr) {
		t.Errorf("missing BotID: error = %v, want *AuthenticationError", err)
	}
	if _, err := AuthenticateBot(context.Background(), AuthenticateBotConfig{BotID: "1"}); !errors.As(err, &authErr) {
		t.Errorf("missing APIKey: error = %v, want *AuthenticationError", err)
	}
}

func TestAuthenticateBotMissingTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"user_id":"42"}`))
	}))
	defer srv.Close()

	_, err := AuthenticateBot(context.Background(), AuthenticateBotConfig{BotID: "1", APIKey: "k", GatewayURL: srv.URL})
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %v, want *AuthenticationError", err)
	}
}

func TestRefreshSession(t *testing.T) {
	newToken := makeJWT(t, map[string]any{"exp": time.Now().Add(2 * time.Hour).Unix(), "usn": "alice2"})
	newRefresh := makeJWT(t, map[string]any{"exp": time.Now().Add(48 * time.Hour).Unix()})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write((&proto.Session{Token: newToken, RefreshToken: newRefresh, IsRemember: true}).Marshal())
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	var callbackSession *ApiSession
	client.OnRefreshSession = func(s *ApiSession) { callbackSession = s }

	sess, err := client.RefreshSession(context.Background())
	if err != nil {
		t.Fatalf("RefreshSession: %v", err)
	}
	if sess.Token != newToken || sess.RefreshToken != newRefresh {
		t.Error("session tokens not updated after refresh")
	}
	if sess.Username != "alice2" {
		t.Errorf("Username = %q, want claims re-decoded", sess.Username)
	}
	if callbackSession == nil || callbackSession.Token != newToken {
		t.Error("OnRefreshSession callback not invoked with the new session")
	}
}

func TestRefreshSessionErrorThenRecover(t *testing.T) {
	newToken := makeJWT(t, map[string]any{"exp": time.Now().Add(2 * time.Hour).Unix()})
	newRefresh := makeJWT(t, map[string]any{"exp": time.Now().Add(48 * time.Hour).Unix()})

	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write((&proto.Session{Token: newToken, RefreshToken: newRefresh}).Marshal())
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	_, err := client.RefreshSession(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("first refresh error = %v, want *APIError", err)
	}

	// A failed refresh must not poison subsequent attempts.
	fail.Store(false)
	sess, err := client.RefreshSession(context.Background())
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if sess.Token != newToken {
		t.Error("session not updated on the retry")
	}
}

func TestRefreshSessionSingleFlight(t *testing.T) {
	newToken := makeJWT(t, map[string]any{"exp": time.Now().Add(2 * time.Hour).Unix()})
	newRefresh := makeJWT(t, map[string]any{"exp": time.Now().Add(48 * time.Hour).Unix()})

	var calls int32
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(entered)
			<-release
		}
		w.Write((&proto.Session{Token: newToken, RefreshToken: newRefresh}).Marshal())
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		client.RefreshSession(context.Background())
	}()
	<-entered // the first refresh is now in flight

	wg.Add(1)
	go func() {
		defer wg.Done()
		client.RefreshSession(context.Background())
	}()
	time.Sleep(50 * time.Millisecond) // let the second caller reach the in-flight wait
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server received %d refresh requests, want 1 (single-flight)", got)
	}
	if client.Token() != newToken {
		t.Error("session not updated after concurrent refresh")
	}
}

func TestCreateDMAndGroupDM(t *testing.T) {
	var gotReq proto.CreateChannelDescRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := make([]byte, r.ContentLength)
		r.Body.Read(data)
		gotReq = proto.CreateChannelDescRequest{}
		gotReq.Unmarshal(data)
		w.Write((&proto.ChannelDescription{ChannelID: "999"}).Marshal())
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	ctx := context.Background()

	desc, err := client.CreateDM(ctx, "123")
	if err != nil {
		t.Fatalf("CreateDM: %v", err)
	}
	if desc.ChannelID != "999" {
		t.Errorf("ChannelID = %q", desc.ChannelID)
	}
	if gotReq.Type != ChannelTypeDM || gotReq.ChannelPrivate != 1 || len(gotReq.UserIDs) != 1 || gotReq.UserIDs[0] != "123" {
		t.Errorf("CreateDM request = %+v", gotReq)
	}

	if _, err := client.CreateGroupDM(ctx, []string{"123", "456"}); err != nil {
		t.Fatalf("CreateGroupDM: %v", err)
	}
	if gotReq.Type != ChannelTypeGroup || len(gotReq.UserIDs) != 2 {
		t.Errorf("CreateGroupDM request = %+v", gotReq)
	}

	if _, err := client.CreateGroupDM(ctx, nil); err == nil {
		t.Error("CreateGroupDM with no users expected error")
	}
}
