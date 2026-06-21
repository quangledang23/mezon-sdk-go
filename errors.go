package mezonlightsdk

import "fmt"

// AuthenticationError is returned when authentication fails.
type AuthenticationError struct {
	Message    string
	StatusCode int
}

func (e *AuthenticationError) Error() string {
	if e.Message == "" {
		return "Authentication failed."
	}
	return e.Message
}

// SessionError is returned when session-related operations fail.
type SessionError struct {
	Message string
}

func (e *SessionError) Error() string {
	if e.Message == "" {
		return "Session error."
	}
	return e.Message
}

// SocketError is returned when socket operations fail, including logical
// errors reported by the server inside a realtime envelope.
type SocketError struct {
	// Code is the server error code, if the error came from the server.
	Code int32
	// Message describes the failure.
	Message string
	// Context holds additional error details reported by the server.
	Context map[string]string
}

func (e *SocketError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("socket error (code %d): %s", e.Code, e.Message)
	}
	return e.Message
}

// APIError is returned for non-2xx HTTP responses from the Mezon API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mezon api error: status %d: %s", e.StatusCode, e.Body)
}
