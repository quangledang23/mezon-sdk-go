package redisstore

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestStoreRoundTrip(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	st := New(rdb, WithPrefix("t:"))
	ctx := context.Background()

	// miss
	if _, ok, err := st.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}

	// set + get
	if err := st.Set(ctx, "k", []byte("hello"), time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	b, ok, err := st.Get(ctx, "k")
	if err != nil || !ok || string(b) != "hello" {
		t.Fatalf("get = (%q, %v, %v), want (hello, true, nil)", b, ok, err)
	}

	// prefix is applied on the wire
	if !mr.Exists("t:k") {
		t.Errorf("key should be stored under prefixed name t:k")
	}

	// ttl was set
	if ttl := mr.TTL("t:k"); ttl <= 0 {
		t.Errorf("ttl = %v, want > 0", ttl)
	}

	// delete
	if err := st.Delete(ctx, "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := st.Get(ctx, "k"); ok {
		t.Errorf("key should be gone after delete")
	}
}
