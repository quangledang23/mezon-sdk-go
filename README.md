# Mezon SDK for Go

A lightweight Go SDK for [Mezon](https://mezon.ai) chat, ported from the
TypeScript package [`mezon-sdk`](../mezon-sdk) (in `mezonai/mezon-js`). It
mirrors the [`mezon-light-sdk-go`](https://github.com/quangledang23/mezon-light-sdk-go)
reference: methods take a `context.Context` and return errors, optional
parameters live in option structs, snowflake IDs are kept as decimal strings,
and message-content offsets are UTF-16 code units (JavaScript string indices,
so an emoji like 🎉 counts as 2) — matching how the Mezon clients index text.

## Features

- Authenticate as a bot with a bot ID + API key, with an ID token, or
  restore a session from stored tokens
- Refresh sessions (single-flight; concurrent callers share one refresh)
- Create DM / group DM channels
- Upload attachments
- Realtime messaging over WebSocket using the protobuf wire protocol
  (join/leave channels, send/receive messages, heartbeat with automatic
  dead-connection detection)
- Rich message content: clickable links (auto-detected or explicit),
  code/bold markup, user/role/`@here` mentions, channel hashtags and
  custom emojis via `ContentBuilder` — text offsets handled for you
- Incoming messages arrive with content, mentions and attachments decoded

## Installation

```sh
go get github.com/quangledang23/mezon-sdk-go
```

## Quick start

```go
package main

import (
	"context"
	"log"

	"github.com/quangledang23/mezon-sdk-go"
)

func main() {
	ctx := context.Background()

	// Authenticate as a bot with credentials from the Mezon developer portal…
	client, err := mezonlightsdk.AuthenticateBot(ctx, mezonlightsdk.AuthenticateBotConfig{
		BotID:  "your-bot-id",
		APIKey: "your-bot-token",
	})
	if err != nil {
		log.Fatal(err)
	}

	// …or with an ID token from an identity provider:
	// client, err := mezonlightsdk.Authenticate(ctx, mezonlightsdk.AuthenticateConfig{
	// 	IDToken:  "id-token-from-provider",
	// 	UserID:   "user-123",
	// 	Username: "johndoe",
	// })

	// …or restore from previously stored tokens:
	// client, err := mezonlightsdk.InitClient(mezonlightsdk.ClientInitConfig{
	// 	Token:        "your-token",
	// 	RefreshToken: "your-refresh-token",
	// 	APIURL:       "https://api.mezon.ai",
	// 	WSURL:        "gw.mezon.ai",
	// 	UserID:       "user-123",
	// })

	// Create a DM channel.
	channel, err := client.CreateDM(ctx, "peer-user-id")
	if err != nil {
		log.Fatal(err)
	}

	// Connect the realtime socket.
	socket := mezonlightsdk.NewLightSocket(client, client.Session())
	err = socket.Connect(ctx, mezonlightsdk.SocketConnectOptions{
		OnError:      func(err error) { log.Println("socket error:", err) },
		OnDisconnect: func() { log.Println("disconnected") },
	})
	if err != nil {
		log.Fatal(err)
	}
	defer socket.Disconnect()

	// Receive messages. The returned function unsubscribes the handler.
	unsubscribe := socket.OnChannelMessage(func(msg *mezonlightsdk.ChannelMessage) {
		log.Printf("received from %s: %v", msg.Username, msg.Content)
	})
	defer unsubscribe()

	// Join the DM channel and send a message.
	if err := socket.JoinDMChannel(ctx, channel.ChannelID); err != nil {
		log.Fatal(err)
	}
	// URLs in plain text become clickable links automatically
	// (set HideLink: true to keep them plain).
	err = socket.SendDM(ctx, mezonlightsdk.SendMessagePayload{
		ChannelID: channel.ChannelID,
		Content:   "Hello! Docs: https://mezon.ai/docs/developer",
	})
	if err != nil {
		log.Fatal(err)
	}

	select {} // keep the process alive to receive messages
}
```

### Sending to a clan channel

`LightSocket` covers DMs and group DMs. For clan channels, use the underlying
`DefaultSocket` directly:

```go
sock, _ := socket.Socket()

// Channel type 1 = text channel; mode 2 = clan channel message.
_, err = sock.JoinChat(ctx, clanID, channelID, 1, true)
ack, err := sock.WriteChatMessage(ctx, clanID, channelID, 2, true,
	mezonlightsdk.NewTextContent("Hello clan! https://mezon.ai"), nil)
```

### Rich content: links, markup, mentions, hashtags, emojis

Mezon messages carry plain text (`t`) plus position-based tokens: `mk`
(markup: links, code, bold), `hg` (channel hashtags) and `ej` (custom
emojis) inside the content, and a `mentions` array next to it. All offsets
are UTF-16 code units (JavaScript string indices, as the Mezon clients
count them — an emoji like 🎉 counts as 2). `ContentBuilder` assembles them
so you never count offsets by hand:

```go
b := mezonlightsdk.NewContentBuilder()
b.Text("Deploy xong ").
	MentionHere().              // "@here", notifies the channel (blue mention)
	Text(", chi tiết: ").
	Link("https://ci.example.com").
	Text(" tại ").
	Hashtag(channelID, "#deploys").
	Text(" — ").
	Bold("quan trọng")

ack, err := sock.WriteChatMessage(ctx, clanID, channelID, 2, true,
	b.Content(), &mezonlightsdk.ChatMessageOptions{
		Mentions:        b.Mentions(),
		MentionEveryone: true, // makes @here actually notify everyone
	})
```

Also available: `MentionUser(userID, "@alice")`, `MentionRole(roleID,
"@admins")` (renders green), `Code("...")`, `Emoji(emojiID, ":smile:")`,
`Markup(markupType, text)` for the remaining `mk` types (`MarkupTypePre`,
`MarkupTypeTriple`, `MarkupTypeSingle`, `MarkupTypeCode`,
`MarkupTypeVoiceLink`), and inline images (webhook-style `images` field,
e.g. with a URL from `UploadAttachment`):

```go
b.Image(&mezonlightsdk.MessageImage{
	Filename: "dog.jpg",
	URL:      "https://cdn.mezon.vn/.../dog.jpg",
	Filetype: "image/jpeg",
	Width:    275,
	Height:   183,
})
```

Note on `@here`: clients render a mention as a blue user mention only when
its `user_id` is the sentinel `mezonlightsdk.MentionHereUserID`
(`"1775731111020111321"`, hardcoded in the official clients); a mention
without a user ID falls into the role-mention path and renders green.
`MentionHere()` handles this for you.

### Receiving messages

Incoming messages arrive decoded: `Content` is the parsed content JSON,
`Mentions` and `Attachments` are typed slices (the server sends them as
protobuf or JSON; both are handled):

```go
socket.OnChannelMessage(func(msg *mezonlightsdk.ChannelMessage) {
	content, _ := msg.Content.(map[string]any)
	text, _ := content["t"].(string)
	log.Printf("%s: %s (mentions: %d)", msg.Username, text, len(msg.Mentions))
})
```

### Uploading attachments

```go
result, err := client.UploadAttachment(ctx, &mezonlightsdk.ApiUploadAttachmentRequest{
	Filename: "image.png",
	Filetype: "image/png",
	Size:     1024,
	Width:    800,
	Height:   600,
})
// result.URL can be used in message attachments.
```

### Session management

```go
// Refresh before the token expires.
if client.IsSessionExpired() {
	if _, err := client.RefreshSession(ctx); err != nil {
		log.Fatal(err)
	}
}

// Persist the session for later restoration via InitClient.
config := client.ExportSession()
```

## Package layout

| Path        | Contents                                                            |
| ----------- | ------------------------------------------------------------------- |
| `.` (root)  | `LightClient`, `LightSocket`, `DefaultSocket`, `MezonApi`, `Session`; `content.go` (outgoing content: `ContentBuilder`, markup/hashtag/emoji/image tokens, link extraction), `message.go` (incoming `ChannelMessage` decoding) |
| `proto`     | Hand-written protobuf wire codecs for the `mezon.api` and `mezon.realtime` messages used by the SDK (field numbers mirror the ts-proto generated code in `mezon-light-sdk/src/proto`) |

## Differences from the TypeScript SDK

- Methods take a `context.Context` and return `error` instead of promises.
- `WriteChatMessage` collects its many optional parameters in
  `ChatMessageOptions`.
- Callbacks (`OnChannelMessage`, `OnDisconnect`, …) are struct fields set
  before `Connect`.
- Snowflake IDs remain `string` in Go structs and are converted to/from
  int64 varints on the wire, matching ts-proto's int64-as-string behavior
  (an ID of `"0"` or `""` is omitted from the wire).
