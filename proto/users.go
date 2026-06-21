package proto

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// User is mezon.api.User — the subset of account fields used by the SDK.
// Field numbers mirror the ts-proto generated code in mezon-js-protobuf.
type User struct {
	ID          string `json:"id,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Online      bool   `json:"online,omitempty"`
}

func (m *User) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *User) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ID)
	b = appendString(b, 2, m.Username)
	b = appendString(b, 3, m.DisplayName)
	b = appendString(b, 4, m.AvatarURL)
	b = appendBool(b, 9, m.Online)
	return b
}

func (m *User) Unmarshal(b []byte) error {
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
			m.Username = d.str()
		case num == 3 && typ == protowire.BytesType:
			m.DisplayName = d.str()
		case num == 4 && typ == protowire.BytesType:
			m.AvatarURL = d.str()
		case num == 9 && typ == protowire.VarintType:
			m.Online = d.bool()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ClanUser is mezon.api.ClanUserList.ClanUser — a clan member with their
// clan-specific profile.
type ClanUser struct {
	User       *User    `json:"user,omitempty"`
	RoleIDs    []string `json:"role_id,omitempty"`
	ClanNick   string   `json:"clan_nick,omitempty"`
	ClanAvatar string   `json:"clan_avatar,omitempty"`
	ClanID     string   `json:"clan_id,omitempty"`
}

func (m *ClanUser) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ClanUser) MarshalAppend(b []byte) []byte {
	if m.User != nil {
		b = appendMessage(b, 1, m.User)
	}
	b = appendPackedIDs(b, 2, m.RoleIDs)
	b = appendString(b, 3, m.ClanNick)
	b = appendString(b, 4, m.ClanAvatar)
	b = appendID(b, 5, m.ClanID)
	return b
}

func (m *ClanUser) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			m.User = &User{}
			d.sub(m.User)
		case num == 2:
			m.RoleIDs = d.packedIDs(typ, m.RoleIDs)
		case num == 3 && typ == protowire.BytesType:
			m.ClanNick = d.str()
		case num == 4 && typ == protowire.BytesType:
			m.ClanAvatar = d.str()
		case num == 5 && typ == protowire.VarintType:
			m.ClanID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ClanUserList is mezon.api.ClanUserList — the response of ListClanUsers.
type ClanUserList struct {
	ClanUsers []*ClanUser `json:"clan_users,omitempty"`
	Cursor    string      `json:"cursor,omitempty"`
	ClanID    string      `json:"clan_id,omitempty"`
}

func (m *ClanUserList) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ClanUserList) MarshalAppend(b []byte) []byte {
	for _, u := range m.ClanUsers {
		b = appendMessage(b, 1, u)
	}
	b = appendString(b, 2, m.Cursor)
	b = appendID(b, 3, m.ClanID)
	return b
}

func (m *ClanUserList) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			u := &ClanUser{}
			d.sub(u)
			m.ClanUsers = append(m.ClanUsers, u)
		case num == 2 && typ == protowire.BytesType:
			m.Cursor = d.str()
		case num == 3 && typ == protowire.VarintType:
			m.ClanID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelUser is mezon.api.ChannelUserList.ChannelUser — a channel member
// with their clan-specific profile.
type ChannelUser struct {
	UserID         string   `json:"user_id,omitempty"`
	RoleIDs        []string `json:"role_id,omitempty"`
	ID             string   `json:"id,omitempty"`
	ThreadID       string   `json:"thread_id,omitempty"`
	ClanNick       string   `json:"clan_nick,omitempty"`
	ClanAvatar     string   `json:"clan_avatar,omitempty"`
	ClanID         string   `json:"clan_id,omitempty"`
	AddedBy        string   `json:"added_by,omitempty"`
	IsBanned       bool     `json:"is_banned,omitempty"`
	ExpiredBanTime int32    `json:"expired_ban_time,omitempty"`
}

func (m *ChannelUser) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelUser) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.UserID)
	b = appendPackedIDs(b, 2, m.RoleIDs)
	b = appendID(b, 3, m.ID)
	b = appendID(b, 4, m.ThreadID)
	b = appendString(b, 5, m.ClanNick)
	b = appendString(b, 6, m.ClanAvatar)
	b = appendID(b, 7, m.ClanID)
	b = appendID(b, 8, m.AddedBy)
	b = appendBool(b, 9, m.IsBanned)
	b = appendInt32(b, 10, m.ExpiredBanTime)
	return b
}

func (m *ChannelUser) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.UserID = d.id()
		case num == 2:
			m.RoleIDs = d.packedIDs(typ, m.RoleIDs)
		case num == 3 && typ == protowire.VarintType:
			m.ID = d.id()
		case num == 4 && typ == protowire.VarintType:
			m.ThreadID = d.id()
		case num == 5 && typ == protowire.BytesType:
			m.ClanNick = d.str()
		case num == 6 && typ == protowire.BytesType:
			m.ClanAvatar = d.str()
		case num == 7 && typ == protowire.VarintType:
			m.ClanID = d.id()
		case num == 8 && typ == protowire.VarintType:
			m.AddedBy = d.id()
		case num == 9 && typ == protowire.VarintType:
			m.IsBanned = d.bool()
		case num == 10 && typ == protowire.VarintType:
			m.ExpiredBanTime = d.int32()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelUserList is mezon.api.ChannelUserList — the response of
// ListChannelUsers.
type ChannelUserList struct {
	ChannelUsers []*ChannelUser `json:"channel_users,omitempty"`
	Cursor       string         `json:"cursor,omitempty"`
	ChannelID    string         `json:"channel_id,omitempty"`
}

func (m *ChannelUserList) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelUserList) MarshalAppend(b []byte) []byte {
	for _, u := range m.ChannelUsers {
		b = appendMessage(b, 1, u)
	}
	b = appendString(b, 2, m.Cursor)
	b = appendID(b, 3, m.ChannelID)
	return b
}

func (m *ChannelUserList) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			u := &ChannelUser{}
			d.sub(u)
			m.ChannelUsers = append(m.ChannelUsers, u)
		case num == 2 && typ == protowire.BytesType:
			m.Cursor = d.str()
		case num == 3 && typ == protowire.VarintType:
			m.ChannelID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ListChannelUsersRequest is mezon.api.ListChannelUsersRequest.
type ListChannelUsersRequest struct {
	ClanID      string `json:"clan_id,omitempty"`
	ChannelID   string `json:"channel_id,omitempty"`
	ChannelType int32  `json:"channel_type,omitempty"`
	Limit       int32  `json:"limit,omitempty"`
	State       int32  `json:"state,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
}

func (m *ListChannelUsersRequest) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ListChannelUsersRequest) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	b = appendID(b, 2, m.ChannelID)
	b = appendInt32(b, 3, m.ChannelType)
	b = appendInt32(b, 4, m.Limit)
	b = appendInt32(b, 5, m.State)
	b = appendString(b, 6, m.Cursor)
	return b
}

func (m *ListChannelUsersRequest) Unmarshal(b []byte) error {
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
			m.Limit = d.int32()
		case num == 5 && typ == protowire.VarintType:
			m.State = d.int32()
		case num == 6 && typ == protowire.BytesType:
			m.Cursor = d.str()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ListChannelDetailRequest is mezon.api.ListChannelDetailRequest.
type ListChannelDetailRequest struct {
	ChannelID string `json:"channel_id,omitempty"`
}

func (m *ListChannelDetailRequest) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ListChannelDetailRequest) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ChannelID)
	return b
}

func (m *ListChannelDetailRequest) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.ChannelID = d.id()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ListChannelDescsRequest is mezon.api.ListChannelDescsRequest.
type ListChannelDescsRequest struct {
	Limit       int32  `json:"limit,omitempty"`
	State       int32  `json:"state,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
	ClanID      string `json:"clan_id,omitempty"`
	ChannelType int32  `json:"channel_type,omitempty"`
	IsMobile    bool   `json:"is_mobile,omitempty"`
	Page        int32  `json:"page,omitempty"`
}

func (m *ListChannelDescsRequest) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ListChannelDescsRequest) MarshalAppend(b []byte) []byte {
	b = appendInt32(b, 1, m.Limit)
	b = appendInt32(b, 2, m.State)
	b = appendString(b, 3, m.Cursor)
	b = appendID(b, 4, m.ClanID)
	b = appendInt32(b, 5, m.ChannelType)
	b = appendBool(b, 6, m.IsMobile)
	b = appendInt32(b, 7, m.Page)
	return b
}

func (m *ListChannelDescsRequest) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.VarintType:
			m.Limit = d.int32()
		case num == 2 && typ == protowire.VarintType:
			m.State = d.int32()
		case num == 3 && typ == protowire.BytesType:
			m.Cursor = d.str()
		case num == 4 && typ == protowire.VarintType:
			m.ClanID = d.id()
		case num == 5 && typ == protowire.VarintType:
			m.ChannelType = d.int32()
		case num == 6 && typ == protowire.VarintType:
			m.IsMobile = d.bool()
		case num == 7 && typ == protowire.VarintType:
			m.Page = d.int32()
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ChannelDescList is mezon.api.ChannelDescList — the response of
// ListChannelDescs.
type ChannelDescList struct {
	ChannelDescs []*ChannelDescription `json:"channeldesc,omitempty"`
}

func (m *ChannelDescList) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ChannelDescList) MarshalAppend(b []byte) []byte {
	for _, c := range m.ChannelDescs {
		b = appendMessage(b, 1, c)
	}
	return b
}

func (m *ChannelDescList) Unmarshal(b []byte) error {
	d := decoder{b: b}
	for {
		num, typ, ok := d.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			c := &ChannelDescription{}
			d.sub(c)
			m.ChannelDescs = append(m.ChannelDescs, c)
		default:
			d.skip(num, typ)
		}
	}
	return d.err
}

// ListClanUsersRequest is mezon.api.ListClanUsersRequest.
type ListClanUsersRequest struct {
	ClanID string `json:"clan_id,omitempty"`
}

func (m *ListClanUsersRequest) Marshal() []byte { return m.MarshalAppend(nil) }

func (m *ListClanUsersRequest) MarshalAppend(b []byte) []byte {
	b = appendID(b, 1, m.ClanID)
	return b
}

func (m *ListClanUsersRequest) Unmarshal(b []byte) error {
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
