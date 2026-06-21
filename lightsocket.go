package mezonlightsdk

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// SocketConnectOptions configures a LightSocket connection.
type SocketConnectOptions struct {
	// OnError is called for socket errors.
	OnError func(err error)
	// OnDisconnect is called when the socket disconnects.
	OnDisconnect func()
	// Verbose enables verbose logging.
	Verbose bool
}

// ChannelMessageHandler handles channel message events.
type ChannelMessageHandler func(message *ChannelMessage)

// waitForSocketReady waits for a socket to be in a ready state with
// exponential backoff.
func waitForSocketReady(socket *DefaultSocket, maxRetries int, initialDelay time.Duration) error {
	delay := initialDelay
	for retry := 0; retry < maxRetries; retry++ {
		if socket.IsOpen() {
			return nil
		}
		time.Sleep(delay)
		delay *= 2 // Exponential backoff
	}
	return &SocketError{Message: fmt.Sprintf("socket failed to connect after %d attempts", maxRetries)}
}

// LightSocket provides a simplified interface for Mezon real-time messaging.
//
//	socket := mezonlightsdk.NewLightSocket(client, client.Session())
//
//	err := socket.Connect(ctx, mezonlightsdk.SocketConnectOptions{
//		OnError:      func(err error) { log.Println("socket error:", err) },
//		OnDisconnect: func() { log.Println("disconnected") },
//	})
//
//	socket.OnChannelMessage(func(msg *mezonlightsdk.ChannelMessage) {
//		log.Println("received message:", msg.Content)
//	})
//
//	err = socket.JoinDMChannel(ctx, "channel-123")
//	err = socket.SendDM(ctx, mezonlightsdk.SendMessagePayload{ChannelID: "channel-123", Content: map[string]string{"t": "Hello!"}})
type LightSocket struct {
	client  *LightClient
	session *Session

	mu                sync.Mutex
	socket            *DefaultSocket
	isConnected       bool
	handlers          map[int]ChannelMessageHandler
	nextHandlerID     int
	errorHandler      func(err error)
	disconnectHandler func()
}

// NewLightSocket creates a LightSocket bound to a client and session.
func NewLightSocket(client *LightClient, session *Session) *LightSocket {
	return &LightSocket{
		client:   client,
		session:  session,
		handlers: make(map[int]ChannelMessageHandler),
	}
}

// IsConnected reports whether the socket is currently connected.
func (s *LightSocket) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isConnected
}

// Socket returns the underlying socket instance, or an error if Connect has
// not been called.
func (s *LightSocket) Socket() (*DefaultSocket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.socket == nil {
		return nil, &SocketError{Message: "socket is not connected, call Connect() first"}
	}
	return s.socket, nil
}

// Connect connects to the Mezon real-time server.
func (s *LightSocket) Connect(ctx context.Context, options SocketConnectOptions) error {
	s.mu.Lock()
	if s.isConnected {
		s.mu.Unlock()
		return &SocketError{Message: "socket is already connected, call Disconnect() first"}
	}
	s.errorHandler = options.OnError
	s.disconnectHandler = options.OnDisconnect

	socket := s.client.CreateSocket(options.Verbose)
	socket.OnError = func(err error) {
		s.mu.Lock()
		handler := s.errorHandler
		s.mu.Unlock()
		if handler != nil {
			handler(err)
		}
	}
	socket.OnDisconnect = func() {
		s.mu.Lock()
		s.isConnected = false
		handler := s.disconnectHandler
		s.mu.Unlock()
		if handler != nil {
			handler()
		}
	}
	socket.OnChannelMessage = func(message *ChannelMessage) {
		if message == nil {
			s.mu.Lock()
			handler := s.errorHandler
			s.mu.Unlock()
			if handler != nil {
				handler(&SocketError{Message: "received nil channel message"})
			}
			return
		}
		s.mu.Lock()
		handlers := make([]ChannelMessageHandler, 0, len(s.handlers))
		for _, h := range s.handlers {
			handlers = append(handlers, h)
		}
		s.mu.Unlock()
		for _, h := range handlers {
			dispatchMessage(h, message)
		}
	}
	s.socket = socket
	s.mu.Unlock()

	if err := socket.Connect(ctx, s.session, true, "0"); err != nil {
		return err
	}

	s.mu.Lock()
	s.isConnected = true
	s.mu.Unlock()
	return nil
}

// dispatchMessage invokes a handler, recovering panics so one handler cannot
// break the dispatch loop (mirroring the try/catch in the TypeScript SDK).
func dispatchMessage(handler ChannelMessageHandler, message *ChannelMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("mezonlightsdk: panic in message handler: %v", r)
		}
	}()
	handler(message)
}

// Disconnect disconnects from the Mezon server.
func (s *LightSocket) Disconnect() {
	s.mu.Lock()
	socket := s.socket
	s.socket = nil
	s.isConnected = false
	s.mu.Unlock()
	if socket != nil {
		socket.Disconnect(true)
	}
}

// SetChannelMessageHandler registers a handler for incoming channel
// messages. Multiple handlers can be registered and all receive messages.
func (s *LightSocket) SetChannelMessageHandler(handler ChannelMessageHandler) {
	s.OnChannelMessage(handler)
}

// OnChannelMessage registers a handler for incoming channel messages and
// returns a function that unsubscribes it.
func (s *LightSocket) OnChannelMessage(handler ChannelMessageHandler) func() {
	s.mu.Lock()
	id := s.nextHandlerID
	s.nextHandlerID++
	s.handlers[id] = handler
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		delete(s.handlers, id)
		s.mu.Unlock()
	}
}

// SetErrorHandler sets the error handler for socket errors.
func (s *LightSocket) SetErrorHandler(handler func(err error)) {
	s.mu.Lock()
	s.errorHandler = handler
	s.mu.Unlock()
}

// JoinDMChannel joins a DM channel to receive messages from it.
func (s *LightSocket) JoinDMChannel(ctx context.Context, channelID string) error {
	socket, err := s.Socket()
	if err != nil {
		return err
	}
	if err := waitForSocketReady(socket, SocketReadyMaxRetry, SocketReadyRetryDelay); err != nil {
		return err
	}
	_, err = socket.JoinChat(ctx, ClanDM, channelID, ChannelTypeDM, false)
	return err
}

// JoinGroupChannel joins a group channel to receive messages from it.
func (s *LightSocket) JoinGroupChannel(ctx context.Context, channelID string) error {
	socket, err := s.Socket()
	if err != nil {
		return err
	}
	if err := waitForSocketReady(socket, SocketReadyMaxRetry, SocketReadyRetryDelay); err != nil {
		return err
	}
	_, err = socket.JoinChat(ctx, ClanDM, channelID, ChannelTypeGroup, false)
	return err
}

// LeaveDMChannel leaves a DM channel.
func (s *LightSocket) LeaveDMChannel(ctx context.Context, channelID string) error {
	socket, err := s.Socket()
	if err != nil {
		return err
	}
	return socket.LeaveChat(ctx, ClanDM, channelID, ChannelTypeDM, false)
}

// LeaveGroupChannel leaves a group channel.
func (s *LightSocket) LeaveGroupChannel(ctx context.Context, channelID string) error {
	socket, err := s.Socket()
	if err != nil {
		return err
	}
	return socket.LeaveChat(ctx, ClanDM, channelID, ChannelTypeGroup, false)
}

// SendDM sends a direct message to a channel.
func (s *LightSocket) SendDM(ctx context.Context, payload SendMessagePayload) error {
	socket, err := s.Socket()
	if err != nil {
		return err
	}
	content := messageContent(payload.Content, payload.HideLink)
	_, err = socket.WriteChatMessage(ctx, ClanDM, payload.ChannelID, StreamModeDM, false, content, &ChatMessageOptions{
		Mentions:        payload.Mentions,
		Attachments:     payload.Attachments,
		MentionEveryone: payload.MentionEveryone,
		Code:            payload.Code,
	})
	return err
}

// SendGroup sends a message to a group channel.
func (s *LightSocket) SendGroup(ctx context.Context, payload SendMessagePayload) error {
	socket, err := s.Socket()
	if err != nil {
		return err
	}
	content := messageContent(payload.Content, payload.HideLink)
	_, err = socket.WriteChatMessage(ctx, ClanDM, payload.ChannelID, StreamModeGroup, false, content, &ChatMessageOptions{
		Mentions:        payload.Mentions,
		Attachments:     payload.Attachments,
		MentionEveryone: payload.MentionEveryone,
		Code:            payload.Code,
	})
	return err
}
