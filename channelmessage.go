package mezon

import (
	"encoding/json"

	"github.com/quangledang23/mezon-sdk-go/api"
)

// ChannelMessage is an inbound chat message, port of the ChannelMessage
// interface in src/socket.ts. Content holds the raw JSON content string; the
// reactions/mentions/attachments/references fields arrive as JSON blobs on the
// wire and are parsed here (mirroring CreateChannelMessageFromEvent).
type ChannelMessage struct {
	ID                string          `json:"id"`
	ChannelID         string          `json:"channel_id"`
	ClanID            string          `json:"clan_id"`
	Code              int32           `json:"code"`
	SenderID          string          `json:"sender_id"`
	Username          string          `json:"username"`
	Avatar            string          `json:"avatar"`
	Content           json.RawMessage `json:"content"`
	Reactions         []Reaction      `json:"reactions"`
	Mentions          []Mention       `json:"mentions"`
	Attachments       []Attachment    `json:"attachments"`
	References        []MessageRef    `json:"references"`
	ReferencedMessage json.RawMessage `json:"referenced_message"`
	MessageID         string          `json:"message_id"`
	ChannelLabel      string          `json:"channel_label"`
	ClanLogo          string          `json:"clan_logo"`
	CategoryName      string          `json:"category_name"`
	DisplayName       string          `json:"display_name"`
	ClanNick          string          `json:"clan_nick"`
	ClanAvatar        string          `json:"clan_avatar"`
	Mode              int32           `json:"mode"`
	HideEditted       bool            `json:"hide_editted"`
	IsPublic          bool            `json:"is_public"`
	TopicID           string          `json:"topic_id"`
	CreateTimeSeconds uint32          `json:"create_time_seconds"`
	UpdateTimeSeconds uint32          `json:"update_time_seconds"`
}

// ContentText returns the "t" text field of the content, if present.
func (m *ChannelMessage) ContentText() string {
	if len(m.Content) == 0 {
		return ""
	}
	var c struct {
		T string `json:"t"`
	}
	_ = json.Unmarshal(m.Content, &c)
	return c.T
}

// channelMessageFromProto converts an inbound api.ChannelMessage protobuf into
// the friendly ChannelMessage, parsing the JSON byte blobs for
// reactions/mentions/attachments/references.
func channelMessageFromProto(cm *api.ChannelMessage) *ChannelMessage {
	if cm == nil {
		return &ChannelMessage{}
	}
	out := &ChannelMessage{
		ID:                itoaID(cm.MessageId),
		ChannelID:         itoaID(cm.ChannelId),
		ClanID:            itoaID(cm.ClanId),
		Code:              cm.Code,
		SenderID:          itoaID(cm.SenderId),
		Username:          cm.Username,
		Avatar:            cm.Avatar,
		MessageID:         itoaID(cm.MessageId),
		ChannelLabel:      cm.ChannelLabel,
		ClanLogo:          cm.ClanLogo,
		CategoryName:      cm.CategoryName,
		DisplayName:       cm.DisplayName,
		ClanNick:          cm.ClanNick,
		ClanAvatar:        cm.ClanAvatar,
		Mode:              cm.Mode,
		HideEditted:       cm.HideEditted,
		IsPublic:          cm.IsPublic,
		TopicID:           itoaID(cm.TopicId),
		CreateTimeSeconds: cm.CreateTimeSeconds,
		UpdateTimeSeconds: cm.UpdateTimeSeconds,
	}
	if cm.Content != "" {
		out.Content = json.RawMessage(cm.Content)
	}
	out.Mentions = decodeMentions(cm.Mentions)
	out.Attachments = decodeAttachments(cm.Attachments)
	out.References = decodeRefs(cm.References)
	out.Reactions = decodeReactions(cm.Reactions)
	if len(cm.ReferencedMessage) > 0 {
		out.ReferencedMessage = json.RawMessage(cm.ReferencedMessage)
	}
	return out
}

// looksLikeJSON reports whether the blob starts with '[' or '{', the same
// heuristic the TS decode* helpers use to choose JSON vs protobuf.
func looksLikeJSON(b []byte) bool {
	return len(b) > 0 && (b[0] == '[' || b[0] == '{')
}

// decodeMentions parses the mentions byte blob, which the server may send as
// JSON or as a protobuf MessageMentionList (port of decodeMentions in utils.ts).
func decodeMentions(b []byte) []Mention {
	if len(b) == 0 {
		return nil
	}
	if looksLikeJSON(b) {
		var out []Mention
		_ = json.Unmarshal(b, &out)
		return out
	}
	var list api.MessageMentionList
	if err := protoUnmarshal(b, &list); err != nil {
		return nil
	}
	out := make([]Mention, 0, len(list.Mentions))
	for _, m := range list.Mentions {
		out = append(out, Mention{
			UserID:            itoaID(m.UserId),
			RoleID:            itoaID(m.RoleId),
			Username:          m.Username,
			Rolename:          m.Rolename,
			S:                 int(m.S),
			E:                 int(m.E),
			CreateTimeSeconds: m.CreateTimeSeconds,
		})
	}
	return out
}

func decodeAttachments(b []byte) []Attachment {
	if len(b) == 0 {
		return nil
	}
	if looksLikeJSON(b) {
		var out []Attachment
		_ = json.Unmarshal(b, &out)
		return out
	}
	var list api.MessageAttachmentList
	if err := protoUnmarshal(b, &list); err != nil {
		return nil
	}
	out := make([]Attachment, 0, len(list.Attachments))
	for _, a := range list.Attachments {
		out = append(out, Attachment{
			Filename:  a.Filename,
			Size:      int(a.Size),
			URL:       a.Url,
			Filetype:  a.Filetype,
			Width:     int(a.Width),
			Height:    int(a.Height),
			Thumbnail: a.Thumbnail,
			Duration:  int(a.Duration),
		})
	}
	return out
}

func decodeRefs(b []byte) []MessageRef {
	if len(b) == 0 {
		return nil
	}
	if looksLikeJSON(b) {
		var out []MessageRef
		_ = json.Unmarshal(b, &out)
		return out
	}
	var list api.MessageRefList
	if err := protoUnmarshal(b, &list); err != nil {
		return nil
	}
	out := make([]MessageRef, 0, len(list.Refs))
	for _, r := range list.Refs {
		out = append(out, MessageRef{
			MessageID:                itoaID(r.MessageId),
			MessageRefID:             itoaID(r.MessageRefId),
			Content:                  r.Content,
			HasAttachment:            r.HasAttachment,
			RefType:                  int(r.RefType),
			MessageSenderID:          itoaID(r.MessageSenderId),
			MessageSenderUsername:    r.MessageSenderUsername,
			MessageSenderAvatar:      r.MessageSenderAvatar,
			MessageSenderClanNick:    r.MessageSenderClanNick,
			MessageSenderDisplayName: r.MessageSenderDisplayName,
		})
	}
	return out
}

func decodeReactions(b []byte) []Reaction {
	if len(b) == 0 {
		return nil
	}
	if looksLikeJSON(b) {
		var out []Reaction
		_ = json.Unmarshal(b, &out)
		return out
	}
	var list api.MessageReactionList
	if err := protoUnmarshal(b, &list); err != nil {
		return nil
	}
	out := make([]Reaction, 0, len(list.Reactions))
	for _, r := range list.Reactions {
		out = append(out, Reaction{
			ID:              itoaID(r.Id),
			EmojiID:         itoaID(r.EmojiId),
			Emoji:           r.Emoji,
			SenderID:        itoaID(r.SenderId),
			SenderName:      r.SenderName,
			Action:          r.Action,
			Count:           int(r.Count),
			MessageSenderID: itoaID(r.MessageSenderId),
		})
	}
	return out
}
