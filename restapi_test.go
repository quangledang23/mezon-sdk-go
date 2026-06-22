package mezon

import (
	"encoding/base64"
	"encoding/json"
	"io"
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
