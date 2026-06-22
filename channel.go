package mezon

import (
	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

// TextChannel is a chat channel, port of src/mezon-client/structures/TextChannel.ts.
type TextChannel struct {
	ID           string
	Name         string
	IsPrivate    bool
	ChannelType  int
	CategoryID   string
	CategoryName string
	ParentID     string
	MeetingCode  string
	Clan         *Clan
	Messages     *CacheManager[string, *Message]

	socket *DefaultSocket
	queue  *AsyncThrottleQueue
}

// SendOptions carries the optional parameters for sending a message.
type SendOptions struct {
	Mentions        []Mention
	Attachments     []Attachment
	References      []MessageRef
	MentionEveryone bool
	Anonymous       bool
	TopicID         string
	Code            int
}

func newTextChannel(d *api.ChannelDescription, clan *Clan, socket *DefaultSocket, queue *AsyncThrottleQueue) *TextChannel {
	c := &TextChannel{
		ID:           itoaID(d.ChannelId),
		Name:         d.ChannelLabel,
		ChannelType:  int(d.Type),
		IsPrivate:    d.ChannelPrivate != 0,
		CategoryID:   itoaID(d.CategoryId),
		CategoryName: d.CategoryName,
		ParentID:     itoaID(d.ParentId),
		MeetingCode:  d.MeetingCode,
		Clan:         clan,
		socket:       socket,
		queue:        queue,
	}
	c.Messages = NewCacheManager[string, *Message](nil, 200)
	return c
}

// Send sends a message to the channel, port of TextChannel.send. The content is
// JSON-serialized and length-validated in UTF-16 code units. opts may be nil.
func (c *TextChannel) Send(content Content, opts *SendOptions) (*rtapi.ChannelMessageAck, error) {
	if opts == nil {
		opts = &SendOptions{}
	}
	clanID := ""
	if c.Clan != nil {
		clanID = c.Clan.ID
	}
	data := ReplyMessageData{
		ClanID:           clanID,
		ChannelID:        c.ID,
		Mode:             int(ConvertChannelTypeToChannelMode(c.ChannelType)),
		IsPublic:         !c.IsPrivate,
		Content:          content,
		Mentions:         opts.Mentions,
		Attachments:      opts.Attachments,
		References:       opts.References,
		AnonymousMessage: opts.Anonymous,
		MentionEveryone:  opts.MentionEveryone,
		Code:             opts.Code,
		TopicID:          opts.TopicID,
	}
	return Enqueue(c.queue, func() (*rtapi.ChannelMessageAck, error) {
		return c.socket.WriteChatMessage(data)
	})
}

// SendEphemeral sends an ephemeral message to the given receivers, port of
// TextChannel.sendEphemeral (without the reply-reference lookup).
func (c *TextChannel) SendEphemeral(receiverIDs []string, content Content, opts *SendOptions) (*api.ChannelMessage, error) {
	if opts == nil {
		opts = &SendOptions{}
	}
	clanID := ""
	if c.Clan != nil {
		clanID = c.Clan.ID
	}
	topic := opts.TopicID
	if topic == "" {
		topic = "0"
	}
	data := EphemeralMessageData{
		ReceiverIDs:      receiverIDs,
		ClanID:           clanID,
		ChannelID:        c.ID,
		Mode:             int(ConvertChannelTypeToChannelMode(c.ChannelType)),
		IsPublic:         !c.IsPrivate,
		Content:          content,
		Mentions:         opts.Mentions,
		Attachments:      opts.Attachments,
		References:       opts.References,
		AnonymousMessage: opts.Anonymous,
		MentionEveryone:  opts.MentionEveryone,
		Code:             int(TypeMessageEphemeral),
		TopicID:          topic,
	}
	return Enqueue(c.queue, func() (*api.ChannelMessage, error) {
		return c.socket.WriteEphemeralMessage(data)
	})
}
