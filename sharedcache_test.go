package mezon

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/quangledang23/mezon-sdk-go/api"
	"google.golang.org/protobuf/proto"
)

// memStore is a tiny in-memory SharedStore for tests.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: map[string][]byte{}} }

func (m *memStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *memStore) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *memStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// TestSharedStoreChannelHit verifies a channel served from the shared L2 store
// is rebuilt without touching the REST API (apiClient is nil here, so any REST
// call would panic).
func TestSharedStoreChannelHit(t *testing.T) {
	store := newMemStore()
	detail := &api.ChannelDescription{ChannelId: 5, ChannelLabel: "general", ClanId: 7}
	b, err := proto.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	_ = store.Set(context.Background(), l2ChannelPrefix+"5", b, time.Minute)

	c, err := NewMezonClient(ClientConfig{BotID: "1", Token: "t", Store: store})
	if err != nil {
		t.Fatalf("NewMezonClient: %v", err)
	}
	c.session = &Session{Token: "t"}

	ch, err := c.Channels.Fetch("5")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if ch.ID != "5" || ch.Name != "general" {
		t.Errorf("channel = {ID:%q Name:%q}, want {5 general}", ch.ID, ch.Name)
	}
	if ch.Clan == nil || ch.Clan.ID != "7" {
		t.Errorf("channel clan = %+v, want clan id 7", ch.Clan)
	}
}

// TestSharedStoreUserHit verifies a user served from the shared L2 store is
// rebuilt without creating a DM channel over REST.
func TestSharedStoreUserHit(t *testing.T) {
	store := newMemStore()
	b, _ := json.Marshal(userDTO{ID: "42", DmChannelID: "999"})
	_ = store.Set(context.Background(), l2UserPrefix+"42", b, time.Minute)

	c, err := NewMezonClient(ClientConfig{BotID: "1", Token: "t", Store: store})
	if err != nil {
		t.Fatalf("NewMezonClient: %v", err)
	}
	c.session = &Session{Token: "t"}

	u, err := c.Users.Fetch("42")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if u.ID != "42" || u.DmChannelID != "999" {
		t.Errorf("user = {ID:%q DM:%q}, want {42 999}", u.ID, u.DmChannelID)
	}
}

// TestSharedStorePopulatedOnDefault confirms the store is optional: with no
// Store configured the caches still work (in-memory only).
func TestNoStoreIsInMemoryOnly(t *testing.T) {
	c, err := NewMezonClient(ClientConfig{BotID: "1", Token: "t"})
	if err != nil {
		t.Fatalf("NewMezonClient: %v", err)
	}
	if c.store != nil {
		t.Errorf("store should be nil by default")
	}
	// l2 helpers must be no-ops (not panic) when store is nil.
	if _, ok := c.l2Get("x"); ok {
		t.Errorf("l2Get with nil store should miss")
	}
	c.l2Set("x", []byte("y"))
	c.l2Delete("x")
}
