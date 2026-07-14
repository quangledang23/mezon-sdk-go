package mezon

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quangledang23/mezon-sdk-go/api"
	"google.golang.org/protobuf/proto"
)

// TestMezonAuthenticateRequest verifies the authenticate call matches the TS
// SDK: Basic auth username is the token with an empty password (base64("token:")),
// the JSON body carries account.appid/token, and the protobuf Session is decoded.
func TestMezonAuthenticateRequest(t *testing.T) {
	const botID, token = "123456789", "secret-token"

	var gotAuth, gotPath, gotMethod, gotAccept string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		out, err := proto.Marshal(&api.Session{Token: "session-jwt"})
		if err != nil {
			t.Errorf("marshal session: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	apiClient := NewMezonApi(token, srv.URL, 0)
	sess, err := apiClient.MezonAuthenticate(botID, token)
	if err != nil {
		t.Fatalf("MezonAuthenticate: %v", err)
	}
	if sess.Token != "session-jwt" {
		t.Errorf("session token = %q, want %q", sess.Token, "session-jwt")
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v2/apps/authenticate/token" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAccept != "application/x-protobuf" {
		t.Errorf("Accept = %q", gotAccept)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(token+":"))
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q (username=token, empty password)", gotAuth, wantAuth)
	}
	account, _ := gotBody["account"].(map[string]any)
	if account == nil || account["appid"] != botID || account["token"] != token {
		t.Errorf("body account = %v, want appid=%q token=%q", account, botID, token)
	}
}

// TestDoProtoOverSocket verifies /mezon.api.Mezon/ requests ride the realtime
// socket when it is open: the HTTP base path is unreachable on purpose, so a
// successful round trip proves the socket carried it.
func TestDoProtoOverSocket(t *testing.T) {
	addr, closeFn := fakeTCPServer(t, "tok")
	defer closeFn()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	s := NewDefaultSocket("", host, port, false, func(string, any) {})
	if err := s.Connect(&Session{Token: "tok"}, false); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer s.Close()

	apiClient := NewMezonApi("k", "http://127.0.0.1:1", 0)
	apiClient.socket = s

	resp := &api.Session{}
	err = apiClient.doProto("bearer", "/mezon.api.Mezon/ListClanDescs", &api.Session{Token: "round-trip"}, resp)
	if err != nil {
		t.Fatalf("doProto: %v", err)
	}
	if resp.Token != "round-trip" {
		t.Fatalf("resp token = %q, want %q", resp.Token, "round-trip")
	}
}

// TestDoProtoFallsBackToHTTP verifies a closed socket routes to HTTP.
func TestDoProtoFallsBackToHTTP(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		out, _ := proto.Marshal(&api.Session{Token: "via-http"})
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	apiClient := NewMezonApi("k", srv.URL, 0)
	apiClient.socket = NewDefaultSocket("", "127.0.0.1", "1", false, func(string, any) {}) // never connected

	resp := &api.Session{}
	if err := apiClient.doProto("bearer", "/mezon.api.Mezon/ListClanDescs", nil, resp); err != nil {
		t.Fatalf("doProto: %v", err)
	}
	if hits != 1 || resp.Token != "via-http" {
		t.Fatalf("hits = %d, token = %q; want HTTP fallback", hits, resp.Token)
	}
}
