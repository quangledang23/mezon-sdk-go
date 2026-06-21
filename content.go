package mezonlightsdk

import (
	"regexp"
	"strings"
)

// Markup types for MessageMarkup.Type (EBacktickType in the Mezon web app).
const (
	// MarkupTypeLink renders the covered text as a clickable link.
	MarkupTypeLink = "lk"
	// MarkupTypePre renders the covered text as a preformatted block.
	MarkupTypePre = "pre"
	// MarkupTypeTriple is a triple-backtick code block.
	MarkupTypeTriple = "t"
	// MarkupTypeSingle is a single-backtick inline code span.
	MarkupTypeSingle = "s"
	// MarkupTypeCode is an inline code span.
	MarkupTypeCode = "c"
	// MarkupTypeBold renders the covered text in bold.
	MarkupTypeBold = "b"
	// MarkupTypeVoiceLink is a link to a voice room.
	MarkupTypeVoiceLink = "vk"
)

// MessageMarkup is one markup token of message content ("mk"); S and E are
// UTF-16 code unit offsets (JavaScript string indices, not bytes) into the
// content text.
type MessageMarkup struct {
	Type string `json:"type"`
	S    int32  `json:"s,omitempty"`
	E    int32  `json:"e,omitempty"`
}

// MessageHashtag is one channel reference of message content ("hg"); S and E
// are UTF-16 code unit offsets (JavaScript string indices, not bytes) into
// the content text.
type MessageHashtag struct {
	ChannelID string `json:"channelId"`
	S         int32  `json:"s,omitempty"`
	E         int32  `json:"e,omitempty"`
}

// MessageEmoji is one custom-emoji token of message content ("ej"); S and E
// are UTF-16 code unit offsets (JavaScript string indices, not bytes) into
// the content text, covering the emoji shortname (e.g. ":smile:").
type MessageEmoji struct {
	EmojiID string `json:"emojiid"`
	S       int32  `json:"s,omitempty"`
	E       int32  `json:"e,omitempty"`
}

// MessageImage is one inline image of message content ("images"), as used by
// webhook payloads; field names are abbreviated on the wire.
type MessageImage struct {
	// Filename is the image file name ("fn").
	Filename string `json:"fn,omitempty"`
	// Size is the file size in bytes ("sz").
	Size int32 `json:"sz,omitempty"`
	// URL is the image URL, e.g. from UploadAttachment.
	URL string `json:"url"`
	// Filetype is the MIME type ("ft"), e.g. "image/jpeg".
	Filetype string `json:"ft,omitempty"`
	// Width and Height are the image dimensions in pixels ("w"/"h").
	Width  int32 `json:"w,omitempty"`
	Height int32 `json:"h,omitempty"`
}

// MessageContent is the structured content of a channel message, the same
// shape the Mezon clients and webhooks use ({"t": ..., "mk": [...], ...}).
type MessageContent struct {
	// T is the message text.
	T string `json:"t"`
	// Mk holds markup tokens (links, code blocks, bold, ...).
	Mk []*MessageMarkup `json:"mk,omitempty"`
	// Hg holds channel hashtag references.
	Hg []*MessageHashtag `json:"hg,omitempty"`
	// Ej holds custom emoji tokens.
	Ej []*MessageEmoji `json:"ej,omitempty"`
	// Images holds inline images (webhook-style payloads).
	Images []*MessageImage `json:"images,omitempty"`
}

var linkRegexp = regexp.MustCompile(`https?://\S+`)

// utf16Len returns the length of s in UTF-16 code units, which is how the
// Mezon clients (JavaScript) index message content.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++ // astral characters take two UTF-16 code units
		}
	}
	return n
}

// ExtractLinks finds http(s) URLs in text and returns "lk" markup tokens for
// them, which clients render as clickable links. Offsets are UTF-16 code
// units, not bytes, matching how Mezon clients index content.
func ExtractLinks(text string) []*MessageMarkup {
	var marks []*MessageMarkup
	for _, loc := range linkRegexp.FindAllStringIndex(text, -1) {
		url := strings.TrimRight(text[loc[0]:loc[1]], `.,;:!?'")]}`)
		if url == "" {
			continue
		}
		s := utf16Len(text[:loc[0]])
		marks = append(marks, &MessageMarkup{
			Type: MarkupTypeLink,
			S:    int32(s),
			E:    int32(s + utf16Len(url)),
		})
	}
	return marks
}

// NewTextContent builds message content from plain text, marking any URLs in
// it as clickable links.
func NewTextContent(text string) *MessageContent {
	return &MessageContent{T: text, Mk: ExtractLinks(text)}
}

// ContentBuilder assembles message content from text, links, code, mentions,
// hashtags and emojis, tracking the UTF-16 offsets of every token so callers
// never count them by hand:
//
//	b := mezonlightsdk.NewContentBuilder()
//	b.Text("Deploy xong ").MentionHere().Text(", chi tiết: ").Link("https://ci.example.com")
//	ack, err := sock.WriteChatMessage(ctx, clanID, channelID, 2, true,
//		b.Content(), &mezonlightsdk.ChatMessageOptions{Mentions: b.Mentions()})
type ContentBuilder struct {
	sb       strings.Builder
	length   int // UTF-16 code units written, not bytes
	mk       []*MessageMarkup
	hg       []*MessageHashtag
	ej       []*MessageEmoji
	images   []*MessageImage
	mentions []*ApiMessageMention
}

// NewContentBuilder creates an empty ContentBuilder.
func NewContentBuilder() *ContentBuilder { return &ContentBuilder{} }

// append writes s to the text and returns its UTF-16 code unit offsets.
func (b *ContentBuilder) append(s string) (start, end int32) {
	start = int32(b.length)
	b.sb.WriteString(s)
	b.length += utf16Len(s)
	return start, int32(b.length)
}

// Text appends plain text.
func (b *ContentBuilder) Text(s string) *ContentBuilder {
	b.append(s)
	return b
}

// Markup appends text covered by a markup token of the given type
// (MarkupTypeBold, MarkupTypeCode, ...).
func (b *ContentBuilder) Markup(markupType, text string) *ContentBuilder {
	s, e := b.append(text)
	b.mk = append(b.mk, &MessageMarkup{Type: markupType, S: s, E: e})
	return b
}

// Link appends a URL rendered as a clickable link.
func (b *ContentBuilder) Link(url string) *ContentBuilder {
	return b.Markup(MarkupTypeLink, url)
}

// Bold appends bold text.
func (b *ContentBuilder) Bold(text string) *ContentBuilder {
	return b.Markup(MarkupTypeBold, text)
}

// Code appends a preformatted code block.
func (b *ContentBuilder) Code(text string) *ContentBuilder {
	return b.Markup(MarkupTypePre, text)
}

// MentionUser appends a user mention; display is the visible text, e.g.
// "@alice".
func (b *ContentBuilder) MentionUser(userID, display string) *ContentBuilder {
	s, e := b.append(display)
	b.mentions = append(b.mentions, &ApiMessageMention{UserID: userID, S: s, E: e})
	return b
}

// MentionRole appends a role mention (rendered green by clients); display is
// the visible text, e.g. "@admins".
func (b *ContentBuilder) MentionRole(roleID, display string) *ContentBuilder {
	s, e := b.append(display)
	b.mentions = append(b.mentions, &ApiMessageMention{RoleID: roleID, S: s, E: e})
	return b
}

// MentionHere appends "@here" with the sentinel user ID, so clients render
// it as a user mention (blue) like the official clients do. Pair it with
// MentionEveryone on the send options/payload to notify the channel.
func (b *ContentBuilder) MentionHere() *ContentBuilder {
	return b.MentionUser(MentionHereUserID, MentionHereTitle)
}

// Hashtag appends a channel reference; display is the visible text, e.g.
// "#general".
func (b *ContentBuilder) Hashtag(channelID, display string) *ContentBuilder {
	s, e := b.append(display)
	b.hg = append(b.hg, &MessageHashtag{ChannelID: channelID, S: s, E: e})
	return b
}

// Emoji appends a custom emoji; display is the shortname covering the token,
// e.g. ":smile:".
func (b *ContentBuilder) Emoji(emojiID, display string) *ContentBuilder {
	s, e := b.append(display)
	b.ej = append(b.ej, &MessageEmoji{EmojiID: emojiID, S: s, E: e})
	return b
}

// Image attaches an inline image; unlike the other tokens it does not
// occupy a text range.
func (b *ContentBuilder) Image(img *MessageImage) *ContentBuilder {
	b.images = append(b.images, img)
	return b
}

// Content returns the assembled message content.
func (b *ContentBuilder) Content() *MessageContent {
	return &MessageContent{T: b.sb.String(), Mk: b.mk, Hg: b.hg, Ej: b.ej, Images: b.images}
}

// Mentions returns the assembled mention entries; they travel next to the
// content (ChatMessageOptions.Mentions or SendMessagePayload.Mentions), not
// inside it.
func (b *ContentBuilder) Mentions() []*ApiMessageMention {
	return b.mentions
}

// messageContent prepares a payload's content for sending: plain text and
// {"t": ...}-shaped content get "lk" markup added for any URLs so clients
// render them as clickable links. Content that already carries markup, other
// shapes, and everything when hideLink is set pass through unchanged.
func messageContent(content any, hideLink bool) any {
	if hideLink {
		return content
	}
	switch c := content.(type) {
	case string:
		return NewTextContent(c)
	case *MessageContent:
		if len(c.Mk) > 0 {
			return c
		}
		if mk := ExtractLinks(c.T); len(mk) > 0 {
			withMk := *c
			withMk.Mk = mk
			return &withMk
		}
	case map[string]string:
		m := make(map[string]any, len(c))
		for k, v := range c {
			m[k] = v
		}
		return mapWithLinkMarkup(m, content)
	case map[string]any:
		return mapWithLinkMarkup(c, content)
	}
	return content
}

// mapWithLinkMarkup returns a copy of m with "lk" markup added for any URLs
// in m["t"], or orig unchanged when there is nothing to add.
func mapWithLinkMarkup(m map[string]any, orig any) any {
	if _, exists := m["mk"]; exists {
		return orig
	}
	t, ok := m["t"].(string)
	if !ok {
		return orig
	}
	mk := ExtractLinks(t)
	if len(mk) == 0 {
		return orig
	}
	out := make(map[string]any, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	out["mk"] = mk
	return out
}
