package mezonlightsdk

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []*MessageMarkup
	}{
		{name: "no links", text: "hello world", want: nil},
		{
			name: "single link",
			text: "see https://mezon.ai now",
			want: []*MessageMarkup{{Type: MarkupTypeLink, S: 4, E: 20}},
		},
		{
			// "xin chào " is 9 UTF-16 units but 10 bytes; offsets must count
			// UTF-16 units.
			name: "multi-byte text before link",
			text: "xin chào https://mezon.ai",
			want: []*MessageMarkup{{Type: MarkupTypeLink, S: 9, E: 25}},
		},
		{
			// "🎉 " is 3 UTF-16 units (the astral emoji takes two) but 2
			// runes; offsets must match JavaScript's "🎉 ".length === 3.
			name: "astral emoji before link",
			text: "🎉 https://mezon.ai",
			want: []*MessageMarkup{{Type: MarkupTypeLink, S: 3, E: 19}},
		},
		{
			name: "trailing punctuation excluded",
			text: "go to https://mezon.ai.",
			want: []*MessageMarkup{{Type: MarkupTypeLink, S: 6, E: 22}},
		},
		{
			name: "multiple links",
			text: "https://a.io and http://b.io",
			want: []*MessageMarkup{
				{Type: MarkupTypeLink, S: 0, E: 12},
				{Type: MarkupTypeLink, S: 17, E: 28},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractLinks(tt.text); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractLinks(%q) = %+v, want %+v", tt.text, got, tt.want)
			}
		})
	}
}

func TestMessageContent(t *testing.T) {
	text := "see https://mezon.ai"
	wantMk := []*MessageMarkup{{Type: MarkupTypeLink, S: 4, E: 20}}

	t.Run("string gets link markup", func(t *testing.T) {
		want := &MessageContent{T: text, Mk: wantMk}
		if got := messageContent(text, false); !reflect.DeepEqual(got, want) {
			t.Errorf("messageContent = %+v, want %+v", got, want)
		}
	})

	t.Run("hideLink passes through", func(t *testing.T) {
		if got := messageContent(text, true); got != any(text) {
			t.Errorf("messageContent(hideLink) = %+v, want %q", got, text)
		}
	})

	t.Run("string map gets link markup", func(t *testing.T) {
		in := map[string]string{"t": text}
		want := map[string]any{"t": text, "mk": wantMk}
		if got := messageContent(in, false); !reflect.DeepEqual(got, want) {
			t.Errorf("messageContent = %+v, want %+v", got, want)
		}
	})

	t.Run("map without links unchanged", func(t *testing.T) {
		in := map[string]string{"t": "hello"}
		if got := messageContent(in, false); !reflect.DeepEqual(got, any(in)) {
			t.Errorf("messageContent = %+v, want %+v", got, in)
		}
	})

	t.Run("existing mk untouched", func(t *testing.T) {
		in := map[string]any{"t": text, "mk": []any{"custom"}}
		if got := messageContent(in, false); !reflect.DeepEqual(got, any(in)) {
			t.Errorf("messageContent = %+v, want %+v", got, in)
		}
	})

	t.Run("MessageContent gets mk filled without mutating input", func(t *testing.T) {
		in := &MessageContent{T: text}
		got, ok := messageContent(in, false).(*MessageContent)
		if !ok || !reflect.DeepEqual(got.Mk, wantMk) {
			t.Errorf("messageContent = %+v, want Mk %+v", got, wantMk)
		}
		if in.Mk != nil {
			t.Errorf("input mutated: %+v", in.Mk)
		}
	})
}

func TestContentBuilderUTF16(t *testing.T) {
	// "🎉 chúc mừng " is 13 UTF-16 units (the astral emoji takes two) but 12
	// runes and 18 bytes; offsets must match JavaScript string indices.
	b := NewContentBuilder()
	b.Text("🎉 chúc mừng ").
		MentionUser("123", "@alice").
		Text(" 🚀 ").
		Bold("xong")

	mentions := b.Mentions()
	if len(mentions) != 1 || mentions[0].S != 13 || mentions[0].E != 19 {
		t.Errorf("mention = %+v, want 13..19", mentions[0])
	}

	// " 🚀 " after "@alice" (ends at 19) is 4 more UTF-16 units.
	wantMk := []*MessageMarkup{{Type: MarkupTypeBold, S: 23, E: 27}}
	if got := b.Content().Mk; !reflect.DeepEqual(got, wantMk) {
		t.Errorf("Mk = %+v, want %+v", got, wantMk)
	}
}

func TestContentBuilder(t *testing.T) {
	// "Chào " is 5 UTF-16 units but 6 bytes, so every offset after it proves
	// the builder counts UTF-16 units, not bytes.
	b := NewContentBuilder()
	b.Text("Chào ").
		MentionUser("123", "@alice").
		Text(" và ").
		MentionHere().
		Text(", xem ").
		Link("https://mezon.ai").
		Text(" tại ").
		Hashtag("456", "#general").
		Text(" ").
		Emoji("789", ":smile:").
		Text(" ").
		Bold("quan trọng")

	content := b.Content()
	wantText := "Chào @alice và @here, xem https://mezon.ai tại #general :smile: quan trọng"
	if content.T != wantText {
		t.Fatalf("T = %q, want %q", content.T, wantText)
	}

	wantMk := []*MessageMarkup{
		{Type: MarkupTypeLink, S: 26, E: 42},
		{Type: MarkupTypeBold, S: 64, E: 74},
	}
	if !reflect.DeepEqual(content.Mk, wantMk) {
		t.Errorf("Mk = %+v, want %+v", content.Mk, wantMk)
	}

	wantHg := []*MessageHashtag{{ChannelID: "456", S: 47, E: 55}}
	if !reflect.DeepEqual(content.Hg, wantHg) {
		t.Errorf("Hg = %+v, want %+v", content.Hg, wantHg)
	}

	wantEj := []*MessageEmoji{{EmojiID: "789", S: 56, E: 63}}
	if !reflect.DeepEqual(content.Ej, wantEj) {
		t.Errorf("Ej = %+v, want %+v", content.Ej, wantEj)
	}

	mentions := b.Mentions()
	if len(mentions) != 2 {
		t.Fatalf("Mentions = %+v, want 2 entries", mentions)
	}
	if mentions[0].UserID != "123" || mentions[0].S != 5 || mentions[0].E != 11 {
		t.Errorf("user mention = %+v, want UserID 123 at 5..11", mentions[0])
	}
	if mentions[1].UserID != MentionHereUserID || mentions[1].S != 15 || mentions[1].E != 20 {
		t.Errorf("here mention = %+v, want sentinel at 15..20", mentions[1])
	}
}

func TestContentBuilderEmpty(t *testing.T) {
	content := NewContentBuilder().Content()
	if content.T != "" || content.Mk != nil || content.Hg != nil || content.Ej != nil {
		t.Errorf("empty builder content = %+v, want zero value", content)
	}
	if mentions := NewContentBuilder().Mentions(); mentions != nil {
		t.Errorf("empty builder mentions = %+v, want nil", mentions)
	}
}

func TestContentJSONFieldNames(t *testing.T) {
	// The wire field names the Mezon clients expect: "channelId" (camelCase)
	// for hashtags, "emojiid" (lowercase) for emojis, abbreviated names for
	// images.
	data, err := json.Marshal(&MessageContent{
		T:  "x",
		Mk: []*MessageMarkup{{Type: MarkupTypeLink, E: 1}},
		Hg: []*MessageHashtag{{ChannelID: "1", E: 1}},
		Ej: []*MessageEmoji{{EmojiID: "2", E: 1}},
		Images: []*MessageImage{{
			Filename: "dog.jpg", Size: 5620, URL: "https://cdn.test/dog.jpg",
			Filetype: "image/jpeg", Width: 275, Height: 183,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"t":"x","mk":[{"type":"lk","e":1}],"hg":[{"channelId":"1","e":1}],"ej":[{"emojiid":"2","e":1}],` +
		`"images":[{"fn":"dog.jpg","sz":5620,"url":"https://cdn.test/dog.jpg","ft":"image/jpeg","w":275,"h":183}]}`
	if string(data) != want {
		t.Errorf("json = %s, want %s", data, want)
	}
}
