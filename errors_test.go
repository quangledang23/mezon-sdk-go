package mezonlightsdk

import "testing"

func TestAuthenticationErrorError(t *testing.T) {
	if got := (&AuthenticationError{}).Error(); got != "Authentication failed." {
		t.Errorf("empty AuthenticationError = %q, want default message", got)
	}
	if got := (&AuthenticationError{Message: "bad token", StatusCode: 401}).Error(); got != "bad token" {
		t.Errorf("AuthenticationError = %q, want %q", got, "bad token")
	}
}

func TestSessionErrorError(t *testing.T) {
	if got := (&SessionError{}).Error(); got != "Session error." {
		t.Errorf("empty SessionError = %q, want default message", got)
	}
	if got := (&SessionError{Message: "expired"}).Error(); got != "expired" {
		t.Errorf("SessionError = %q, want %q", got, "expired")
	}
}

func TestSocketErrorError(t *testing.T) {
	if got := (&SocketError{Message: "closed"}).Error(); got != "closed" {
		t.Errorf("SocketError without code = %q, want %q", got, "closed")
	}
	got := (&SocketError{Code: 3, Message: "denied"}).Error()
	want := "socket error (code 3): denied"
	if got != want {
		t.Errorf("SocketError with code = %q, want %q", got, want)
	}
}

func TestAPIErrorError(t *testing.T) {
	got := (&APIError{StatusCode: 500, Body: "boom"}).Error()
	want := "mezon api error: status 500: boom"
	if got != want {
		t.Errorf("APIError = %q, want %q", got, want)
	}
}
