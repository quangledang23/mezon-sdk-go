package mezon

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Content is a message content payload. It is serialized to JSON and sent as
// the message's `content` string on the wire (mirroring `content: any` in the
// TS SDK). Use Text for a plain text message, or pass any struct/map for
// embeds, components, buzz, etc.
type Content = any

// Text builds a plain-text message content ({"t": s}).
func Text(s string) map[string]any {
	return map[string]any{"t": s}
}

// maxContentLength is the server limit enforced client-side, mirroring
// socket_manager.ts (8000 characters, measured in UTF-16 code units).
const maxContentLength = 8000

// marshalContent serializes content the same way the JS SDK does
// (JSON.stringify), without Go's default HTML escaping of <, > and &, so the
// produced JSON string matches what a JS bot would send.
func marshalContent(content Content) (string, error) {
	if content == nil {
		content = map[string]any{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(content); err != nil {
		return "", err
	}
	// json.Encoder appends a trailing newline; strip it to match JSON.stringify.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return string(out), nil
}

// validateContentLength enforces the 8000 UTF-16 code-unit limit, measuring
// exactly like the JS SDK's `JSON.stringify(content).length` check. This is the
// "push message in UTF-16" requirement: the length the server validates against
// is the JS string length (UTF-16 code units) of the JSON content, not its byte
// or rune count.
func validateContentLength(jsonContent string) error {
	if l := UTF16Len(jsonContent); l > maxContentLength {
		return fmt.Errorf(
			"message.content exceeds the allowed length! Maximum total of %d characters. Current length: %d",
			maxContentLength, l,
		)
	}
	return nil
}
