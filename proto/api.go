package proto

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// ChannelMessage is mezon.api.ChannelMessage — a message sent on a channel.
// Reactions/Mentions/Attachments/References hold raw serialized payloads
// (JSON or nested protobuf), exactly as transmitted by the server.
type ChannelMessage struct {
	ClanID            string `json:"clan_id,omitempty"`
	ChannelID         string `json:"channel_id,omitempty"`
	MessageID         string `json:"message_id,omitempty"`
	Code              int32  `json:"code,omitempty"`
	SenderID          string `json:"sender_id,omitempty"`
	Username          string `json:"username,omitempty"`
	Avatar            string `json:"avatar,omitempty"`
	Content           string `json:"content,omitempty"`
	ChannelLabel      string `json:"channel_label,omitempty"`
	ClanLogo          string `json:"clan_logo,omitempty"`
	CategoryName      string `json:"category_name,omitempty"`
	DisplayName       string `json:"display_name,omitempty"`
	ClanNick          string `json:"clan_nick,omitempty"`
	ClanAvatar        string `json:"clan_avatar,omitempty"`
	Reactions         []byte `json:"reactions,omitempty"`
	Mentions          []byte `json:"mentions,omitempty"`
	Attachments       []byte `json:"attachments,omitempty"`
	References        []byte `json:"references,omitempty"`
	ReferencedMessage []byte `json:"referenced_message,omitempty"`
	CreateTimeSeconds uint32 `json:"create_time_seconds,omitempty"`
	UpdateTimeSeconds uint32 `json:"update_time_seconds,omitempty"`
	Mode              int32  `json:"mode,omitempty"`
	HideEditted       bool   `json:"hide_editted,omitempty"`
	IsPublic          bool   `json:"is_public,omitempty"`
	TopicID           string `json:"topic_id,omitempty"`
}

func (m *ChannelMessage) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelMessage) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	b = appendID(b, 2, m.ChannelID)
	b = appendID(b, 3, m.MessageID)
	b = appendInt32(b, 4, m.Code)
	b = appendID(b, 5, m.SenderID)
	b = appendString(b, 6, m.Username)
	b = appendString(b, 7, m.Avatar)
	b = appendString(b, 8, m.Content)
	b = appendString(b, 9, m.ChannelLabel)
	b = appendString(b, 10, m.ClanLogo)
	b = appendString(b, 11, m.CategoryName)
	b = appendString(b, 12, m.DisplayName)
	b = appendString(b, 13, m.ClanNick)
	b = appendString(b, 14, m.ClanAvatar)
	b = appendBytes(b, 15, m.Reactions)
	b = appendBytes(b, 16, m.Mentions)
	b = appendBytes(b, 17, m.Attachments)
	b = appendBytes(b, 18, m.References)
	b = appendBytes(b, 19, m.ReferencedMessage)
	b = appendUint32(b, 20, m.CreateTimeSeconds)
	b = appendUint32(b, 21, m.UpdateTimeSeconds)
	b = appendInt32(b, 22, m.Mode)
	b = appendBool(b, 23, m.HideEditted)
	b = appendBool(b, 24, m.IsPublic)
	b = appendID(b, 25, m.TopicID)
	return b
}

func (m *ChannelMessage) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ClanID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.ChannelID = d.id()
		case num == 3 && typ == protowire.VarintType:
			m.MessageID = d.id()
		case num == 4 && typ == protowire.VarintType:
			m.Code = d.int32()
		case num == 5 && typ == protowire.VarintType:
			m.SenderID = d.id()
		case num == 6 && typ == protowire.BytesType:
			m.Username = d.str()
		case num == 7 && typ == protowire.BytesType:
			m.Avatar = d.str()
		case num == 8 && typ == protowire.BytesType:
			m.Content = d.str()
		case num == 9 && typ == protowire.BytesType:
			m.ChannelLabel = d.str()
		case num == 10 && typ == protowire.BytesType:
			m.ClanLogo = d.str()
		case num == 11 && typ == protowire.BytesType:
			m.CategoryName = d.str()
		case num == 12 && typ == protowire.BytesType:
			m.DisplayName = d.str()
		case num == 13 && typ == protowire.BytesType:
			m.ClanNick = d.str()
		case num == 14 && typ == protowire.BytesType:
			m.ClanAvatar = d.str()
		case num == 15 && typ == protowire.BytesType:
			m.Reactions = d.bytes()
		case num == 16 && typ == protowire.BytesType:
			m.Mentions = d.bytes()
		case num == 17 && typ == protowire.BytesType:
			m.Attachments = d.bytes()
		case num == 18 && typ == protowire.BytesType:
			m.References = d.bytes()
		case num == 19 && typ == protowire.BytesType:
			m.ReferencedMessage = d.bytes()
		case num == 20 && typ == protowire.VarintType:
			m.CreateTimeSeconds = d.uint32()
		case num == 21 && typ == protowire.VarintType:
			m.UpdateTimeSeconds = d.uint32()
		case num == 22 && typ == protowire.VarintType:
			m.Mode = d.int32()
		case num == 23 && typ == protowire.VarintType:
			m.HideEditted = d.bool()
		case num == 24 && typ == protowire.VarintType:
			m.IsPublic = d.bool()
		case num == 25 && typ == protowire.VarintType:
			m.TopicID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// MessageMention is mezon.api.MessageMention.
type MessageMention struct {
	ID                string `json:"id,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	Username          string `json:"username,omitempty"`
	RoleID            string `json:"role_id,omitempty"`
	Rolename          string `json:"rolename,omitempty"`
	CreateTimeSeconds uint32 `json:"create_time_seconds,omitempty"`
	S                 int32  `json:"s,omitempty"`
	E                 int32  `json:"e,omitempty"`
}

func (m *MessageMention) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *MessageMention) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ID)
	b = appendID(b, 2, m.UserID)
	b = appendString(b, 3, m.Username)
	b = appendID(b, 4, m.RoleID)
	b = appendString(b, 5, m.Rolename)
	b = appendUint32(b, 6, m.CreateTimeSeconds)
	b = appendInt32(b, 7, m.S)
	b = appendInt32(b, 8, m.E)
	return b
}

func (m *MessageMention) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.UserID = d.id()
		case num == 3 && typ == protowire.BytesType:
			m.Username = d.str()
		case num == 4 && typ == protowire.VarintType:
			m.RoleID = d.id()
		case num == 5 && typ == protowire.BytesType:
			m.Rolename = d.str()
		case num == 6 && typ == protowire.VarintType:
			m.CreateTimeSeconds = d.uint32()
		case num == 7 && typ == protowire.VarintType:
			m.S = d.int32()
		case num == 8 && typ == protowire.VarintType:
			m.E = d.int32()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// MessageAttachment is mezon.api.MessageAttachment.
type MessageAttachment struct {
	Filename  string `json:"filename,omitempty"`
	Size      int32  `json:"size,omitempty"`
	URL       string `json:"url,omitempty"`
	Filetype  string `json:"filetype,omitempty"`
	Width     int32  `json:"width,omitempty"`
	Height    int32  `json:"height,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Duration  int32  `json:"duration,omitempty"`
}

func (m *MessageAttachment) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *MessageAttachment) MarshalAppend(b []byte) []byte {
	b = appendString(b, 1, m.Filename)
	b = appendInt32(b, 2, m.Size)
	b = appendString(b, 3, m.URL)
	b = appendString(b, 4, m.Filetype)
	b = appendInt32(b, 5, m.Width)
	b = appendInt32(b, 6, m.Height)
	b = appendString(b, 7, m.Thumbnail)
	b = appendInt32(b, 8, m.Duration)
	return b
}

func (m *MessageAttachment) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			m.Filename = d.str()
		case num == 2 && typ == protowire.VarintType:
			m.Size = d.int32()
		case num == 3 && typ == protowire.BytesType:
			m.URL = d.str()
		case num == 4 && typ == protowire.BytesType:
			m.Filetype = d.str()
		case num == 5 && typ == protowire.VarintType:
			m.Width = d.int32()
		case num == 6 && typ == protowire.VarintType:
			m.Height = d.int32()
		case num == 7 && typ == protowire.BytesType:
			m.Thumbnail = d.str()
		case num == 8 && typ == protowire.VarintType:
			m.Duration = d.int32()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// MessageAttachmentList is mezon.api.MessageAttachmentList.
type MessageAttachmentList struct {
	Attachments []*MessageAttachment `json:"attachments,omitempty"`
}

func (m *MessageAttachmentList) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *MessageAttachmentList) MarshalAppend(b []byte) []byte {
	for _, a := range m.Attachments {
		b = appendMessage(b, 1, a)
	}
	return b
}

func (m *MessageAttachmentList) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		if num == 1 && typ == protowire.BytesType {
			a := &MessageAttachment{}
			d.sub(a)
			if d.err == nil {
				m.Attachments = append(m.Attachments, a)
			}
		} else {
			d.skip(num, typ)
		}
	}
	return d.err
}

// MessageMentionList is mezon.api.MessageMentionList.
type MessageMentionList struct {
	Mentions []*MessageMention `json:"mentions,omitempty"`
}

func (m *MessageMentionList) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *MessageMentionList) MarshalAppend(b []byte) []byte {
	for _, v := range m.Mentions {
		b = appendMessage(b, 1, v)
	}
	return b
}

func (m *MessageMentionList) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		if num == 1 && typ == protowire.BytesType {
			v := &MessageMention{}
			d.sub(v)
			if d.err == nil {
				m.Mentions = append(m.Mentions, v)
			}
		} else {
			d.skip(num, typ)
		}
	}
	return d.err
}

// MessageRef is mezon.api.MessageRef.
type MessageRef struct {
	MessageID                string `json:"message_id,omitempty"`
	MessageRefID             string `json:"message_ref_id,omitempty"`
	Content                  string `json:"content,omitempty"`
	HasAttachment            bool   `json:"has_attachment,omitempty"`
	RefType                  int32  `json:"ref_type,omitempty"`
	MessageSenderID          string `json:"message_sender_id,omitempty"`
	MessageSenderUsername    string `json:"message_sender_username,omitempty"`
	MessageSenderAvatar      string `json:"mesages_sender_avatar,omitempty"`
	MessageSenderClanNick    string `json:"message_sender_clan_nick,omitempty"`
	MessageSenderDisplayName string `json:"message_sender_display_name,omitempty"`
}

func (m *MessageRef) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *MessageRef) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.MessageID)
	b = appendID(b, 2, m.MessageRefID)
	b = appendString(b, 3, m.Content)
	b = appendBool(b, 4, m.HasAttachment)
	b = appendInt32(b, 5, m.RefType)
	b = appendID(b, 6, m.MessageSenderID)
	b = appendString(b, 7, m.MessageSenderUsername)
	b = appendString(b, 8, m.MessageSenderAvatar)
	b = appendString(b, 9, m.MessageSenderClanNick)
	b = appendString(b, 10, m.MessageSenderDisplayName)
	return b
}

func (m *MessageRef) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.MessageID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.MessageRefID = d.id()
		case num == 3 && typ == protowire.BytesType:
			m.Content = d.str()
		case num == 4 && typ == protowire.VarintType:
			m.HasAttachment = d.bool()
		case num == 5 && typ == protowire.VarintType:
			m.RefType = d.int32()
		case num == 6 && typ == protowire.VarintType:
			m.MessageSenderID = d.id()
		case num == 7 && typ == protowire.BytesType:
			m.MessageSenderUsername = d.str()
		case num == 8 && typ == protowire.BytesType:
			m.MessageSenderAvatar = d.str()
		case num == 9 && typ == protowire.BytesType:
			m.MessageSenderClanNick = d.str()
		case num == 10 && typ == protowire.BytesType:
			m.MessageSenderDisplayName = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelMessageHeader is mezon.api.ChannelMessageHeader.
type ChannelMessageHeader struct {
	ID               string `json:"id,omitempty"`
	TimestampSeconds uint32 `json:"timestamp_seconds,omitempty"`
	SenderID         string `json:"sender_id,omitempty"`
	Content          string `json:"content,omitempty"`
}

func (m *ChannelMessageHeader) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelMessageHeader) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ID)
	b = appendUint32(b, 2, m.TimestampSeconds)
	b = appendID(b, 3, m.SenderID)
	b = appendString(b, 4, m.Content)
	return b
}

func (m *ChannelMessageHeader) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.TimestampSeconds = d.uint32()
		case num == 3 && typ == protowire.VarintType:
			m.SenderID = d.id()
		case num == 4 && typ == protowire.BytesType:
			m.Content = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// Session is mezon.api.Session — a user's session used to authenticate messages.
type Session struct {
	Created      bool   `json:"created,omitempty"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	IsRemember   bool   `json:"is_remember,omitempty"`
	APIURL       string `json:"api_url,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	WsURL        string `json:"ws_url,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

func (m *Session) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *Session) MarshalAppend(b []byte) []byte {
	b = appendBool(b, 1, m.Created)
	b = appendString(b, 2, m.Token)
	b = appendString(b, 3, m.RefreshToken)
	b = appendID(b, 4, m.UserID)
	b = appendBool(b, 5, m.IsRemember)
	b = appendString(b, 6, m.APIURL)
	b = appendString(b, 7, m.IDToken)
	b = appendString(b, 8, m.WsURL)
	b = appendString(b, 9, m.SessionID)
	return b
}

func (m *Session) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.Created = d.bool()
		case num == 2 && typ == protowire.BytesType:
			m.Token = d.str()
		case num == 3 && typ == protowire.BytesType:
			m.RefreshToken = d.str()
		case num == 4 && typ == protowire.VarintType:
			m.UserID = d.id()
		case num == 5 && typ == protowire.VarintType:
			m.IsRemember = d.bool()
		case num == 6 && typ == protowire.BytesType:
			m.APIURL = d.str()
		case num == 7 && typ == protowire.BytesType:
			m.IDToken = d.str()
		case num == 8 && typ == protowire.BytesType:
			m.WsURL = d.str()
		case num == 9 && typ == protowire.BytesType:
			m.SessionID = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// SessionRefreshRequest is mezon.api.SessionRefreshRequest.
type SessionRefreshRequest struct {
	Token      string            `json:"token,omitempty"`
	Vars       map[string]string `json:"vars,omitempty"`
	IsRemember bool              `json:"is_remember,omitempty"`
}

func (m *SessionRefreshRequest) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *SessionRefreshRequest) MarshalAppend(b []byte) []byte {
	b = appendString(b, 1, m.Token)
	b = appendStringMap(b, 2, m.Vars)
	b = appendBool(b, 3, m.IsRemember)
	return b
}

func (m *SessionRefreshRequest) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			m.Token = d.str()
		case num == 2 && typ == protowire.BytesType:
			if m.Vars == nil {
				m.Vars = make(map[string]string)
			}
			d.stringMapEntry(m.Vars)
		case num == 3 && typ == protowire.VarintType:
			m.IsRemember = d.bool()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// CreateChannelDescRequest is mezon.api.CreateChannelDescRequest.
type CreateChannelDescRequest struct {
	ClanID         string   `json:"clan_id,omitempty"`
	ParentID       string   `json:"parent_id,omitempty"`
	ChannelID      string   `json:"channel_id,omitempty"`
	CategoryID     string   `json:"category_id,omitempty"`
	Type           int32    `json:"type,omitempty"`
	ChannelLabel   string   `json:"channel_label,omitempty"`
	ChannelPrivate int32    `json:"channel_private,omitempty"`
	UserIDs        []string `json:"user_ids,omitempty"`
	AppID          string   `json:"app_id,omitempty"`
}

func (m *CreateChannelDescRequest) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *CreateChannelDescRequest) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	b = appendID(b, 2, m.ParentID)
	b = appendID(b, 3, m.ChannelID)
	b = appendID(b, 4, m.CategoryID)
	b = appendInt32(b, 5, m.Type)
	b = appendString(b, 6, m.ChannelLabel)
	b = appendInt32(b, 7, m.ChannelPrivate)
	b = appendPackedIDs(b, 8, m.UserIDs)
	b = appendID(b, 9, m.AppID)
	return b
}

func (m *CreateChannelDescRequest) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ClanID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.ParentID = d.id()
		case num == 3 && typ == protowire.VarintType:
			m.ChannelID = d.id()
		case num == 4 && typ == protowire.VarintType:
			m.CategoryID = d.id()
		case num == 5 && typ == protowire.VarintType:
			m.Type = d.int32()
		case num == 6 && typ == protowire.BytesType:
			m.ChannelLabel = d.str()
		case num == 7 && typ == protowire.VarintType:
			m.ChannelPrivate = d.int32()
		case num == 8:
			m.UserIDs = d.packedIDs(typ, m.UserIDs)
		case num == 9 && typ == protowire.VarintType:
			m.AppID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelDescription is mezon.api.ChannelDescription.
type ChannelDescription struct {
	ClanID            string                `json:"clan_id,omitempty"`
	ParentID          string                `json:"parent_id,omitempty"`
	ChannelID         string                `json:"channel_id,omitempty"`
	CategoryID        string                `json:"category_id,omitempty"`
	CategoryName      string                `json:"category_name,omitempty"`
	Type              int32                 `json:"type,omitempty"`
	CreatorID         string                `json:"creator_id,omitempty"`
	ChannelLabel      string                `json:"channel_label,omitempty"`
	ChannelPrivate    int32                 `json:"channel_private,omitempty"`
	Avatars           []string              `json:"avatars,omitempty"`
	UserIDs           []string              `json:"user_ids,omitempty"`
	LastSentMessage   *ChannelMessageHeader `json:"last_sent_message,omitempty"`
	LastSeenMessage   *ChannelMessageHeader `json:"last_seen_message,omitempty"`
	Onlines           []bool                `json:"onlines,omitempty"`
	MeetingCode       string                `json:"meeting_code,omitempty"`
	CountMessUnread   int32                 `json:"count_mess_unread,omitempty"`
	Active            int32                 `json:"active,omitempty"`
	LastPinMessage    string                `json:"last_pin_message,omitempty"`
	Usernames         []string              `json:"usernames,omitempty"`
	CreatorName       string                `json:"creator_name,omitempty"`
	CreateTimeSeconds uint32                `json:"create_time_seconds,omitempty"`
	UpdateTimeSeconds uint32                `json:"update_time_seconds,omitempty"`
	DisplayNames      []string              `json:"display_names,omitempty"`
	ChannelAvatar     string                `json:"channel_avatar,omitempty"`
	ClanName          string                `json:"clan_name,omitempty"`
	AppID             string                `json:"app_id,omitempty"`
	IsMute            bool                  `json:"is_mute,omitempty"`
	AgeRestricted     int32                 `json:"age_restricted,omitempty"`
	Topic             string                `json:"topic,omitempty"`
	E2ee              int32                 `json:"e2ee,omitempty"`
	MemberCount       int32                 `json:"member_count,omitempty"`
}

func (m *ChannelDescription) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelDescription) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	b = appendID(b, 2, m.ParentID)
	b = appendID(b, 3, m.ChannelID)
	b = appendID(b, 4, m.CategoryID)
	b = appendString(b, 5, m.CategoryName)
	b = appendInt32(b, 6, m.Type)
	b = appendID(b, 7, m.CreatorID)
	b = appendString(b, 8, m.ChannelLabel)
	b = appendInt32(b, 9, m.ChannelPrivate)
	for _, v := range m.Avatars {
		b = appendString(b, 10, v)
	}
	b = appendPackedIDs(b, 11, m.UserIDs)
	if m.LastSentMessage != nil {
		b = appendMessage(b, 12, m.LastSentMessage)
	}
	if m.LastSeenMessage != nil {
		b = appendMessage(b, 13, m.LastSeenMessage)
	}
	b = appendPackedBools(b, 14, m.Onlines)
	b = appendString(b, 15, m.MeetingCode)
	b = appendInt32(b, 16, m.CountMessUnread)
	b = appendInt32(b, 17, m.Active)
	b = appendString(b, 18, m.LastPinMessage)
	for _, v := range m.Usernames {
		b = appendString(b, 19, v)
	}
	b = appendString(b, 20, m.CreatorName)
	b = appendUint32(b, 21, m.CreateTimeSeconds)
	b = appendUint32(b, 22, m.UpdateTimeSeconds)
	for _, v := range m.DisplayNames {
		b = appendString(b, 23, v)
	}
	b = appendString(b, 24, m.ChannelAvatar)
	b = appendString(b, 25, m.ClanName)
	b = appendID(b, 26, m.AppID)
	b = appendBool(b, 27, m.IsMute)
	b = appendInt32(b, 28, m.AgeRestricted)
	b = appendString(b, 29, m.Topic)
	b = appendInt32(b, 30, m.E2ee)
	b = appendInt32(b, 31, m.MemberCount)
	return b
}

func (m *ChannelDescription) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ClanID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.ParentID = d.id()
		case num == 3 && typ == protowire.VarintType:
			m.ChannelID = d.id()
		case num == 4 && typ == protowire.VarintType:
			m.CategoryID = d.id()
		case num == 5 && typ == protowire.BytesType:
			m.CategoryName = d.str()
		case num == 6 && typ == protowire.VarintType:
			m.Type = d.int32()
		case num == 7 && typ == protowire.VarintType:
			m.CreatorID = d.id()
		case num == 8 && typ == protowire.BytesType:
			m.ChannelLabel = d.str()
		case num == 9 && typ == protowire.VarintType:
			m.ChannelPrivate = d.int32()
		case num == 10 && typ == protowire.BytesType:
			m.Avatars = append(m.Avatars, d.str())
		case num == 11:
			m.UserIDs = d.packedIDs(typ, m.UserIDs)
		case num == 12 && typ == protowire.BytesType:
			m.LastSentMessage = &ChannelMessageHeader{}
			d.sub(m.LastSentMessage)
		case num == 13 && typ == protowire.BytesType:
			m.LastSeenMessage = &ChannelMessageHeader{}
			d.sub(m.LastSeenMessage)
		case num == 14:
			m.Onlines = d.packedBools(typ, m.Onlines)
		case num == 15 && typ == protowire.BytesType:
			m.MeetingCode = d.str()
		case num == 16 && typ == protowire.VarintType:
			m.CountMessUnread = d.int32()
		case num == 17 && typ == protowire.VarintType:
			m.Active = d.int32()
		case num == 18 && typ == protowire.BytesType:
			m.LastPinMessage = d.str()
		case num == 19 && typ == protowire.BytesType:
			m.Usernames = append(m.Usernames, d.str())
		case num == 20 && typ == protowire.BytesType:
			m.CreatorName = d.str()
		case num == 21 && typ == protowire.VarintType:
			m.CreateTimeSeconds = d.uint32()
		case num == 22 && typ == protowire.VarintType:
			m.UpdateTimeSeconds = d.uint32()
		case num == 23 && typ == protowire.BytesType:
			m.DisplayNames = append(m.DisplayNames, d.str())
		case num == 24 && typ == protowire.BytesType:
			m.ChannelAvatar = d.str()
		case num == 25 && typ == protowire.BytesType:
			m.ClanName = d.str()
		case num == 26 && typ == protowire.VarintType:
			m.AppID = d.id()
		case num == 27 && typ == protowire.VarintType:
			m.IsMute = d.bool()
		case num == 28 && typ == protowire.VarintType:
			m.AgeRestricted = d.int32()
		case num == 29 && typ == protowire.BytesType:
			m.Topic = d.str()
		case num == 30 && typ == protowire.VarintType:
			m.E2ee = d.int32()
		case num == 31 && typ == protowire.VarintType:
			m.MemberCount = d.int32()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// UploadAttachmentRequest is mezon.api.UploadAttachmentRequest.
type UploadAttachmentRequest struct {
	Filename string `json:"filename,omitempty"`
	Filetype string `json:"filetype,omitempty"`
	Size     int32  `json:"size,omitempty"`
	Width    int32  `json:"width,omitempty"`
	Height   int32  `json:"height,omitempty"`
}

func (m *UploadAttachmentRequest) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *UploadAttachmentRequest) MarshalAppend(b []byte) []byte {
	b = appendString(b, 1, m.Filename)
	b = appendString(b, 2, m.Filetype)
	b = appendInt32(b, 3, m.Size)
	b = appendInt32(b, 4, m.Width)
	b = appendInt32(b, 5, m.Height)
	return b
}

func (m *UploadAttachmentRequest) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			m.Filename = d.str()
		case num == 2 && typ == protowire.BytesType:
			m.Filetype = d.str()
		case num == 3 && typ == protowire.VarintType:
			m.Size = d.int32()
		case num == 4 && typ == protowire.VarintType:
			m.Width = d.int32()
		case num == 5 && typ == protowire.VarintType:
			m.Height = d.int32()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// UploadAttachment is mezon.api.UploadAttachment.
type UploadAttachment struct {
	Filename string `json:"filename,omitempty"`
	URL      string `json:"url,omitempty"`
}

func (m *UploadAttachment) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *UploadAttachment) MarshalAppend(b []byte) []byte {
	b = appendString(b, 1, m.Filename)
	b = appendString(b, 2, m.URL)
	return b
}

func (m *UploadAttachment) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			m.Filename = d.str()
		case num == 2 && typ == protowire.BytesType:
			m.URL = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

