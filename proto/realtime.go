package proto

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// Envelope is mezon.realtime.Envelope — an envelope for a realtime message.
// Only the message kinds used by the light SDK are mapped; unknown fields
// are skipped on decode.
type Envelope struct {
	Cid                int32               `json:"cid,omitempty"`
	Channel            *Channel            `json:"channel,omitempty"`
	ClanJoin           *ClanJoin           `json:"clan_join,omitempty"`
	ChannelJoin        *ChannelJoin        `json:"channel_join,omitempty"`
	ChannelLeave       *ChannelLeave       `json:"channel_leave,omitempty"`
	ChannelMessage     *ChannelMessage     `json:"channel_message,omitempty"`
	ChannelMessageAck  *ChannelMessageAck  `json:"channel_message_ack,omitempty"`
	ChannelMessageSend *ChannelMessageSend `json:"channel_message_send,omitempty"`
	Error              *Error              `json:"error,omitempty"`
	Ping               *Ping               `json:"ping,omitempty"`
	Pong               *Pong               `json:"pong,omitempty"`
}

func (m *Envelope) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *Envelope) MarshalAppend(b []byte) []byte {
	b = appendInt32(b, 1, m.Cid)
	if m.Channel != nil {
		b = appendMessage(b, 2, m.Channel)
	}
	if m.ClanJoin != nil {
		b = appendMessage(b, 3, m.ClanJoin)
	}
	if m.ChannelJoin != nil {
		b = appendMessage(b, 4, m.ChannelJoin)
	}
	if m.ChannelLeave != nil {
		b = appendMessage(b, 5, m.ChannelLeave)
	}
	if m.ChannelMessage != nil {
		b = appendMessage(b, 6, m.ChannelMessage)
	}
	if m.ChannelMessageAck != nil {
		b = appendMessage(b, 7, m.ChannelMessageAck)
	}
	if m.ChannelMessageSend != nil {
		b = appendMessage(b, 8, m.ChannelMessageSend)
	}
	if m.Error != nil {
		b = appendMessage(b, 12, m.Error)
	}
	if m.Ping != nil {
		b = appendMessage(b, 22, m.Ping)
	}
	if m.Pong != nil {
		b = appendMessage(b, 23, m.Pong)
	}
	return b
}

func (m *Envelope) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.Cid = d.int32()
		case num == 2 && typ == protowire.BytesType:
			m.Channel = &Channel{}
			d.sub(m.Channel)
		case num == 3 && typ == protowire.BytesType:
			m.ClanJoin = &ClanJoin{}
			d.sub(m.ClanJoin)
		case num == 4 && typ == protowire.BytesType:
			m.ChannelJoin = &ChannelJoin{}
			d.sub(m.ChannelJoin)
		case num == 5 && typ == protowire.BytesType:
			m.ChannelLeave = &ChannelLeave{}
			d.sub(m.ChannelLeave)
		case num == 6 && typ == protowire.BytesType:
			m.ChannelMessage = &ChannelMessage{}
			d.sub(m.ChannelMessage)
		case num == 7 && typ == protowire.BytesType:
			m.ChannelMessageAck = &ChannelMessageAck{}
			d.sub(m.ChannelMessageAck)
		case num == 8 && typ == protowire.BytesType:
			m.ChannelMessageSend = &ChannelMessageSend{}
			d.sub(m.ChannelMessageSend)
		case num == 12 && typ == protowire.BytesType:
			m.Error = &Error{}
			d.sub(m.Error)
		case num == 22 && typ == protowire.BytesType:
			m.Ping = &Ping{}
			d.sub(m.Ping)
		case num == 23 && typ == protowire.BytesType:
			m.Pong = &Pong{}
			d.sub(m.Pong)
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// Channel is mezon.realtime.Channel — a realtime chat channel.
type Channel struct {
	ID           string          `json:"id,omitempty"`
	Presences    []*UserPresence `json:"presences,omitempty"`
	Self         *UserPresence   `json:"self,omitempty"`
	ChanelLabel  string          `json:"chanel_label,omitempty"`
	ClanLogo     string          `json:"clan_logo,omitempty"`
	CategoryName string          `json:"category_name,omitempty"`
}

func (m *Channel) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *Channel) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ID)
	for _, p := range m.Presences {
		b = appendMessage(b, 2, p)
	}
	if m.Self != nil {
		b = appendMessage(b, 3, m.Self)
	}
	b = appendString(b, 4, m.ChanelLabel)
	b = appendString(b, 5, m.ClanLogo)
	b = appendString(b, 6, m.CategoryName)
	return b
}

func (m *Channel) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ID = d.id()
		case num == 2 && typ == protowire.BytesType:
			p := &UserPresence{}
			d.sub(p)
			if d.err == nil {
				m.Presences = append(m.Presences, p)
			}
		case num == 3 && typ == protowire.BytesType:
			m.Self = &UserPresence{}
			d.sub(m.Self)
		case num == 4 && typ == protowire.BytesType:
			m.ChanelLabel = d.str()
		case num == 5 && typ == protowire.BytesType:
			m.ClanLogo = d.str()
		case num == 6 && typ == protowire.BytesType:
			m.CategoryName = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// UserPresence is mezon.realtime.UserPresence.
type UserPresence struct {
	UserID     string  `json:"user_id,omitempty"`
	SessionID  int32   `json:"session_id,omitempty"`
	Username   string  `json:"username,omitempty"`
	Status     *string `json:"status,omitempty"`
	IsMobile   bool    `json:"is_mobile,omitempty"`
	UserStatus string  `json:"user_status,omitempty"`
}

func (m *UserPresence) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *UserPresence) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.UserID)
	b = appendInt32(b, 2, m.SessionID)
	b = appendString(b, 3, m.Username)
	b = appendStringValue(b, 4, m.Status)
	b = appendBool(b, 5, m.IsMobile)
	b = appendString(b, 6, m.UserStatus)
	return b
}

func (m *UserPresence) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.UserID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.SessionID = d.int32()
		case num == 3 && typ == protowire.BytesType:
			m.Username = d.str()
		case num == 4 && typ == protowire.BytesType:
			m.Status = d.stringValue()
		case num == 5 && typ == protowire.VarintType:
			m.IsMobile = d.bool()
		case num == 6 && typ == protowire.BytesType:
			m.UserStatus = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ClanJoin is mezon.realtime.ClanJoin — joins clan-level realtime events.
type ClanJoin struct {
	ClanID string `json:"clan_id,omitempty"`
}

func (m *ClanJoin) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ClanJoin) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	return b
}

func (m *ClanJoin) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ClanID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelJoin is mezon.realtime.ChannelJoin — join a realtime chat channel.
type ChannelJoin struct {
	ClanID      string `json:"clan_id,omitempty"`
	ChannelID   string `json:"channel_id,omitempty"`
	ChannelType int32  `json:"channel_type,omitempty"`
	IsPublic    bool   `json:"is_public,omitempty"`
}

func (m *ChannelJoin) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelJoin) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	b = appendID(b, 2, m.ChannelID)
	b = appendInt32(b, 3, m.ChannelType)
	b = appendBool(b, 4, m.IsPublic)
	return b
}

func (m *ChannelJoin) Unmarshal(b []byte) error {
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
			m.ChannelType = d.int32()
		case num == 4 && typ == protowire.VarintType:
			m.IsPublic = d.bool()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelLeave is mezon.realtime.ChannelLeave — leave a realtime channel.
type ChannelLeave struct {
	ClanID      string `json:"clan_id,omitempty"`
	ChannelID   string `json:"channel_id,omitempty"`
	ChannelType int32  `json:"channel_type,omitempty"`
	IsPublic    bool   `json:"is_public,omitempty"`
}

func (m *ChannelLeave) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelLeave) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	b = appendID(b, 2, m.ChannelID)
	b = appendInt32(b, 3, m.ChannelType)
	b = appendBool(b, 4, m.IsPublic)
	return b
}

func (m *ChannelLeave) Unmarshal(b []byte) error {
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
			m.ChannelType = d.int32()
		case num == 4 && typ == protowire.VarintType:
			m.IsPublic = d.bool()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelMessageAck is mezon.realtime.ChannelMessageAck — a receipt reply
// from a channel message send operation.
type ChannelMessageAck struct {
	ChannelID         string `json:"channel_id,omitempty"`
	MessageID         string `json:"message_id,omitempty"`
	Code              int32  `json:"code,omitempty"`
	Username          string `json:"username,omitempty"`
	CreateTimeSeconds uint32 `json:"create_time_seconds,omitempty"`
	UpdateTimeSeconds uint32 `json:"update_time_seconds,omitempty"`
	Persistent        *bool  `json:"persistent,omitempty"`
	ClanLogo          string `json:"clan_logo,omitempty"`
	CategoryName      string `json:"category_name,omitempty"`
}

func (m *ChannelMessageAck) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelMessageAck) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ChannelID)
	b = appendID(b, 2, m.MessageID)
	b = appendInt32(b, 3, m.Code)
	b = appendString(b, 4, m.Username)
	b = appendUint32(b, 5, m.CreateTimeSeconds)
	b = appendUint32(b, 6, m.UpdateTimeSeconds)
	b = appendBoolValue(b, 7, m.Persistent)
	b = appendString(b, 8, m.ClanLogo)
	b = appendString(b, 9, m.CategoryName)
	return b
}

func (m *ChannelMessageAck) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ChannelID = d.id()
		case num == 2 && typ == protowire.VarintType:
			m.MessageID = d.id()
		case num == 3 && typ == protowire.VarintType:
			m.Code = d.int32()
		case num == 4 && typ == protowire.BytesType:
			m.Username = d.str()
		case num == 5 && typ == protowire.VarintType:
			m.CreateTimeSeconds = d.uint32()
		case num == 6 && typ == protowire.VarintType:
			m.UpdateTimeSeconds = d.uint32()
		case num == 7 && typ == protowire.BytesType:
			m.Persistent = d.boolValue()
		case num == 8 && typ == protowire.BytesType:
			m.ClanLogo = d.str()
		case num == 9 && typ == protowire.BytesType:
			m.CategoryName = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelMessageSend is mezon.realtime.ChannelMessageSend — send a message
// to a realtime channel.
type ChannelMessageSend struct {
	ClanID           string               `json:"clan_id,omitempty"`
	ChannelID        string               `json:"channel_id,omitempty"`
	Content          string               `json:"content,omitempty"`
	Mentions         []*MessageMention    `json:"mentions,omitempty"`
	Attachments      []*MessageAttachment `json:"attachments,omitempty"`
	References       []*MessageRef        `json:"references,omitempty"`
	Mode             int32                `json:"mode,omitempty"`
	AnonymousMessage bool                 `json:"anonymous_message,omitempty"`
	MentionEveryone  bool                 `json:"mention_everyone,omitempty"`
	Avatar           string               `json:"avatar,omitempty"`
	IsPublic         bool                 `json:"is_public,omitempty"`
	Code             int32                `json:"code,omitempty"`
	TopicID          string               `json:"topic_id,omitempty"`
	ID               string               `json:"id,omitempty"`
}

func (m *ChannelMessageSend) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelMessageSend) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	b = appendID(b, 2, m.ChannelID)
	b = appendString(b, 3, m.Content)
	for _, v := range m.Mentions {
		b = appendMessage(b, 4, v)
	}
	for _, v := range m.Attachments {
		b = appendMessage(b, 5, v)
	}
	for _, v := range m.References {
		b = appendMessage(b, 6, v)
	}
	b = appendInt32(b, 7, m.Mode)
	b = appendBool(b, 8, m.AnonymousMessage)
	b = appendBool(b, 9, m.MentionEveryone)
	b = appendString(b, 10, m.Avatar)
	b = appendBool(b, 11, m.IsPublic)
	b = appendInt32(b, 12, m.Code)
	b = appendID(b, 13, m.TopicID)
	b = appendID(b, 14, m.ID)
	return b
}

func (m *ChannelMessageSend) Unmarshal(b []byte) error {
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
		case num == 3 && typ == protowire.BytesType:
			m.Content = d.str()
		case num == 4 && typ == protowire.BytesType:
			v := &MessageMention{}
			d.sub(v)
			if d.err == nil {
				m.Mentions = append(m.Mentions, v)
			}
		case num == 5 && typ == protowire.BytesType:
			v := &MessageAttachment{}
			d.sub(v)
			if d.err == nil {
				m.Attachments = append(m.Attachments, v)
			}
		case num == 6 && typ == protowire.BytesType:
			v := &MessageRef{}
			d.sub(v)
			if d.err == nil {
				m.References = append(m.References, v)
			}
		case num == 7 && typ == protowire.VarintType:
			m.Mode = d.int32()
		case num == 8 && typ == protowire.VarintType:
			m.AnonymousMessage = d.bool()
		case num == 9 && typ == protowire.VarintType:
			m.MentionEveryone = d.bool()
		case num == 10 && typ == protowire.BytesType:
			m.Avatar = d.str()
		case num == 11 && typ == protowire.VarintType:
			m.IsPublic = d.bool()
		case num == 12 && typ == protowire.VarintType:
			m.Code = d.int32()
		case num == 13 && typ == protowire.VarintType:
			m.TopicID = d.id()
		case num == 14 && typ == protowire.VarintType:
			m.ID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// Error is mezon.realtime.Error — a logical error which may occur on the
// server.
type Error struct {
	Code    int32             `json:"code,omitempty"`
	Message string            `json:"message,omitempty"`
	Context map[string]string `json:"context,omitempty"`
}

func (m *Error) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *Error) MarshalAppend(b []byte) []byte {
	b = appendInt32(b, 1, m.Code)
	b = appendString(b, 2, m.Message)
	b = appendStringMap(b, 3, m.Context)
	return b
}

func (m *Error) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.Code = d.int32()
		case num == 2 && typ == protowire.BytesType:
			m.Message = d.str()
		case num == 3 && typ == protowire.BytesType:
			if m.Context == nil {
				m.Context = make(map[string]string)
			}
			d.stringMapEntry(m.Context)
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// Ping is mezon.realtime.Ping — an application-level heartbeat.
type Ping struct{}

func (m *Ping) Marshal() []byte               { return nil }
func (m *Ping) MarshalAppend(b []byte) []byte { return b }
func (m *Ping) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		d.skip(num, typ)
	}
	return d.err
}

// Pong is mezon.realtime.Pong — an application-level heartbeat response.
type Pong struct{}

func (m *Pong) Marshal() []byte               { return nil }
func (m *Pong) MarshalAppend(b []byte) []byte { return b }
func (m *Pong) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		d.skip(num, typ)
	}
	return d.err
}
