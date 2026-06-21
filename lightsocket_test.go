package mezonlightsdk

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitForSocketReady(t *testing.T) {
	t.Run("never ready", func(t *testing.T) {
		s := NewDefaultSocket("localhost", "0", false, false)
		err := waitForSocketReady(s, 3, time.Millisecond)
		var sockErr *SocketError
		if !errors.As(err, &sockErr) {
			t.Fatalf("error = %v, want *SocketError", err)
		}
	})

	t.Run("already ready", func(t *testing.T) {
		s := NewDefaultSocket("localhost", "0", false, false)
		s.state = StateConnected
		if err := waitForSocketReady(s, 3, time.Millisecond); err != nil {
			t.Fatalf("waitForSocketReady: %v", err)
		}
	})
}

func TestLightSocketBeforeConnect(t *testing.T) {
	ls := NewLightSocket(nil, nil)

	if ls.IsConnected() {
		t.Error("IsConnected() = true before Connect")
	}
	if _, err := ls.Socket(); err == nil {
		t.Error("Socket() before Connect expected error")
	}

	// context.Background instead of t.Context: go.mod targets Go 1.22.
	ctx := context.Background()
	var sockErr *SocketError
	if err := ls.JoinDMChannel(ctx, "123"); !errors.As(err, &sockErr) {
		t.Errorf("JoinDMChannel error = %v, want *SocketError", err)
	}
	if err := ls.LeaveDMChannel(ctx, "123"); !errors.As(err, &sockErr) {
		t.Errorf("LeaveDMChannel error = %v, want *SocketError", err)
	}
	if err := ls.SendDM(ctx, SendMessagePayload{ChannelID: "123"}); !errors.As(err, &sockErr) {
		t.Errorf("SendDM error = %v, want *SocketError", err)
	}
	if err := ls.SendGroup(ctx, SendMessagePayload{ChannelID: "123"}); !errors.As(err, &sockErr) {
		t.Errorf("SendGroup error = %v, want *SocketError", err)
	}

	// Disconnect without Connect must be a safe no-op.
	ls.Disconnect()
}

func TestLightSocketHandlerRegistration(t *testing.T) {
	ls := NewLightSocket(nil, nil)

	var first, second int
	unsubFirst := ls.OnChannelMessage(func(m *ChannelMessage) { first++ })
	ls.SetChannelMessageHandler(func(m *ChannelMessage) { second++ })

	dispatch := func() {
		ls.mu.Lock()
		handlers := make([]ChannelMessageHandler, 0, len(ls.handlers))
		for _, h := range ls.handlers {
			handlers = append(handlers, h)
		}
		ls.mu.Unlock()
		for _, h := range handlers {
			dispatchMessage(h, &ChannelMessage{ID: "1"})
		}
	}

	dispatch()
	if first != 1 || second != 1 {
		t.Fatalf("after first dispatch: first = %d, second = %d, want 1 and 1", first, second)
	}

	unsubFirst()
	dispatch()
	if first != 1 || second != 2 {
		t.Fatalf("after unsubscribe: first = %d, second = %d, want 1 and 2", first, second)
	}

	// Unsubscribing twice must be a safe no-op.
	unsubFirst()
}

func TestDispatchMessageRecoversPanic(t *testing.T) {
	// A panicking handler must not crash the dispatch loop.
	dispatchMessage(func(m *ChannelMessage) { panic("handler bug") }, &ChannelMessage{})
}

func TestLightSocketSetErrorHandler(t *testing.T) {
	ls := NewLightSocket(nil, nil)
	called := false
	ls.SetErrorHandler(func(err error) { called = true })

	ls.mu.Lock()
	handler := ls.errorHandler
	ls.mu.Unlock()
	if handler == nil {
		t.Fatal("error handler not stored")
	}
	handler(errors.New("x"))
	if !called {
		t.Error("stored handler is not the one provided")
	}
}
