// Package redisstore provides a Redis-backed mezon.SharedStore, so multiple
// Mezon bot instances can share an L2 cache of channel/user lookups and avoid
// redundant REST calls. It is a separate Go module: importing it pulls in
// github.com/redis/go-redis/v9, while the core mezon-sdk-go module stays
// dependency-light.
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	client, _ := mezon.NewMezonClient(mezon.ClientConfig{
//	    BotID: id, Token: token,
//	    Store:    redisstore.New(rdb),
//	    CacheTTL: 10 * time.Minute,
//	})
package redisstore

import (
	"context"
	"time"

	mezon "github.com/quangledang23/mezon-sdk-go"
	"github.com/redis/go-redis/v9"
)

// Store adapts a go-redis client to mezon.SharedStore. It works with any
// redis.UniversalClient (standalone, cluster, sentinel, ring).
type Store struct {
	client redis.UniversalClient
	prefix string
}

// compile-time check that Store satisfies the SDK interface.
var _ mezon.SharedStore = (*Store)(nil)

// Option configures a Store.
type Option func(*Store)

// WithPrefix namespaces every key (useful when one Redis serves several bots).
func WithPrefix(prefix string) Option {
	return func(s *Store) { s.prefix = prefix }
}

// New wraps a go-redis client as a SharedStore.
func New(client redis.UniversalClient, opts ...Option) *Store {
	s := &Store{client: client}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Get returns the value for key, or (nil, false, nil) on a miss.
func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := s.client.Get(ctx, s.prefix+key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// Set stores value under key with the given TTL (no expiry when ttl <= 0).
func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl < 0 {
		ttl = 0
	}
	return s.client.Set(ctx, s.prefix+key, value, ttl).Err()
}

// Delete removes key.
func (s *Store) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, s.prefix+key).Err()
}
