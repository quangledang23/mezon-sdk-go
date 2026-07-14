package mezon

import (
	"fmt"
	"sync"

	"github.com/quangledang23/mezon-sdk-go/api"
)

// Clan is a Mezon clan (server/guild), port of
// src/mezon-client/structures/Clan.ts.
type Clan struct {
	ID               string
	Name             string
	WelcomeChannelID string
	ClanName         string
	Channels         *CacheManager[string, *TextChannel]
	SessionToken     string
	ClientID         string

	apiClient *MezonApi
	socket    *DefaultSocket
	queue     *AsyncThrottleQueue
	client    *MezonClient

	loadMu         sync.Mutex
	channelsLoaded bool
}

func newClan(id, name, welcomeChannelID, clanName string, client *MezonClient, sessionToken string) *Clan {
	c := &Clan{
		ID:               id,
		Name:             name,
		WelcomeChannelID: welcomeChannelID,
		ClanName:         clanName,
		SessionToken:     sessionToken,
		ClientID:         client.ClientID,
		apiClient:        client.apiClient,
		socket:           client.socket,
		queue:            client.queue,
		client:           client,
	}
	c.Channels = NewCacheManager[string, *TextChannel](func(channelID string) (*TextChannel, error) {
		return client.Channels.Fetch(channelID)
	}, 0)
	return c
}

// LoadChannels loads the clan's text channels once, port of Clan.loadChannels.
func (c *Clan) LoadChannels() error {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	if c.channelsLoaded {
		return nil
	}
	channels, err := c.apiClient.ListChannelDescs(c.SessionToken, int32(ChannelTypeChannel), c.ID, 0, 0, "", false)
	if err != nil {
		return err
	}
	for _, ch := range channels.GetChanneldesc() {
		if ch.ChannelId == 0 {
			continue
		}
		channelObj := newTextChannel(ch, c, c.socket, c.queue)
		c.Channels.Set(channelObj.ID, channelObj)
		c.client.Channels.Set(channelObj.ID, channelObj)
	}
	c.channelsLoaded = true
	return nil
}

// CreateChannelData configures Clan.CreateChannel.
type CreateChannelData struct {
	Label string
	// Type is one of the ChannelType* constants; 0 => ChannelTypeChannel.
	Type    int
	Private bool
	// CategoryID places the channel in a category; "" => the clan default.
	CategoryID string
	// UserIDs invites members, typically for private channels.
	UserIDs []string
	// ParentID makes the channel a thread under the given channel.
	ParentID string
}

// CreateChannel creates a channel in the clan and caches it as a live
// TextChannel. Bulk creation is one call per channel — the server has no
// batch endpoint.
func (c *Clan) CreateChannel(d CreateChannelData) (*api.ChannelDescription, error) {
	if d.Label == "" {
		return nil, fmt.Errorf("mezon: channel label is required")
	}
	channelType := d.Type
	if channelType == 0 {
		channelType = int(ChannelTypeChannel)
	}
	private := int32(0)
	if d.Private {
		private = 1
	}
	userIDs := make([]int64, 0, len(d.UserIDs))
	for _, id := range d.UserIDs {
		userIDs = append(userIDs, atoiID(id))
	}
	ch, err := c.apiClient.CreateChannelDesc(c.SessionToken, &api.CreateChannelDescRequest{
		ClanId:         atoiID(c.ID),
		ParentId:       atoiID(d.ParentID),
		CategoryId:     atoiID(d.CategoryID),
		Type:           int32(channelType),
		ChannelLabel:   d.Label,
		ChannelPrivate: private,
		UserIds:        userIDs,
	})
	if err != nil || ch == nil {
		return nil, err
	}
	channelObj := newTextChannel(ch, c, c.socket, c.queue)
	c.Channels.Set(channelObj.ID, channelObj)
	c.client.Channels.Set(channelObj.ID, channelObj)
	return ch, nil
}

// DeleteChannel deletes a channel from the clan and evicts it from the caches.
func (c *Clan) DeleteChannel(channelID string) error {
	if err := c.apiClient.DeleteChannelDesc(c.SessionToken, c.ID, channelID); err != nil {
		return err
	}
	c.Channels.Delete(channelID)
	c.client.Channels.Delete(channelID)
	return nil
}

// ListRoles lists clan roles, port of Clan.listRoles.
func (c *Clan) ListRoles(limit, state int32, cursor string) (*api.RoleListEventResponse, error) {
	return c.apiClient.ListRoles(c.SessionToken, c.ID, limit, state, cursor)
}

// UpdateRole updates a clan role, port of Clan.updateRole.
func (c *Clan) UpdateRole(req *api.UpdateRoleRequest) error {
	return c.apiClient.UpdateRole(c.SessionToken, req)
}

// ListChannelVoiceUsers lists users in the clan's voice channels, port of
// Clan.listChannelVoiceUsers.
func (c *Clan) ListChannelVoiceUsers(limit int32) (*api.VoiceChannelUserList, error) {
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("0 < limit <= 500")
	}
	return c.apiClient.ListChannelVoiceUsers(c.SessionToken, c.ID, limit)
}
