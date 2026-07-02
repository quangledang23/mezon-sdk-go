package mezon

import (
	"encoding/json"

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
	// The Messages cache falls back to the optional persistent MessageStore on a
	// miss, port of the TextChannel messages CacheManager fetcher which reads
	// from messageDB.getMessageById.
	c.Messages = NewCacheManager[string, *Message](c.loadMessageFromStore, 200)
	return c
}

// loadMessageFromStore loads a message from the client's MessageStore, port of
// the TextChannel.messages fetcher. It returns ErrNotFound when no store is
// configured or the message is absent.
func (c *TextChannel) loadMessageFromStore(messageID string) (*Message, error) {
	if c.Clan == nil || c.Clan.client == nil || c.Clan.client.messageDB == nil {
		return nil, ErrNotFound
	}
	clanID := c.Clan.ID
	cm, err := c.Clan.client.messageDB.GetMessageByID(messageID, c.ID, clanID)
	if err != nil || cm == nil {
		return nil, ErrNotFound
	}
	return newMessageFromChannelMessage(cm, c, c.socket, c.queue), nil
}

// Send sends a message to the channel, port of TextChannel.send. The content is
// JSON-serialized and length-validated in UTF-16 code units. It returns the
// created Message built from the send ack. opts may be nil.
func (c *TextChannel) Send(content Content, opts *SendOptions) (*Message, error) {
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
		ChannelType:      c.ChannelType,
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
	return Enqueue(c.queue, func() (*Message, error) {
		ack, err := c.socket.WriteChatMessage(data)
		if err != nil {
			return nil, err
		}
		return c.createMessageFromAck(ack, data), nil
	})
}

// createMessageFromAck builds and caches a Message from a send ack and the data
// used to send it, port of TextChannel.createMessageFromAck. The sender is the
// bot itself.
func (c *TextChannel) createMessageFromAck(ack *rtapi.ChannelMessageAck, data ReplyMessageData) *Message {
	channelID := data.ChannelID
	if ack != nil && ack.ChannelId != 0 {
		channelID = itoaID(ack.ChannelId)
	}
	senderID := ""
	if c.Clan != nil {
		senderID = c.Clan.ClientID
	}
	msg := &Message{
		ChannelID:   channelID,
		ClanID:      data.ClanID,
		SenderID:    senderID,
		Mentions:    data.Mentions,
		Attachments: data.Attachments,
		References:  data.References,
		TopicID:     data.TopicID,
		Mode:        data.Mode,
		Channel:     c,
		socket:      c.socket,
		queue:       c.queue,
	}
	if ack != nil {
		msg.ID = itoaID(ack.MessageId)
		msg.CreateTimeSeconds = ack.CreateTimeSeconds
		msg.UpdateTimeSeconds = ack.UpdateTimeSeconds
		msg.Code = int(ack.Code)
		msg.Username = ack.Username
		if ack.Persistent != nil {
			msg.Persistent = ack.Persistent.Value
		}
	}
	if raw, err := marshalContent(data.Content); err == nil {
		msg.Content = json.RawMessage(raw)
	}
	c.Messages.Set(msg.ID, msg)
	return msg
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
		ChannelType:      c.ChannelType,
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
