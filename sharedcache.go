package mezon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/quangledang23/mezon-sdk-go/api"
	"google.golang.org/protobuf/proto"
)

// SharedStore is an optional L2 cache backend that can be shared across bot
// instances (e.g. backed by Redis). It stores opaque bytes keyed by string;
// the SDK serializes only the data needed to rebuild a channel/user, never the
// live objects (which hold per-process socket/queue references). Implementations
// must be safe for concurrent use. A miss returns (nil, false, nil); transient
// backend errors are returned and treated as a miss by the SDK.
type SharedStore interface {
	Get(ctx context.Context, key string) (value []byte, ok bool, err error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

const (
	defaultCacheTTL = 5 * time.Minute
	// defaultMaxCache bounds the in-memory Users/Channels caches so a
	// long-running bot does not accumulate every channel/sender forever.
	defaultMaxCache = 5000

	l2ChannelPrefix = "mezon:ch:"
	l2UserPrefix    = "mezon:user:"
)

// l2Get reads a value from the shared store, treating a nil store or any error
// as a miss (errors are logged). Bounded by the client timeout.
func (c *MezonClient) l2Get(key string) ([]byte, bool) {
	if c.store == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	b, ok, err := c.store.Get(ctx, key)
	if err != nil {
		log.Printf("mezon: shared cache get %q failed: %v", key, err)
		return nil, false
	}
	return b, ok
}

// l2Set best-effort writes a value to the shared store with the configured TTL.
func (c *MezonClient) l2Set(key string, value []byte) {
	if c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	if err := c.store.Set(ctx, key, value, c.cacheTTL); err != nil {
		log.Printf("mezon: shared cache set %q failed: %v", key, err)
	}
}

// l2Delete best-effort removes a key from the shared store.
func (c *MezonClient) l2Delete(key string) {
	if c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	if err := c.store.Delete(ctx, key); err != nil {
		log.Printf("mezon: shared cache delete %q failed: %v", key, err)
	}
}

// userDTO is the serializable slice of a User kept in the shared store. The
// expensive part to recompute is the DM channel id (a REST CreateDMChannel
// call); the live socket/queue/manager refs are re-injected on read, and
// presence fields are re-enriched locally from inbound messages.
type userDTO struct {
	ID          string `json:"id"`
	DmChannelID string `json:"dm_channel_id"`
}

// fetchChannel loads a channel: in-memory L1 (handled by CacheManager) misses
// land here, which consult the shared L2 store, then fall back to the REST API
// and populate L2.
func (c *MezonClient) fetchChannel(id string) (*TextChannel, error) {
	if c.session == nil {
		return nil, ErrNotFound
	}
	if b, ok := c.l2Get(l2ChannelPrefix + id); ok {
		var detail api.ChannelDescription
		if err := proto.Unmarshal(b, &detail); err == nil {
			return c.buildChannel(&detail), nil
		}
	}
	detail, err := c.apiClient.ListChannelDetail(c.session.Token, id)
	if err != nil {
		return nil, err
	}
	// Guard against the server returning an empty/zero detail, port of the
	// invalid-channel-detail check in MezonClientCore.fetchChannel.
	if detail == nil || detail.ChannelId == 0 {
		return nil, fmt.Errorf("invalid channel detail response for %s", id)
	}
	if b, err := proto.Marshal(detail); err == nil {
		c.l2Set(l2ChannelPrefix+id, b)
	}
	return c.buildChannel(detail), nil
}

// cacheDmChannel caches a DM channel description as a live TextChannel under the
// DM pseudo-clan, port of MezonClientCore._cacheDmChannel.
func (c *MezonClient) cacheDmChannel(desc *api.ChannelDescription) *TextChannel {
	if desc == nil || desc.ChannelId == 0 {
		return nil
	}
	return c.buildChannel(desc)
}

// initDmChannelCache pre-caches all discovered DM channels, port of
// MezonClientCore._initDmChannelCache.
func (c *MezonClient) initDmChannelCache() {
	if c.channelManager == nil {
		return
	}
	for _, desc := range c.channelManager.GetAllDMChannelDescs() {
		c.cacheDmChannel(desc)
	}
}

// buildChannel rebuilds a live TextChannel from channel detail, wiring it to the
// owning clan (creating the pseudo-clan when needed so DM channels with
// clan_id "0" still have a non-nil Clan) and the per-process socket/queue.
func (c *MezonClient) buildChannel(detail *api.ChannelDescription) *TextChannel {
	clanID := itoaID(detail.ClanId)
	clan, ok := c.Clans.Get(clanID)
	if !ok || clan == nil {
		token := ""
		if c.session != nil {
			token = c.session.Token
		}
		clan = newClan(clanID, "unknown", "", "", c, token)
		c.Clans.Set(clanID, clan)
	}
	return c.upsertChannel(clan, detail)
}

// upsertChannel merges a channel description into the caches: an already
// cached TextChannel is updated in place — so its Messages cache and any held
// references survive (e.g. across a reconnect) — and a new one is created
// otherwise; either way it is (re)registered in both the clan and client
// caches. Port of the existing-channel reuse added to Clan.fetchAndMergeChannels
// and MezonClientCore._cacheDmChannel in mezon-js PR #1129.
func (c *MezonClient) upsertChannel(clan *Clan, desc *api.ChannelDescription) *TextChannel {
	id := itoaID(desc.ChannelId)
	channel, ok := clan.Channels.Get(id)
	if !ok {
		channel, ok = c.Channels.Get(id)
	}
	if ok {
		channel.updateFromDesc(desc)
	} else {
		channel = newTextChannel(desc, clan, c.socket, c.queue)
	}
	clan.Channels.Set(id, channel)
	c.Channels.Set(id, channel)
	return channel
}

// fetchUser loads a user: consult L2, then create/lookup the DM channel via REST
// and populate L2.
func (c *MezonClient) fetchUser(id string) (*User, error) {
	if c.session == nil {
		return nil, ErrNotFound
	}
	if b, ok := c.l2Get(l2UserPrefix + id); ok {
		var dto userDTO
		if err := json.Unmarshal(b, &dto); err == nil && dto.DmChannelID != "" {
			u := c.buildUser(dto)
			c.Users.Set(id, u)
			return u, nil
		}
	}
	dm, err := c.channelManager.CreateDMChannel(id)
	if err != nil || dm == nil {
		return nil, ErrNotFound
	}
	dto := userDTO{ID: id, DmChannelID: itoaID(dm.ChannelId)}
	if b, err := json.Marshal(dto); err == nil {
		c.l2Set(l2UserPrefix+id, b)
	}
	u := c.buildUser(dto)
	c.Users.Set(id, u)
	return u, nil
}

// buildUser rebuilds a live User from its DTO, injecting the per-process refs.
func (c *MezonClient) buildUser(dto userDTO) *User {
	return &User{
		ID:             dto.ID,
		DmChannelID:    dto.DmChannelID,
		socket:         c.socket,
		queue:          c.queue,
		channelManager: c.channelManager,
	}
}
