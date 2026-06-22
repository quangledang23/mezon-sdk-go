package mezon

import (
	"strconv"

	"github.com/quangledang23/mezon-sdk-go/api"
)

// Mention is a user/role mention with UTF-16 start/end offsets into the message
// text (S/E). The Mezon clients compute these offsets in UTF-16 code units; use
// MentionSpan to derive them from text so they line up across clients.
type Mention struct {
	UserID            string `json:"user_id,omitempty"`
	RoleID            string `json:"role_id,omitempty"`
	Username          string `json:"username,omitempty"`
	Rolename          string `json:"rolename,omitempty"`
	S                 int    `json:"s"`
	E                 int    `json:"e"`
	CreateTimeSeconds uint32 `json:"create_time_seconds,omitempty"`
}

// Attachment is a message attachment.
type Attachment struct {
	Filename  string `json:"filename,omitempty"`
	Size      int    `json:"size,omitempty"`
	URL       string `json:"url,omitempty"`
	Filetype  string `json:"filetype,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

// MessageRef references another message (used for replies/quotes).
type MessageRef struct {
	MessageID                string `json:"message_id,omitempty"`
	MessageRefID             string `json:"message_ref_id,omitempty"`
	Content                  string `json:"content,omitempty"`
	HasAttachment            bool   `json:"has_attachment,omitempty"`
	RefType                  int    `json:"ref_type,omitempty"`
	MessageSenderID          string `json:"message_sender_id,omitempty"`
	MessageSenderUsername    string `json:"message_sender_username,omitempty"`
	MessageSenderAvatar      string `json:"message_sender_avatar,omitempty"`
	MessageSenderClanNick    string `json:"message_sender_clan_nick,omitempty"`
	MessageSenderDisplayName string `json:"message_sender_display_name,omitempty"`
}

// Reaction is a message reaction.
type Reaction struct {
	ID              string `json:"id,omitempty"`
	EmojiID         string `json:"emoji_id,omitempty"`
	Emoji           string `json:"emoji,omitempty"`
	SenderID        string `json:"sender_id,omitempty"`
	SenderName      string `json:"sender_name,omitempty"`
	Action          bool   `json:"action,omitempty"`
	Count           int    `json:"count,omitempty"`
	MessageSenderID string `json:"message_sender_id,omitempty"`
}

// ReactPayload is the input to Message.React.
type ReactPayload struct {
	ID           string
	EmojiID      string
	Emoji        string
	Count        int
	ActionDelete bool
}

// --- conversions to the protobuf wire types used by the socket -------------

func atoiID(s string) int64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func itoaID(v int64) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatInt(v, 10)
}

func mentionsToProto(ms []Mention) []*api.MessageMention {
	if len(ms) == 0 {
		return nil
	}
	out := make([]*api.MessageMention, 0, len(ms))
	for _, m := range ms {
		out = append(out, &api.MessageMention{
			UserId:            atoiID(m.UserID),
			RoleId:            atoiID(m.RoleID),
			Username:          m.Username,
			Rolename:          m.Rolename,
			S:                 int32(m.S),
			E:                 int32(m.E),
			CreateTimeSeconds: m.CreateTimeSeconds,
		})
	}
	return out
}

func attachmentsToProto(as []Attachment) []*api.MessageAttachment {
	if len(as) == 0 {
		return nil
	}
	out := make([]*api.MessageAttachment, 0, len(as))
	for _, a := range as {
		out = append(out, &api.MessageAttachment{
			Filename:  a.Filename,
			Size:      int32(a.Size),
			Url:       a.URL,
			Filetype:  a.Filetype,
			Width:     int32(a.Width),
			Height:    int32(a.Height),
			Thumbnail: a.Thumbnail,
			Duration:  int32(a.Duration),
		})
	}
	return out
}

func refsToProto(rs []MessageRef) []*api.MessageRef {
	if len(rs) == 0 {
		return nil
	}
	out := make([]*api.MessageRef, 0, len(rs))
	for _, r := range rs {
		out = append(out, &api.MessageRef{
			MessageId:                atoiID(r.MessageID),
			MessageRefId:             atoiID(r.MessageRefID),
			Content:                  r.Content,
			HasAttachment:            r.HasAttachment,
			RefType:                  int32(r.RefType),
			MessageSenderId:          atoiID(r.MessageSenderID),
			MessageSenderUsername:    r.MessageSenderUsername,
			MessageSenderAvatar:      r.MessageSenderAvatar,
			MessageSenderClanNick:    r.MessageSenderClanNick,
			MessageSenderDisplayName: r.MessageSenderDisplayName,
		})
	}
	return out
}
