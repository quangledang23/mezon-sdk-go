package proto

import (
	"reflect"
	"testing"
)

func TestClanJoinRoundTrip(t *testing.T) {
	in := &ClanJoin{ClanID: "1975864750969458688"}
	out := &ClanJoin{}
	if err := out.Unmarshal(in.Marshal()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestEnvelopeClanJoinRoundTrip(t *testing.T) {
	in := &Envelope{Cid: 5, ClanJoin: &ClanJoin{ClanID: "123456"}}
	out := &Envelope{}
	if err := out.Unmarshal(in.Marshal()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestClanUserListRoundTrip(t *testing.T) {
	in := &ClanUserList{
		ClanUsers: []*ClanUser{{
			User:       &User{ID: "42", Username: "alice", DisplayName: "Alice", AvatarURL: "https://a.test/p.png", Online: true},
			RoleIDs:    []string{"7", "8"},
			ClanNick:   "Ali",
			ClanAvatar: "https://a.test/c.png",
			ClanID:     "99",
		}},
		Cursor: "next",
		ClanID: "99",
	}
	out := &ClanUserList{}
	if err := out.Unmarshal(in.Marshal()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestChannelUserListRoundTrip(t *testing.T) {
	in := &ChannelUserList{
		ChannelUsers: []*ChannelUser{{
			UserID:   "42",
			RoleIDs:  []string{"7"},
			ID:       "1",
			ThreadID: "2",
			ClanNick: "Ali",
			ClanID:   "99",
			AddedBy:  "3",
			IsBanned: true,
		}},
		Cursor:    "next",
		ChannelID: "55",
	}
	out := &ChannelUserList{}
	if err := out.Unmarshal(in.Marshal()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestListRequestsRoundTrip(t *testing.T) {
	{
		in := &ListClanUsersRequest{ClanID: "99"}
		out := &ListClanUsersRequest{}
		if err := out.Unmarshal(in.Marshal()); err != nil || !reflect.DeepEqual(in, out) {
			t.Errorf("ListClanUsersRequest round trip = %+v, %v", out, err)
		}
	}
	{
		in := &ListChannelUsersRequest{ClanID: "99", ChannelID: "55", ChannelType: 1, Limit: 100, State: 1, Cursor: "c"}
		out := &ListChannelUsersRequest{}
		if err := out.Unmarshal(in.Marshal()); err != nil || !reflect.DeepEqual(in, out) {
			t.Errorf("ListChannelUsersRequest round trip = %+v, %v", out, err)
		}
	}
	{
		in := &ListChannelDetailRequest{ChannelID: "55"}
		out := &ListChannelDetailRequest{}
		if err := out.Unmarshal(in.Marshal()); err != nil || !reflect.DeepEqual(in, out) {
			t.Errorf("ListChannelDetailRequest round trip = %+v, %v", out, err)
		}
	}
	{
		in := &ListChannelDescsRequest{Limit: 100, State: 1, Cursor: "c", ClanID: "99", ChannelType: 3, IsMobile: true, Page: 2}
		out := &ListChannelDescsRequest{}
		if err := out.Unmarshal(in.Marshal()); err != nil || !reflect.DeepEqual(in, out) {
			t.Errorf("ListChannelDescsRequest round trip = %+v, %v", out, err)
		}
	}
}

func TestChannelDescListRoundTrip(t *testing.T) {
	in := &ChannelDescList{
		ChannelDescs: []*ChannelDescription{{ChannelID: "55", ChannelLabel: "general", Type: 1}},
	}
	out := &ChannelDescList{}
	if err := out.Unmarshal(in.Marshal()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestSessionWsURLRoundTrip(t *testing.T) {
	in := &Session{Token: "t", RefreshToken: "r", WsURL: "sock.test", SessionID: "abc"}
	out := &Session{}
	if err := out.Unmarshal(in.Marshal()); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}
