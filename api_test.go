package mezonlightsdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

func TestNewMezonApiDefaultTimeout(t *testing.T) {
	api := NewMezonApi("key", 0, "https://api.test")
	if api.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want 7s default", api.Timeout)
	}
	if api.BasePath() != "https://api.test" {
		t.Errorf("BasePath() = %q", api.BasePath())
	}

	api.SetBasePath("https://other.test")
	if api.BasePath() != "https://other.test" {
		t.Errorf("BasePath() after SetBasePath = %q", api.BasePath())
	}
}

func TestAuthenticateIdToken(t *testing.T) {
	var gotAuth, gotContentType string
	var gotBody ApiAuthenticationIdToken
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/account/authenticate/idtoken" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		json.NewEncoder(w).Encode(AuthenticationIdTokenResponse{
			Token:        "tok",
			RefreshToken: "refresh",
			APIURL:       "https://api.test",
			WSURL:        "gw.test",
			UserID:       "42",
		})
	}))
	defer srv.Close()

	api := NewMezonApi("serverkey", 0, srv.URL)
	resp, err := api.AuthenticateIdToken(context.Background(), "serverkey", "", &ApiAuthenticationIdToken{
		IDToken:  "id-token",
		UserID:   "42",
		Username: "alice",
	})
	if err != nil {
		t.Fatalf("AuthenticateIdToken: %v", err)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("serverkey:"))
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotContentType != "application/proto" {
		t.Errorf("Content-Type = %q, want application/proto", gotContentType)
	}
	if gotBody.IDToken != "id-token" || gotBody.UserID != "42" || gotBody.Username != "alice" {
		t.Errorf("request body = %+v", gotBody)
	}
	if resp.Token != "tok" || resp.RefreshToken != "refresh" || resp.UserID != "42" {
		t.Errorf("response = %+v", resp)
	}
}

func TestAuthenticateIdTokenNilBody(t *testing.T) {
	api := NewMezonApi("key", 0, "http://unused")
	if _, err := api.AuthenticateIdToken(context.Background(), "key", "", nil); err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestAuthenticateIdTokenNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	api := NewMezonApi("key", 0, srv.URL)
	resp, err := api.AuthenticateIdToken(context.Background(), "key", "", &ApiAuthenticationIdToken{})
	if err != nil {
		t.Fatalf("AuthenticateIdToken: %v", err)
	}
	if *resp != (AuthenticationIdTokenResponse{}) {
		t.Errorf("204 response should yield zero value, got %+v", resp)
	}
}

func TestAuthenticateIdTokenHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	api := NewMezonApi("key", 0, srv.URL)
	_, err := api.AuthenticateIdToken(context.Background(), "key", "", &ApiAuthenticationIdToken{})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
}

func TestAuthenticateApp(t *testing.T) {
	var gotAuth string
	var gotBody ApiAuthenticateAppRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/apps/authenticate/token" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		// The real gateway answers with JSON.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"tok","refresh_token":"refresh","user_id":"42","api_url":"https://api.test","ws_url":"sock.test"}`))
	}))
	defer srv.Close()

	api := NewMezonApi("api-key", 0, srv.URL)
	sess, err := api.AuthenticateApp(context.Background(), "api-key", "", &ApiAuthenticateAppRequest{
		Account: ApiAppAccount{AppID: "42", Token: "api-key"},
	})
	if err != nil {
		t.Fatalf("AuthenticateApp: %v", err)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("api-key:"))
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotBody.Account.AppID != "42" || gotBody.Account.Token != "api-key" {
		t.Errorf("request body = %+v", gotBody)
	}
	if sess.Token != "tok" || sess.RefreshToken != "refresh" || sess.UserID != "42" ||
		sess.APIURL != "https://api.test" || sess.WsURL != "sock.test" {
		t.Errorf("response = %+v", sess)
	}
}

func TestAuthenticateAppProtobufResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write((&proto.Session{Token: "tok", RefreshToken: "refresh", WsURL: "sock.test"}).Marshal())
	}))
	defer srv.Close()

	api := NewMezonApi("api-key", 0, srv.URL)
	sess, err := api.AuthenticateApp(context.Background(), "api-key", "", &ApiAuthenticateAppRequest{})
	if err != nil {
		t.Fatalf("AuthenticateApp: %v", err)
	}
	if sess.Token != "tok" || sess.WsURL != "sock.test" {
		t.Errorf("response = %+v", sess)
	}
}

func TestAuthenticateAppNilBody(t *testing.T) {
	api := NewMezonApi("key", 0, "http://unused")
	if _, err := api.AuthenticateApp(context.Background(), "key", "", nil); err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestSessionRefresh(t *testing.T) {
	var gotReq proto.SessionRefreshRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mezon.api.Mezon/SessionRefresh" {
			t.Errorf("path = %q", r.URL.Path)
		}
		data, _ := io.ReadAll(r.Body)
		if err := gotReq.Unmarshal(data); err != nil {
			t.Errorf("request body is not a SessionRefreshRequest: %v", err)
		}
		w.Write((&proto.Session{Token: "new-token", RefreshToken: "new-refresh", IsRemember: true}).Marshal())
	}))
	defer srv.Close()

	api := NewMezonApi("key", 0, srv.URL)
	sess, err := api.SessionRefresh(context.Background(), "key", "", &ApiSessionRefreshRequest{
		Token:      "old-refresh",
		Vars:       map[string]string{"k": "v"},
		IsRemember: true,
	})
	if err != nil {
		t.Fatalf("SessionRefresh: %v", err)
	}
	if gotReq.Token != "old-refresh" || gotReq.Vars["k"] != "v" || !gotReq.IsRemember {
		t.Errorf("request = %+v", gotReq)
	}
	if sess.Token != "new-token" || sess.RefreshToken != "new-refresh" || !sess.IsRemember {
		t.Errorf("response = %+v", sess)
	}
}

func TestCreateChannelDesc(t *testing.T) {
	var gotAuth string
	var gotReq proto.CreateChannelDescRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mezon.api.Mezon/CreateChannelDesc" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		data, _ := io.ReadAll(r.Body)
		if err := gotReq.Unmarshal(data); err != nil {
			t.Errorf("request body is not a CreateChannelDescRequest: %v", err)
		}
		w.Write((&proto.ChannelDescription{ChannelID: "123", Type: ChannelTypeDM}).Marshal())
	}))
	defer srv.Close()

	api := NewMezonApi("key", 0, srv.URL)
	desc, err := api.CreateChannelDesc(context.Background(), "bearer-token", &ApiCreateChannelDescRequest{
		Type:           ChannelTypeDM,
		ChannelPrivate: 1,
		UserIDs:        []string{"456"},
	})
	if err != nil {
		t.Fatalf("CreateChannelDesc: %v", err)
	}
	if gotAuth != "Bearer bearer-token" {
		t.Errorf("Authorization = %q, want Bearer token", gotAuth)
	}
	if gotReq.Type != ChannelTypeDM || gotReq.ChannelPrivate != 1 || len(gotReq.UserIDs) != 1 || gotReq.UserIDs[0] != "456" {
		t.Errorf("request = %+v", gotReq)
	}
	if desc.ChannelID != "123" || desc.Type != ChannelTypeDM {
		t.Errorf("response = %+v", desc)
	}
}

func TestUploadAttachmentFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mezon.api.Mezon/UploadAttachmentFile" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var req proto.UploadAttachmentRequest
		data, _ := io.ReadAll(r.Body)
		if err := req.Unmarshal(data); err != nil {
			t.Errorf("request body is not an UploadAttachmentRequest: %v", err)
		}
		w.Write((&proto.UploadAttachment{Filename: req.Filename, URL: "https://cdn.test/" + req.Filename}).Marshal())
	}))
	defer srv.Close()

	api := NewMezonApi("key", 0, srv.URL)
	up, err := api.UploadAttachmentFile(context.Background(), "token", &ApiUploadAttachmentRequest{
		Filename: "photo.png",
		Filetype: "image/png",
		Size:     1024,
	})
	if err != nil {
		t.Fatalf("UploadAttachmentFile: %v", err)
	}
	if up.Filename != "photo.png" || up.URL != "https://cdn.test/photo.png" {
		t.Errorf("response = %+v", up)
	}
}

func TestProtoEndpointsNilBody(t *testing.T) {
	api := NewMezonApi("key", 0, "http://unused")
	ctx := context.Background()

	if _, err := api.SessionRefresh(ctx, "key", "", nil); err == nil {
		t.Error("SessionRefresh(nil) expected error")
	}
	if _, err := api.CreateChannelDesc(ctx, "tok", nil); err == nil {
		t.Error("CreateChannelDesc(nil) expected error")
	}
	if _, err := api.UploadAttachmentFile(ctx, "tok", nil); err == nil {
		t.Error("UploadAttachmentFile(nil) expected error")
	}
}

func TestAuthHeaders(t *testing.T) {
	if got := basicAuthHeader("", "pw"); got != "" {
		t.Errorf("basicAuthHeader with empty username = %q, want empty", got)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pw"))
	if got := basicAuthHeader("user", "pw"); got != want {
		t.Errorf("basicAuthHeader = %q, want %q", got, want)
	}
	if got := bearerAuthHeader(""); got != "" {
		t.Errorf("bearerAuthHeader with empty token = %q, want empty", got)
	}
	if got := bearerAuthHeader("tok"); got != "Bearer tok" {
		t.Errorf("bearerAuthHeader = %q, want %q", got, "Bearer tok")
	}
}
