package mezonlightsdk

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Session is a session authenticated for a user with the Mezon server.
type Session struct {
	// Token is the authorization token used to construct this session.
	Token string
	// Created reports whether the user account for this session was just
	// created.
	Created bool
	// CreatedAt is the UNIX timestamp when this session was created.
	CreatedAt int64
	// ExpiresAt is the UNIX timestamp when this session will expire.
	ExpiresAt int64
	// RefreshExpiresAt is the UNIX timestamp when the refresh token will
	// expire.
	RefreshExpiresAt int64
	// RefreshToken can be used for session token renewal.
	RefreshToken string
	// Username of the user who owns this session.
	Username string
	// UserID of the user who owns this session.
	UserID string
	// Vars holds any custom properties associated with this session.
	Vars map[string]string
	// IsRemember reports whether "Remember Me" is enabled.
	IsRemember bool
	// APIURL is the API endpoint that belongs to the user.
	APIURL string
	// WSURL is the WebSocket host that belongs to the user.
	WSURL string
	// IDToken is the ID token for zklogin, if any.
	IDToken string
}

// NewSession builds a session from JWT tokens, decoding expiry and user
// claims from the token payloads.
func NewSession(token, refreshToken string, created bool, apiURL, wsURL, idToken string, isRemember bool) (*Session, error) {
	s := &Session{
		Created:    created,
		CreatedAt:  time.Now().Unix(),
		APIURL:     apiURL,
		WSURL:      wsURL,
		IDToken:    idToken,
		IsRemember: isRemember,
	}
	if err := s.Update(token, refreshToken, isRemember); err != nil {
		return nil, err
	}
	return s, nil
}

// RestoreSession restores a session from previously stored tokens.
func RestoreSession(token, refreshToken, apiURL, wsURL string, isRemember bool) (*Session, error) {
	return NewSession(token, refreshToken, false, apiURL, wsURL, "", isRemember)
}

// IsExpired reports whether the session token has expired at the given time.
func (s *Session) IsExpired(now time.Time) bool {
	return s.ExpiresAt-now.Unix() <= 0
}

// IsRefreshExpired reports whether the refresh token has expired at the
// given time.
func (s *Session) IsRefreshExpired(now time.Time) bool {
	return s.RefreshExpiresAt-now.Unix() <= 0
}

// Update replaces the session tokens and re-decodes the JWT claims.
func (s *Session) Update(token, refreshToken string, isRemember bool) error {
	claims, err := decodeJWTClaims(token)
	if err != nil {
		return &SessionError{Message: "jwt is not valid: " + err.Error()}
	}

	// Clients that have just updated to refresh tokens may not have a cached
	// refresh token yet.
	if refreshToken != "" {
		refreshClaims, err := decodeJWTClaims(refreshToken)
		if err != nil {
			return &SessionError{Message: "refresh jwt is not valid: " + err.Error()}
		}
		s.RefreshExpiresAt = refreshClaims.exp
		s.RefreshToken = refreshToken
		s.IsRemember = isRemember
	}

	s.Token = token
	s.ExpiresAt = claims.exp
	s.Username = claims.usn
	s.UserID = claims.uid
	s.Vars = claims.vrs
	return nil
}

type jwtClaims struct {
	exp int64
	usn string
	uid string
	vrs map[string]string
}

func decodeJWTClaims(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("token must consist of 3 parts")
	}

	payload, err := decodeBase64URLSegment(parts[1])
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}

	claims := &jwtClaims{
		exp: claimInt64(raw["exp"]),
		usn: claimString(raw["usn"]),
		uid: claimString(raw["uid"]),
	}
	if vrs, ok := raw["vrs"].(map[string]any); ok {
		claims.vrs = make(map[string]string, len(vrs))
		for k, v := range vrs {
			claims.vrs[k] = claimString(v)
		}
	}
	return claims, nil
}

// decodeBase64URLSegment decodes a JWT segment, accepting both URL-safe and
// standard alphabets, with or without padding.
func decodeBase64URLSegment(seg string) ([]byte, error) {
	seg = strings.TrimRight(seg, "=")
	if d, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return d, nil
	}
	return base64.RawStdEncoding.DecodeString(seg)
}

func claimInt64(v any) int64 {
	switch t := v.(type) {
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
		if f, err := t.Float64(); err == nil {
			return int64(f)
		}
	case string:
		var n json.Number = json.Number(t)
		if i, err := n.Int64(); err == nil {
			return i
		}
	}
	return 0
}

func claimString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
