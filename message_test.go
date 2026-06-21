package mezonlightsdk

import (
	"reflect"
	"testing"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

func TestSafeJSONParse(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want any
	}{
		{
			name: "empty input",
			raw:  "",
			want: map[string]any{"t": ""},
		},
		{
			name: "empty array literal",
			raw:  "[]",
			want: map[string]any{"t": "[]"},
		},
		{
			name: "valid object",
			raw:  `{"t":"hello"}`,
			want: map[string]any{"t": "hello"},
		},
		{
			name: "valid array",
			raw:  `[1,2]`,
			want: []any{float64(1), float64(2)},
		},
		{
			name: "valid scalar",
			raw:  `42`,
			want: float64(42),
		},
		{
			name: "raw newline inside string literal",
			raw:  "{\"t\":\"line1\nline2\"}",
			want: map[string]any{"t": "line1\nline2"},
		},
		{
			name: "raw carriage return inside string literal",
			raw:  "{\"t\":\"a\rb\"}",
			want: map[string]any{"t": "a\rb"},
		},
		{
			name: "invalid JSON falls back to t wrapper",
			raw:  "plain text message",
			want: map[string]any{"t": "plain text message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeJSONParse([]byte(tt.raw))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SafeJSONParse(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDecodeAttachments(t *testing.T) {
	att := &proto.MessageAttachment{
		Filename: "photo.png",
		Size:     1024,
		URL:      "https://cdn.example.com/photo.png",
		Filetype: "image/png",
		Width:    800,
		Height:   600,
	}

	t.Run("empty input", func(t *testing.T) {
		if got := DecodeAttachments(nil); got != nil {
			t.Errorf("DecodeAttachments(nil) = %v, want nil", got)
		}
		if got := DecodeAttachments([]byte{}); got != nil {
			t.Errorf("DecodeAttachments(empty) = %v, want nil", got)
		}
	})

	t.Run("JSON array", func(t *testing.T) {
		data := []byte(`[{"filename":"photo.png","size":1024,"url":"https://cdn.example.com/photo.png","filetype":"image/png","width":800,"height":600}]`)
		got := DecodeAttachments(data)
		if len(got) != 1 || !reflect.DeepEqual(got[0], att) {
			t.Errorf("DecodeAttachments(JSON array) = %+v, want [%+v]", got, att)
		}
	})

	t.Run("JSON wrapper object", func(t *testing.T) {
		data := []byte(`{"attachments":[{"filename":"photo.png","size":1024,"url":"https://cdn.example.com/photo.png","filetype":"image/png","width":800,"height":600}]}`)
		got := DecodeAttachments(data)
		if len(got) != 1 || !reflect.DeepEqual(got[0], att) {
			t.Errorf("DecodeAttachments(JSON wrapper) = %+v, want [%+v]", got, att)
		}
	})

	t.Run("protobuf MessageAttachmentList", func(t *testing.T) {
		list := &proto.MessageAttachmentList{Attachments: []*proto.MessageAttachment{att}}
		got := DecodeAttachments(list.Marshal())
		if len(got) != 1 || !reflect.DeepEqual(got[0], att) {
			t.Errorf("DecodeAttachments(protobuf) = %+v, want [%+v]", got, att)
		}
	})

	t.Run("invalid JSON array", func(t *testing.T) {
		if got := DecodeAttachments([]byte(`[{"bad`)); got != nil {
			t.Errorf("DecodeAttachments(invalid JSON) = %v, want nil", got)
		}
	})

	t.Run("invalid protobuf", func(t *testing.T) {
		if got := DecodeAttachments([]byte{0xff}); got != nil {
			t.Errorf("DecodeAttachments(invalid protobuf) = %v, want nil", got)
		}
	})
}

func TestDecodeMentions(t *testing.T) {
	mention := &proto.MessageMention{
		UserID:   MentionHereUserID,
		Username: "@here",
		S:        0,
		E:        5,
	}

	t.Run("empty input", func(t *testing.T) {
		if got := DecodeMentions(nil); got != nil {
			t.Errorf("DecodeMentions(nil) = %v, want nil", got)
		}
		if got := DecodeMentions([]byte{}); got != nil {
			t.Errorf("DecodeMentions(empty) = %v, want nil", got)
		}
	})

	t.Run("JSON array", func(t *testing.T) {
		data := []byte(`[{"user_id":"1775731111020111321","username":"@here","e":5}]`)
		got := DecodeMentions(data)
		if len(got) != 1 || !reflect.DeepEqual(got[0], mention) {
			t.Errorf("DecodeMentions(JSON array) = %+v, want [%+v]", got, mention)
		}
	})

	t.Run("JSON wrapper object", func(t *testing.T) {
		data := []byte(`{"mentions":[{"user_id":"1775731111020111321","username":"@here","e":5}]}`)
		got := DecodeMentions(data)
		if len(got) != 1 || !reflect.DeepEqual(got[0], mention) {
			t.Errorf("DecodeMentions(JSON wrapper) = %+v, want [%+v]", got, mention)
		}
	})

	t.Run("protobuf MessageMentionList", func(t *testing.T) {
		list := &proto.MessageMentionList{Mentions: []*proto.MessageMention{mention}}
		got := DecodeMentions(list.Marshal())
		if len(got) != 1 || !reflect.DeepEqual(got[0], mention) {
			t.Errorf("DecodeMentions(protobuf) = %+v, want [%+v]", got, mention)
		}
	})

	t.Run("invalid JSON array", func(t *testing.T) {
		if got := DecodeMentions([]byte(`[{"bad`)); got != nil {
			t.Errorf("DecodeMentions(invalid JSON) = %v, want nil", got)
		}
	})

	t.Run("invalid protobuf", func(t *testing.T) {
		if got := DecodeMentions([]byte{0xff}); got != nil {
			t.Errorf("DecodeMentions(invalid protobuf) = %v, want nil", got)
		}
	})
}

func TestNewChannelMessageFromProto(t *testing.T) {
	attachments := &proto.MessageAttachmentList{
		Attachments: []*proto.MessageAttachment{{Filename: "doc.pdf", URL: "https://cdn.test/doc.pdf"}},
	}
	pm := &proto.ChannelMessage{
		ClanID:            "100",
		ChannelID:         "200",
		MessageID:         "300",
		Code:              1,
		SenderID:          "400",
		Username:          "alice",
		Avatar:            "https://cdn.test/a.png",
		Content:           `{"t":"hello"}`,
		ChannelLabel:      "general",
		DisplayName:       "Alice",
		Attachments:       attachments.Marshal(),
		CreateTimeSeconds: 1700000000,
		Mode:              4,
		HideEditted:       true,
		IsPublic:          true,
		TopicID:           "500",
	}

	got := newChannelMessageFromProto(pm)

	if got.ID != "300" || got.MessageID != "300" {
		t.Errorf("ID = %q, MessageID = %q, want both %q", got.ID, got.MessageID, "300")
	}
	if got.ChannelID != "200" || got.ClanID != "100" || got.SenderID != "400" || got.TopicID != "500" {
		t.Error("identifier fields not copied")
	}
	wantContent := map[string]any{"t": "hello"}
	if !reflect.DeepEqual(got.Content, wantContent) {
		t.Errorf("Content = %#v, want %#v", got.Content, wantContent)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "doc.pdf" {
		t.Errorf("Attachments = %+v, want decoded protobuf list", got.Attachments)
	}
	if got.Username != "alice" || got.DisplayName != "Alice" || got.ChannelLabel != "general" {
		t.Error("descriptive fields not copied")
	}
	if got.Code != 1 || got.Mode != 4 || got.CreateTimeSeconds != 1700000000 {
		t.Error("numeric fields not copied")
	}
	if !got.HideEditted || !got.IsPublic {
		t.Error("boolean fields not copied")
	}
}

func TestNewChannelMessageFromProtoPlainTextContent(t *testing.T) {
	got := newChannelMessageFromProto(&proto.ChannelMessage{Content: "raw text"})
	wantContent := map[string]any{"t": "raw text"}
	if !reflect.DeepEqual(got.Content, wantContent) {
		t.Errorf("Content = %#v, want %#v", got.Content, wantContent)
	}
	if got.Attachments != nil {
		t.Errorf("Attachments = %v, want nil for empty payload", got.Attachments)
	}
}
