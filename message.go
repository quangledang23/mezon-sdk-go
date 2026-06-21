package mezonlightsdk

import (
	"encoding/json"
	"strings"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

// ChannelMessage is a message received on a channel, with content, mentions
// and attachments already decoded (the wire-level counterpart is
// proto.ChannelMessage).
type ChannelMessage struct {
	ID                string                  `json:"id"`
	Avatar            string                  `json:"avatar,omitempty"`
	ChannelID         string                  `json:"channel_id"`
	ChannelLabel      string                  `json:"channel_label"`
	ClanID            string                  `json:"clan_id,omitempty"`
	Code              int32                   `json:"code"`
	Content           any                     `json:"content"`
	Mentions          []*ApiMessageMention    `json:"mentions,omitempty"`
	Attachments       []*ApiMessageAttachment `json:"attachments,omitempty"`
	SenderID          string                  `json:"sender_id"`
	ClanLogo          string                  `json:"clan_logo,omitempty"`
	CategoryName      string                  `json:"category_name,omitempty"`
	Username          string                  `json:"username,omitempty"`
	ClanNick          string                  `json:"clan_nick,omitempty"`
	ClanAvatar        string                  `json:"clan_avatar,omitempty"`
	DisplayName       string                  `json:"display_name,omitempty"`
	CreateTimeSeconds uint32                  `json:"create_time_seconds,omitempty"`
	UpdateTimeSeconds uint32                  `json:"update_time_seconds,omitempty"`
	Mode              int32                   `json:"mode,omitempty"`
	MessageID         string                  `json:"message_id,omitempty"`
	HideEditted       bool                    `json:"hide_editted,omitempty"`
	IsPublic          bool                    `json:"is_public,omitempty"`
	TopicID           string                  `json:"topic_id,omitempty"`
}

// newChannelMessageFromProto mirrors createChannelMessageFromEvent in the
// TypeScript SDK: it decodes content (JSON) plus mentions and attachments
// (JSON or protobuf list messages).
func newChannelMessageFromProto(pm *proto.ChannelMessage) *ChannelMessage {
	return &ChannelMessage{
		ID:                pm.MessageID,
		Avatar:            pm.Avatar,
		ChannelID:         pm.ChannelID,
		ChannelLabel:      pm.ChannelLabel,
		ClanID:            pm.ClanID,
		Code:              pm.Code,
		Content:           SafeJSONParse([]byte(pm.Content)),
		Mentions:          DecodeMentions(pm.Mentions),
		Attachments:       DecodeAttachments(pm.Attachments),
		SenderID:          pm.SenderID,
		ClanLogo:          pm.ClanLogo,
		CategoryName:      pm.CategoryName,
		Username:          pm.Username,
		ClanNick:          pm.ClanNick,
		ClanAvatar:        pm.ClanAvatar,
		DisplayName:       pm.DisplayName,
		CreateTimeSeconds: pm.CreateTimeSeconds,
		UpdateTimeSeconds: pm.UpdateTimeSeconds,
		Mode:              pm.Mode,
		MessageID:         pm.MessageID,
		HideEditted:       pm.HideEditted,
		IsPublic:          pm.IsPublic,
		TopicID:           pm.TopicID,
	}
}

// SafeJSONParse decodes raw JSON content. On failure (or for empty input) it
// returns map[string]any{"t": <raw string>}, mirroring safeJSONParse in the
// TypeScript SDK.
func SafeJSONParse(raw []byte) any {
	s := string(raw)
	if s == "" || s == "[]" {
		return map[string]any{"t": s}
	}

	var out any
	if err := json.Unmarshal(raw, &out); err == nil {
		return out
	}

	// Retry with bare newlines escaped, as some payloads contain raw control
	// characters inside string literals.
	fixed := strings.NewReplacer("\n", "\\n", "\r", "\\r").Replace(s)
	if err := json.Unmarshal([]byte(fixed), &out); err == nil {
		return out
	}

	return map[string]any{"t": s}
}

// DecodeAttachments decodes a channel message attachments payload, which may
// be either JSON or a protobuf-encoded MessageAttachmentList.
func DecodeAttachments(data []byte) []*proto.MessageAttachment {
	if len(data) == 0 {
		return nil
	}

	// '[' (JSON array) or '{' (JSON object) marks a JSON payload.
	if data[0] == '[' || data[0] == '{' {
		var list []*proto.MessageAttachment
		if err := json.Unmarshal(data, &list); err == nil {
			return list
		}
		var wrapper struct {
			Attachments []*proto.MessageAttachment `json:"attachments"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil {
			return wrapper.Attachments
		}
		return nil
	}

	list := &proto.MessageAttachmentList{}
	if err := list.Unmarshal(data); err == nil {
		return list.Attachments
	}
	return nil
}

// DecodeMentions decodes a channel message mentions payload, which may be
// either JSON or a protobuf-encoded MessageMentionList.
func DecodeMentions(data []byte) []*proto.MessageMention {
	if len(data) == 0 {
		return nil
	}

	// '[' (JSON array) or '{' (JSON object) marks a JSON payload.
	if data[0] == '[' || data[0] == '{' {
		var list []*proto.MessageMention
		if err := json.Unmarshal(data, &list); err == nil {
			return list
		}
		var wrapper struct {
			Mentions []*proto.MessageMention `json:"mentions"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil {
			return wrapper.Mentions
		}
		return nil
	}

	list := &proto.MessageMentionList{}
	if err := list.Unmarshal(data); err == nil {
		return list.Mentions
	}
	return nil
}
