package mezon

import (
	"encoding/json"

	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

// Message is a cached chat message with reply/edit/react/delete actions, port
// of src/mezon-client/structures/Message.ts.
type Message struct {
	ID                string
	ClanID            string
	ChannelID         string
	SenderID          string
	Content           json.RawMessage
	Mentions          []Mention
	Attachments       []Attachment
	References        []MessageRef
	Reactions         []Reaction
	TopicID           string
	CreateTimeSeconds uint32
	UpdateTimeSeconds uint32
	Code              int
	Username          string
	CreateTime        string
	UpdateTime        string
	Persistent        bool
	Mode              int
	Channel           *TextChannel

	socket *DefaultSocket
	queue  *AsyncThrottleQueue
}

// MessageID returns the message id (alias of ID), port of the Message.message_id
// getter in Message.ts.
func (m *Message) MessageID() string { return m.ID }

func newMessageFromChannelMessage(cm *ChannelMessage, channel *TextChannel, socket *DefaultSocket, queue *AsyncThrottleQueue) *Message {
	return &Message{
		ID:                cm.MessageID,
		ClanID:            cm.ClanID,
		ChannelID:         cm.ChannelID,
		SenderID:          cm.SenderID,
		Content:           cm.Content,
		Mentions:          cm.Mentions,
		Attachments:       cm.Attachments,
		References:        cm.References,
		Reactions:         cm.Reactions,
		TopicID:           cm.TopicID,
		CreateTimeSeconds: cm.CreateTimeSeconds,
		UpdateTimeSeconds: cm.UpdateTimeSeconds,
		Code:              int(cm.Code),
		Username:          cm.Username,
		Mode:              int(cm.Mode),
		Channel:           channel,
		socket:            socket,
		queue:             queue,
	}
}

func (m *Message) mode() int {
	return int(ConvertChannelTypeToChannelMode(m.Channel.ChannelType))
}

// Reply replies to this message, port of Message.reply. It returns the created
// Message built from the send ack. opts may be nil.
func (m *Message) Reply(content Content, opts *SendOptions) (*Message, error) {
	if opts == nil {
		opts = &SendOptions{}
	}
	return Enqueue(m.queue, func() (*Message, error) {
		var senderUsername, senderAvatar string
		if client := m.Channel.Clan.client; client != nil {
			if user, err := client.Users.Fetch(m.SenderID); err == nil && user != nil {
				senderUsername = firstNonEmpty(user.ClanNick, user.DisplayName, user.Username)
				senderAvatar = firstNonEmpty(user.ClanAvatar, user.Avatar)
			}
		}
		refs := []MessageRef{{
			MessageRefID:          m.ID,
			MessageSenderID:       m.SenderID,
			MessageSenderUsername: senderUsername,
			MessageSenderAvatar:   senderAvatar,
			Content:               string(m.Content),
		}}
		topic := opts.TopicID
		if topic == "" {
			topic = m.TopicID
		}
		data := ReplyMessageData{
			ClanID:           m.Channel.Clan.ID,
			ChannelID:        m.Channel.ID,
			ChannelType:      m.Channel.ChannelType,
			Mode:             m.mode(),
			IsPublic:         !m.Channel.IsPrivate,
			Content:          content,
			Mentions:         opts.Mentions,
			Attachments:      opts.Attachments,
			References:       refs,
			AnonymousMessage: opts.Anonymous,
			MentionEveryone:  opts.MentionEveryone,
			Code:             opts.Code,
			TopicID:          topic,
		}
		ack, err := m.socket.WriteChatMessage(data)
		if err != nil {
			return nil, err
		}
		return m.Channel.createMessageFromAck(ack, data), nil
	})
}

// Update edits this message, port of Message.update. It mutates this Message
// with the new content/mentions/attachments, refreshes it in the channel cache,
// and returns it.
func (m *Message) Update(content Content, mentions []Mention, attachments []Attachment) (*Message, error) {
	return Enqueue(m.queue, func() (*Message, error) {
		topic := m.TopicID
		if topic == "" {
			topic = "0"
		}
		data := UpdateMessageData{
			ClanID:            m.Channel.Clan.ID,
			ChannelID:         m.Channel.ID,
			ChannelType:       m.Channel.ChannelType,
			Mode:              m.mode(),
			IsPublic:          !m.Channel.IsPrivate,
			MessageID:         m.ID,
			Content:           content,
			Mentions:          mentions,
			Attachments:       attachments,
			CreateTimeSeconds: m.CreateTimeSeconds,
			TopicID:           topic,
			IsUpdateMsgTopic:  m.TopicID != "",
		}
		if _, err := m.socket.UpdateChatMessage(data); err != nil {
			return nil, err
		}
		if raw, err := marshalContent(content); err == nil {
			m.Content = json.RawMessage(raw)
		}
		if mentions != nil {
			m.Mentions = mentions
		}
		if attachments != nil {
			m.Attachments = attachments
		}
		m.Channel.Messages.Set(m.ID, m)
		return m, nil
	})
}

// React adds or removes a reaction, port of Message.react.
func (m *Message) React(p ReactPayload) (*api.MessageReaction, error) {
	return Enqueue(m.queue, func() (*api.MessageReaction, error) {
		emojiID := p.EmojiID
		if emojiID == "" {
			emojiID = "0"
		}
		reactID := p.ID
		if reactID == "" {
			reactID = "0"
		}
		data := ReactMessageData{
			ID:              reactID,
			ClanID:          m.Channel.Clan.ID,
			ChannelID:       m.Channel.ID,
			ChannelType:     m.Channel.ChannelType,
			Mode:            m.mode(),
			IsPublic:        !m.Channel.IsPrivate,
			MessageID:       m.ID,
			EmojiID:         emojiID,
			Emoji:           p.Emoji,
			Count:           p.Count,
			MessageSenderID: m.SenderID,
			ActionDelete:    p.ActionDelete,
		}
		return m.socket.WriteMessageReaction(data)
	})
}

// Delete removes this message, port of Message.delete.
func (m *Message) Delete() (*rtapi.ChannelMessageAck, error) {
	return Enqueue(m.queue, func() (*rtapi.ChannelMessageAck, error) {
		topic := m.TopicID
		if topic == "" {
			topic = "0"
		}
		data := RemoveMessageData{
			ClanID:      m.Channel.Clan.ID,
			ChannelID:   m.Channel.ID,
			ChannelType: m.Channel.ChannelType,
			Mode:        m.mode(),
			IsPublic:    !m.Channel.IsPrivate,
			MessageID:   m.ID,
			TopicID:     topic,
		}
		return m.socket.RemoveChatMessage(data)
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
