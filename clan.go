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
	return c.reloadChannelsLocked()
}

// ReloadChannels re-fetches the clan's channels and merges them into the
// caches even when they were already loaded, so a reconnect picks up channels
// created or changed while the socket was down, port of Clan.reloadChannels
// (mezon-js PR #1129).
func (c *Clan) ReloadChannels() error {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	return c.reloadChannelsLocked()
}

// reloadChannelsLocked fetches the channel list and merges it, updating
// existing TextChannel objects in place (preserving their Messages caches)
// and creating the rest, port of Clan.fetchAndMergeChannels. Callers hold
// loadMu.
func (c *Clan) reloadChannelsLocked() error {
	channels, err := c.apiClient.ListChannelDescs(c.SessionToken, int32(ChannelTypeChannel), c.ID, 0, 0, "", false)
	if err != nil {
		return err
	}
	for _, ch := range channels.GetChanneldesc() {
		if ch.ChannelId == 0 {
			continue
		}
		c.client.upsertChannel(c, ch)
	}
	c.channelsLoaded = true
	return nil
}

// updateFromDesc refreshes the clan's fields from a freshly listed clan
// description on reconnect instead of recreating the object, port of
// Clan.updateFromDesc (mezon-js PR #1129).
func (c *Clan) updateFromDesc(name, welcomeChannelID, sessionToken string) {
	if name != "" {
		c.Name = name
		c.ClanName = name
	}
	if welcomeChannelID != "" {
		c.WelcomeChannelID = welcomeChannelID
	}
	c.SessionToken = sessionToken
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

// CreateRole creates a role in this clan and returns it. A zero req.ClanId is
// filled in from the clan.
func (c *Clan) CreateRole(req *api.CreateRoleRequest) (*api.Role, error) {
	if req.ClanId == 0 {
		req.ClanId = atoiID(c.ID)
	}
	return c.apiClient.CreateRole(c.SessionToken, req)
}

// DeleteRole deletes a role from this clan.
func (c *Clan) DeleteRole(roleID string) error {
	return c.apiClient.DeleteRole(c.SessionToken, roleID, c.ID)
}

// UpdateChannelPrivate makes a channel private (private=true) or public.
// roleIDs and userIDs keep access when turning private; both may be nil.
func (c *Clan) UpdateChannelPrivate(channelID string, private bool, roleIDs, userIDs []string) error {
	req := &api.ChangeChannelPrivateRequest{
		ClanId:    atoiID(c.ID),
		ChannelId: atoiID(channelID),
	}
	if private {
		req.ChannelPrivate = 1
	}
	for _, id := range roleIDs {
		req.RoleIds = append(req.RoleIds, atoiID(id))
	}
	for _, id := range userIDs {
		req.UserIds = append(req.UserIds, atoiID(id))
	}
	return c.apiClient.UpdateChannelPrivate(c.SessionToken, req)
}

// AddRolesToChannel grants roles access to a private channel.
func (c *Clan) AddRolesToChannel(channelID string, roleIDs []string) error {
	req := &api.AddRoleChannelDescRequest{ChannelId: atoiID(channelID)}
	for _, id := range roleIDs {
		req.RoleIds = append(req.RoleIds, atoiID(id))
	}
	return c.apiClient.AddRolesChannelDesc(c.SessionToken, req)
}

// SetRoleChannelPermission sets per-channel permission overrides for a role
// (req.RoleId) or a user (req.UserId).
func (c *Clan) SetRoleChannelPermission(req *api.UpdateRoleChannelRequest) error {
	return c.apiClient.SetRoleChannelPermission(c.SessionToken, req)
}

// ListPermissions lists the permission definitions known to the server,
// for mapping slugs like "send-message" to permission ids.
func (c *Clan) ListPermissions() (*api.PermissionList, error) {
	return c.apiClient.GetListPermission(c.SessionToken)
}

// ListChannelVoiceUsers lists users in the clan's voice channels, port of
// Clan.listChannelVoiceUsers.
func (c *Clan) ListChannelVoiceUsers(limit int32) (*api.VoiceChannelUserList, error) {
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("0 < limit <= 500")
	}
	return c.apiClient.ListChannelVoiceUsers(c.SessionToken, c.ID, limit)
}
