package mezon

import "errors"

var (
	// ErrNotFound is returned when a cached entity cannot be found or fetched.
	ErrNotFound = errors.New("mezon: not found")
	// ErrSocketClosed is returned when sending on a closed socket.
	ErrSocketClosed = errors.New("mezon: socket connection has not been established yet")
	// ErrSendTimeout is returned when a socket request does not get a response in time.
	ErrSendTimeout = errors.New("mezon: timed out while waiting for a response")
)
