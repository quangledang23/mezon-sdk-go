package mezon

import "github.com/quangledang23/mezon-sdk-go/rtapi"

// bindInternalListeners wires the cache-maintenance handlers, port of
// MezonClient._setupInternalListeners. These run before user handlers.
func (c *MezonClient) bindInternalListeners() {
	if c.internalsBound {
		return
	}
	c.internalsBound = true

	c.events.on(EventChannelMessage, func(p any) {
		if m, ok := p.(*ChannelMessage); ok {
			c.cacheChannelMessage(m)
			c.cacheUserFromMessage(m)
		}
	})
	c.events.on(EventChannelDeleted, func(p any) {
		if e, ok := p.(*rtapi.ChannelDeletedEvent); ok {
			channelID := itoaID(e.ChannelId)
			c.Channels.Delete(channelID)
			if clan, ok := c.Clans.Get(itoaID(e.ClanId)); ok {
				clan.Channels.Delete(channelID)
			}
		}
	})
}

// cacheChannelMessage stores an inbound message in its channel cache, port of
// _initChannelMessageCache (best-effort; errors are swallowed like the TS).
func (c *MezonClient) cacheChannelMessage(e *ChannelMessage) {
	if e.ClanID != "" && e.ClanID != "0" {
		if clan, ok := c.Clans.Get(e.ClanID); ok {
			_ = clan.LoadChannels()
		}
	}
	channel, err := c.Channels.Fetch(e.ChannelID)
	if err != nil || channel == nil || e.MessageID == "" {
		return
	}
	msg := newMessageFromChannelMessage(e, channel, c.socket, c.queue)
	channel.Messages.Set(e.MessageID, msg)
}

// cacheUserFromMessage ensures the sender is cached, port of _initUserClanCache.
func (c *MezonClient) cacheUserFromMessage(e *ChannelMessage) {
	if e.SenderID == "" {
		return
	}
	dmID := c.channelManager.GetAllDMChannels()[e.SenderID]
	if u, ok := c.Users.Get(e.SenderID); ok {
		if e.Username != "" {
			u.Username = e.Username
		}
		if e.ClanNick != "" {
			u.ClanNick = e.ClanNick
		}
		if e.ClanAvatar != "" {
			u.ClanAvatar = e.ClanAvatar
		}
		if e.DisplayName != "" {
			u.DisplayName = e.DisplayName
		}
		if e.Avatar != "" {
			u.Avatar = e.Avatar
		}
		if dmID != "" {
			u.DmChannelID = dmID
		}
		return
	}
	c.Users.Set(e.SenderID, &User{
		ID:             e.SenderID,
		Username:       e.Username,
		ClanNick:       e.ClanNick,
		ClanAvatar:     e.ClanAvatar,
		DisplayName:    e.DisplayName,
		Avatar:         e.Avatar,
		DmChannelID:    dmID,
		socket:         c.socket,
		queue:          c.queue,
		channelManager: c.channelManager,
	})
}
