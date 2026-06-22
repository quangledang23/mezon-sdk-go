package mezon

import "sync"

// Handler receives an emitted event payload. Callers type-assert payload to the
// concrete event type documented on each MezonClient.On* method.
type Handler func(payload any)

// emitter is a minimal event emitter (port of the EventEmitter usage in the
// TS client). Handlers for an event run synchronously in registration order.
type emitter struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

func newEmitter() *emitter {
	return &emitter{handlers: make(map[string][]Handler)}
}

func (e *emitter) on(event string, h Handler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[event] = append(e.handlers[event], h)
}

func (e *emitter) emit(event string, payload any) {
	e.mu.RLock()
	hs := e.handlers[event]
	e.mu.RUnlock()
	for _, h := range hs {
		h(payload)
	}
}
