package mezon

import "testing"

func TestUTF16Len(t *testing.T) {
	cases := []struct {
		in   string
		want int // JavaScript "...".length
	}{
		{"", 0},
		{"hello", 5},
		{"héllo", 5}, // é is one UTF-16 code unit
		{"日本語", 3},   // CJK in BMP, one unit each
		{"😀", 2},     // astral plane -> surrogate pair
		{"a😀b", 4},   // 1 + 2 + 1
		{"hi 👋🏽", 7}, // wave + skin tone modifier are two astral graphemes
	}
	for _, c := range cases {
		if got := UTF16Len(c.in); got != c.want {
			t.Errorf("UTF16Len(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMentionSpan(t *testing.T) {
	// Plain ASCII.
	if s, e, ok := MentionSpan("hi @bob!", "@bob"); !ok || s != 3 || e != 7 {
		t.Errorf("ascii: got (%d,%d,%v), want (3,7,true)", s, e, ok)
	}
	// An emoji before the mention shifts the UTF-16 offset by 2, not 1 (byte)
	// or 1 (rune). This is the exact failure a naive port would hit.
	if s, e, ok := MentionSpan("😀 @bob", "@bob"); !ok || s != 3 || e != 7 {
		t.Errorf("emoji-prefixed: got (%d,%d,%v), want (3,7,true)", s, e, ok)
	}
	if _, _, ok := MentionSpan("no mention here", "@bob"); ok {
		t.Errorf("missing: expected ok=false")
	}
}

func TestValidateContentLength(t *testing.T) {
	// 4001 astral chars -> 8002 UTF-16 code units -> over the 8000 limit, even
	// though it is only 4001 runes.
	big := ""
	for i := 0; i < 4001; i++ {
		big += "😀"
	}
	if err := validateContentLength(big); err == nil {
		t.Errorf("expected over-length error for %d UTF-16 units", UTF16Len(big))
	}
	if err := validateContentLength("short"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMarshalContentNoHTMLEscape(t *testing.T) {
	got, err := marshalContent(map[string]any{"t": "a<b>&c"})
	if err != nil {
		t.Fatal(err)
	}
	// JS JSON.stringify does not escape <, > or &.
	want := `{"t":"a<b>&c"}`
	if got != want {
		t.Errorf("marshalContent = %q, want %q", got, want)
	}
}
