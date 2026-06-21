package mezonlightsdk

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// makeJWT builds an unsigned JWT whose payload carries the given claims, for
// use across the test suite.
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestNewSessionDecodesClaims(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	refreshExp := time.Now().Add(24 * time.Hour).Unix()
	token := makeJWT(t, map[string]any{
		"exp": exp,
		"usn": "johndoe",
		"uid": "42",
		"vrs": map[string]any{"role": "admin", "level": 7},
	})
	refresh := makeJWT(t, map[string]any{"exp": refreshExp})

	s, err := NewSession(token, refresh, true, "https://api.test", "gw.test", "id-token", true)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if s.Token != token || s.RefreshToken != refresh {
		t.Error("tokens not stored on session")
	}
	if !s.Created || s.CreatedAt == 0 {
		t.Errorf("Created = %v, CreatedAt = %d, want true and non-zero", s.Created, s.CreatedAt)
	}
	if s.ExpiresAt != exp {
		t.Errorf("ExpiresAt = %d, want %d", s.ExpiresAt, exp)
	}
	if s.RefreshExpiresAt != refreshExp {
		t.Errorf("RefreshExpiresAt = %d, want %d", s.RefreshExpiresAt, refreshExp)
	}
	if s.Username != "johndoe" || s.UserID != "42" {
		t.Errorf("Username = %q, UserID = %q", s.Username, s.UserID)
	}
	if s.Vars["role"] != "admin" || s.Vars["level"] != "7" {
		t.Errorf("Vars = %v, want role=admin level=7", s.Vars)
	}
	if s.APIURL != "https://api.test" || s.WSURL != "gw.test" || s.IDToken != "id-token" {
		t.Error("endpoint fields not stored on session")
	}
	if !s.IsRemember {
		t.Error("IsRemember = false, want true")
	}
}

func TestNewSessionInvalidToken(t *testing.T) {
	refresh := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})

	_, err := NewSession("not-a-jwt", refresh, false, "", "", "", false)
	var sessErr *SessionError
	if !errors.As(err, &sessErr) {
		t.Fatalf("NewSession(invalid token) error = %v, want *SessionError", err)
	}
}

func TestNewSessionInvalidRefreshToken(t *testing.T) {
	token := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})

	_, err := NewSession(token, "broken.refresh", false, "", "", "", false)
	var sessErr *SessionError
	if !errors.As(err, &sessErr) {
		t.Fatalf("NewSession(invalid refresh) error = %v, want *SessionError", err)
	}
}

func TestRestoreSession(t *testing.T) {
	token := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "uid": "7"})
	refresh := makeJWT(t, map[string]any{"exp": time.Now().Add(2 * time.Hour).Unix()})

	s, err := RestoreSession(token, refresh, "https://api.test", "gw.test", true)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if s.Created {
		t.Error("restored session reports Created = true")
	}
	if s.UserID != "7" {
		t.Errorf("UserID = %q, want %q", s.UserID, "7")
	}
}

func TestSessionUpdateKeepsRefreshWhenEmpty(t *testing.T) {
	oldRefreshExp := time.Now().Add(24 * time.Hour).Unix()
	token := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	refresh := makeJWT(t, map[string]any{"exp": oldRefreshExp})

	s, err := NewSession(token, refresh, false, "", "", "", true)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	newExp := time.Now().Add(2 * time.Hour).Unix()
	newToken := makeJWT(t, map[string]any{"exp": newExp, "usn": "renamed"})
	if err := s.Update(newToken, "", false); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if s.Token != newToken || s.ExpiresAt != newExp || s.Username != "renamed" {
		t.Error("token claims not re-decoded on update")
	}
	if s.RefreshToken != refresh || s.RefreshExpiresAt != oldRefreshExp {
		t.Error("empty refresh token must keep the previous refresh state")
	}
	if !s.IsRemember {
		t.Error("IsRemember must be untouched when refresh token is empty")
	}
}

func TestSessionExpiry(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s := &Session{ExpiresAt: 1_000_000, RefreshExpiresAt: 1_000_001}

	if !s.IsExpired(now) {
		t.Error("ExpiresAt == now must report expired")
	}
	if s.IsRefreshExpired(now) {
		t.Error("RefreshExpiresAt > now must not report expired")
	}
	if !s.IsRefreshExpired(now.Add(time.Second)) {
		t.Error("RefreshExpiresAt == now must report expired")
	}
	if (&Session{ExpiresAt: 1_000_001}).IsExpired(now) {
		t.Error("ExpiresAt > now must not report expired")
	}
}

func TestDecodeJWTClaimsErrors(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"wrong part count", "only.two"},
		{"payload not base64", "h.!!!.s"},
		{"payload not JSON", "h." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeJWTClaims(tt.token); err == nil {
				t.Errorf("decodeJWTClaims(%q) expected error", tt.token)
			}
		})
	}
}

func TestDecodeJWTClaimsStringExp(t *testing.T) {
	token := makeJWT(t, map[string]any{"exp": "12345"})
	claims, err := decodeJWTClaims(token)
	if err != nil {
		t.Fatalf("decodeJWTClaims: %v", err)
	}
	if claims.exp != 12345 {
		t.Errorf("exp = %d, want 12345", claims.exp)
	}
}

func TestDecodeBase64URLSegment(t *testing.T) {
	// URL-safe alphabet ('-' and '_').
	urlSeg := base64.RawURLEncoding.EncodeToString([]byte{0xfb, 0xef, 0xff})
	if got, err := decodeBase64URLSegment(urlSeg); err != nil || string(got) != "\xfb\xef\xff" {
		t.Errorf("decodeBase64URLSegment(url-safe) = %x, %v", got, err)
	}

	// Standard alphabet ('+' and '/') as a fallback.
	stdSeg := base64.RawStdEncoding.EncodeToString([]byte{0xfb, 0xef, 0xff})
	if got, err := decodeBase64URLSegment(stdSeg); err != nil || string(got) != "\xfb\xef\xff" {
		t.Errorf("decodeBase64URLSegment(std) = %x, %v", got, err)
	}

	// Padded input.
	if got, err := decodeBase64URLSegment("QQ=="); err != nil || string(got) != "A" {
		t.Errorf("decodeBase64URLSegment(padded) = %q, %v", got, err)
	}
}

func TestClaimInt64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
	}{
		{"json integer", json.Number("123"), 123},
		{"json float", json.Number("123.9"), 123},
		{"numeric string", "456", 456},
		{"non-numeric string", "abc", 0},
		{"nil", nil, 0},
		{"bool", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claimInt64(tt.in); got != tt.want {
				t.Errorf("claimInt64(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestClaimString(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"string", "hello", "hello"},
		{"json number", json.Number("7"), "7"},
		{"nil", nil, ""},
		{"bool marshals to JSON", true, "true"},
		{"map marshals to JSON", map[string]any{"a": 1}, `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claimString(tt.in); got != tt.want {
				t.Errorf("claimString(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
