package mezon

import (
	"fmt"
)

// User is a Mezon user with DM capability, port of
// src/mezon-client/structures/User.ts.
type User struct {
	ID          string
	Username    string
	ClanNick    string
	ClanAvatar  string
	DisplayName string
	Avatar      string
	DmChannelID string

	socket         *DefaultSocket
	queue          *AsyncThrottleQueue
	channelManager *ChannelManager
}

// SendDM sends a direct message to the user, port of User.sendDM. code is the
// message TypeMessage code (use 0 for a normal chat message). It returns the
// created Message built from the send ack.
func (u *User) SendDM(content Content, code int, attachments []Attachment) (*Message, error) {
	return Enqueue(u.queue, func() (*Message, error) {
		if u.DmChannelID == "" {
			ch, err := u.channelManager.CreateDMChannel(u.ID)
			if err == nil && ch != nil {
				u.DmChannelID = itoaID(ch.ChannelId)
				if u.channelManager.client != nil {
					u.channelManager.client.cacheDmChannel(ch)
				}
			}
		}
		if u.DmChannelID == "" {
			return nil, fmt.Errorf("can not get dmChannelId for user %s", u.ID)
		}
		data := ReplyMessageData{
			ClanID:      "0",
			ChannelID:   u.DmChannelID,
			ChannelType: int(ChannelTypeDM),
			Mode:        int(StreamModeDM),
			IsPublic:    false,
			Content:     content,
			Attachments: attachments,
			Code:        code,
		}
		ack, err := u.socket.WriteChatMessage(data)
		if err != nil {
			return nil, err
		}
		channel, err := u.channelManager.client.Channels.Fetch(u.DmChannelID)
		if err != nil || channel == nil {
			return nil, err
		}
		return channel.createMessageFromAck(ack, data), nil
	})
}
