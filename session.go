package mezon

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Session is a user session authenticated with the Mezon server. It mirrors
// the Session class in src/session.ts: the token is a JWT whose payload carries
// the expiry and custom vars.
type Session struct {
	Token            string         `json:"token"`
	RefreshToken     string         `json:"refresh_token"`
	CreatedAt        int64          `json:"created_at"`
	ExpiresAt        int64          `json:"expires_at"`
	RefreshExpiresAt int64          `json:"refresh_expires_at"`
	UserID           string         `json:"user_id"`
	Vars             map[string]any `json:"vars"`
	APIURL           string         `json:"api_url"`
	IDToken          string         `json:"id_token"`
	WsURL            string         `json:"ws_url"`
}

type apiSessionRaw struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	APIURL       string `json:"api_url"`
	IDToken      string `json:"id_token"`
	WsURL        string `json:"ws_url"`
}

// NewSession builds a Session from an authenticate response, decoding the JWT
// to populate the expiry and vars (port of Session.constructor + update).
func NewSession(token, refreshToken, userID, apiURL, idToken, wsURL string) (*Session, error) {
	s := &Session{
		Token:        token,
		RefreshToken: refreshToken,
		UserID:       userID,
		CreatedAt:    time.Now().Unix(),
		APIURL:       apiURL,
		IDToken:      idToken,
		WsURL:        wsURL,
	}
	if err := s.update(token, refreshToken); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) update(token, refreshToken string) error {
	exp, vars, err := decodeJWT(token)
	if err != nil {
		return err
	}
	if refreshToken != "" {
		rexp, _, err := decodeJWT(refreshToken)
		if err != nil {
			return errors.New("refresh jwt is not valid")
		}
		s.RefreshExpiresAt = rexp
		s.RefreshToken = refreshToken
	}
	s.Token = token
	s.ExpiresAt = exp
	s.Vars = vars
	return nil
}

// IsExpired reports whether the session token has expired at the given unix time.
func (s *Session) IsExpired(now int64) bool { return s.ExpiresAt-now < 0 }

// IsRefreshExpired reports whether the refresh token has expired at the given unix time.
func (s *Session) IsRefreshExpired(now int64) bool { return s.RefreshExpiresAt-now < 0 }

func decodeJWT(token string) (exp int64, vars map[string]any, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, nil, errors.New("jwt is not valid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		// fall back to standard base64 (with padding) for tolerance
		payload, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return 0, nil, err
		}
	}
	var claims struct {
		Exp json.Number    `json:"exp"`
		Vrs map[string]any `json:"vrs"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, nil, err
	}
	e, _ := claims.Exp.Int64()
	return e, claims.Vrs, nil
}
