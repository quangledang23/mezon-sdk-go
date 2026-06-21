package proto

import (
	"reflect"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func roundTrip[T interface {
	Marshal() []byte
	Unmarshal([]byte) error
}](t *testing.T, in T, out T) {
	t.Helper()
	data := in.Marshal()
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip mismatch:\n in: %#v\nout: %#v", in, out)
	}
}

func TestEnvelopeChannelMessageSendRoundTrip(t *testing.T) {
	in := &Envelope{
		Cid: 7,
		ChannelMessageSend: &ChannelMessageSend{
			ClanID:    "8",
			ChannelID: "1779484504377790464",
			Content:   `{"t":"hello"}`,
			Mode:      4,
			Code:      1,
			Attachments: []*MessageAttachment{
				{Filename: "image.png", Filetype: "image/png", Size: 1024, Width: 800, Height: 600},
			},
			Mentions: []*MessageMention{
				{UserID: "123456789", Username: "someone", S: 1, E: 8},
			},
			References: []*MessageRef{
				{MessageID: "42", Content: "ref", HasAttachment: true, RefType: 2},
			},
			MentionEveryone: true,
			Avatar:          "https://example.com/a.png",
			TopicID:         "99",
			ID:              "100",
		},
	}
	roundTrip(t, in, &Envelope{})
}

func TestEnvelopeChannelMessageRoundTrip(t *testing.T) {
	in := &Envelope{
		ChannelMessage: &ChannelMessage{
			ClanID:            "8",
			ChannelID:         "1779484504377790464",
			MessageID:         "1786284901100617728",
			Code:              0,
			SenderID:          "1665963703185768448",
			Username:          "alice",
			Avatar:            "https://example.com/avatar.png",
			Content:           `{"t":"hi"}`,
			ChannelLabel:      "general",
			DisplayName:       "Alice",
			Attachments:       []byte(`[{"filename":"f.png"}]`),
			CreateTimeSeconds: 1717000000,
			Mode:              4,
			IsPublic:          true,
			TopicID:           "5",
		},
	}
	roundTrip(t, in, &Envelope{})
}

func TestEnvelopeJoinLeaveErrorRoundTrip(t *testing.T) {
	roundTrip(t, &Envelope{
		Cid:         1,
		ChannelJoin: &ChannelJoin{ClanID: "9", ChannelID: "123", ChannelType: 3, IsPublic: false},
	}, &Envelope{})

	roundTrip(t, &Envelope{
		Cid:          2,
		ChannelLeave: &ChannelLeave{ClanID: "9", ChannelID: "123", ChannelType: 2, IsPublic: true},
	}, &Envelope{})

	roundTrip(t, &Envelope{
		Cid:   3,
		Error: &Error{Code: 4, Message: "match not found", Context: map[string]string{"k": "v"}},
	}, &Envelope{})

	roundTrip(t, &Envelope{Cid: 4, Ping: &Ping{}}, &Envelope{})
	roundTrip(t, &Envelope{Cid: 5, Pong: &Pong{}}, &Envelope{})
}

func TestChannelRoundTrip(t *testing.T) {
	in := &Channel{
		ID:          "777",
		ChanelLabel: "dm",
		Presences: []*UserPresence{
			{UserID: "1", SessionID: 2, Username: "u", Status: ptr("online"), IsMobile: true, UserStatus: "busy"},
		},
		Self: &UserPresence{UserID: "3", Username: "me"},
	}
	roundTrip(t, in, &Channel{})
}

func TestChannelMessageAckRoundTrip(t *testing.T) {
	in := &ChannelMessageAck{
		ChannelID:         "123",
		MessageID:         "456",
		Code:              1,
		Username:          "bob",
		CreateTimeSeconds: 1717000001,
		UpdateTimeSeconds: 1717000002,
		Persistent:        ptr(true),
		ClanLogo:          "logo",
		CategoryName:      "cat",
	}
	roundTrip(t, in, &ChannelMessageAck{})

	// Wrapper with false value must survive the round trip too.
	in2 := &ChannelMessageAck{ChannelID: "1", Persistent: ptr(false)}
	roundTrip(t, in2, &ChannelMessageAck{})
}

func TestSessionRoundTrip(t *testing.T) {
	in := &Session{
		Created:      true,
		Token:        "tok",
		RefreshToken: "ref",
		UserID:       "1665963703185768448",
		IsRemember:   true,
		APIURL:       "https://api.mezon.ai",
		IDToken:      "idtok",
	}
	roundTrip(t, in, &Session{})
}

func TestSessionRefreshRequestRoundTrip(t *testing.T) {
	in := &SessionRefreshRequest{
		Token:      "refresh-token",
		Vars:       map[string]string{"a": "1", "b": "2"},
		IsRemember: true,
	}
	roundTrip(t, in, &SessionRefreshRequest{})
}

func TestCreateChannelDescRequestRoundTrip(t *testing.T) {
	in := &CreateChannelDescRequest{
		Type:           3,
		ChannelPrivate: 1,
		UserIDs:        []string{"1665963703185768448", "42"},
	}
	roundTrip(t, in, &CreateChannelDescRequest{})
}

func TestChannelDescriptionRoundTrip(t *testing.T) {
	in := &ChannelDescription{
		ClanID:          "8",
		ChannelID:       "1779484504377790464",
		Type:            3,
		CreatorID:       "9",
		ChannelLabel:    "dm",
		ChannelPrivate:  1,
		Avatars:         []string{"a1", "a2"},
		UserIDs:         []string{"1", "2"},
		Onlines:         []bool{true, false},
		Usernames:       []string{"u1", "u2"},
		DisplayNames:    []string{"d1", "d2"},
		LastSentMessage: &ChannelMessageHeader{ID: "5", TimestampSeconds: 1717000000, SenderID: "2", Content: "hi"},
		MemberCount:     2,
		E2ee:            1,
	}
	roundTrip(t, in, &ChannelDescription{})
}

func TestUploadAttachmentRoundTrip(t *testing.T) {
	roundTrip(t, &UploadAttachmentRequest{
		Filename: "image.png",
		Filetype: "image/png",
		Size:     1024,
		Width:    800,
		Height:   600,
	}, &UploadAttachmentRequest{})

	roundTrip(t, &UploadAttachment{Filename: "image.png", URL: "https://cdn.mezon.ai/image.png"}, &UploadAttachment{})
}

func TestMessageAttachmentListRoundTrip(t *testing.T) {
	in := &MessageAttachmentList{
		Attachments: []*MessageAttachment{
			{Filename: "a.png", URL: "https://x/a.png", Size: 1, Width: 2, Height: 3, Duration: 4},
			{Filename: "b.mp4", Filetype: "video/mp4", Thumbnail: "https://x/b.jpg"},
		},
	}
	roundTrip(t, in, &MessageAttachmentList{})
}

func TestMessageMentionListRoundTrip(t *testing.T) {
	in := &MessageMentionList{
		Mentions: []*MessageMention{
			{UserID: "1775731111020111321", Username: "@here", E: 5},
			{RoleID: "42", Rolename: "admin", S: 6, E: 12},
		},
	}
	roundTrip(t, in, &MessageMentionList{})
}

func TestLargeSnowflakeIDs(t *testing.T) {
	// IDs near the int64 boundary must survive the string<->varint conversion.
	in := &ChannelJoin{ClanID: "9223372036854775807", ChannelID: "1842581456916774912", ChannelType: 3}
	out := &ChannelJoin{}
	roundTrip(t, in, out)
}

func TestNegativeInt32(t *testing.T) {
	in := &Error{Code: -1, Message: "unrecognized"}
	roundTrip(t, in, &Error{})
}

func TestZeroIDsAreOmitted(t *testing.T) {
	// ts-proto uses "0" as the default for int64-as-string IDs and omits
	// them from the wire; "" and "0" must therefore encode identically
	// (to nothing), and decode back to the Go zero value "".
	withZero := (&ChannelJoin{ClanID: "0", ChannelID: "123"}).Marshal()
	withEmpty := (&ChannelJoin{ClanID: "", ChannelID: "123"}).Marshal()
	if !reflect.DeepEqual(withZero, withEmpty) {
		t.Fatalf("\"0\" and \"\" IDs must encode identically: %x vs %x", withZero, withEmpty)
	}

	out := &ChannelJoin{}
	if err := out.Unmarshal(withZero); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if out.ClanID != "" || out.ChannelID != "123" {
		t.Fatalf("unexpected decode result: %#v", out)
	}
}
