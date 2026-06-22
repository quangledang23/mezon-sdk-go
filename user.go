package mezon

import (
	"fmt"

	"github.com/quangledang23/mezon-sdk-go/rtapi"
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
// message TypeMessage code (use 0 for a normal chat message).
func (u *User) SendDM(content Content, code int, attachments []Attachment) (*rtapi.ChannelMessageAck, error) {
	return Enqueue(u.queue, func() (*rtapi.ChannelMessageAck, error) {
		if u.DmChannelID == "" {
			ch, err := u.channelManager.CreateDMChannel(u.ID)
			if err == nil && ch != nil {
				u.DmChannelID = itoaID(ch.ChannelId)
			}
		}
		if u.DmChannelID == "" {
			return nil, fmt.Errorf("can not get dmChannelId for user %s", u.ID)
		}
		data := ReplyMessageData{
			ClanID:      "0",
			ChannelID:   u.DmChannelID,
			Mode:        int(StreamModeDM),
			IsPublic:    false,
			Content:     content,
			Attachments: attachments,
			Code:        code,
		}
		return u.socket.WriteChatMessage(data)
	})
}
