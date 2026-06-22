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
