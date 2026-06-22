# mezon-go

A Go port of the [mezon-sdk](../mezon-sdk) TypeScript client for the Mezon
server. It speaks the same protobuf-over-WebSocket realtime protocol and the same
protobuf REST API, so a Go bot behaves like a JS bot.

```go
import mezon "github.com/quangledang23/mezon-sdk-go"
```

## Getting started

```go
client, err := mezon.NewMezonClient(mezon.ClientConfig{
    BotID: os.Getenv("MEZON_BOT_ID"),
    Token: os.Getenv("MEZON_BOT_TOKEN"),
})
if err != nil { log.Fatal(err) }

client.OnChannelMessage(func(m *mezon.ChannelMessage) {
    if m.ContentText() == "ping" {
        ch, _ := client.Channels.Fetch(m.ChannelID)
        ch.Send(mezon.Text("pong"), nil)
    }
})

if err := client.Login(); err != nil { log.Fatal(err) }
select {} // the SDK runs its read/heartbeat loops in goroutines
```

A complete bot is in [`example/`](./example).

## What `Login` does

Mirrors `MezonClientCore.login`:

1. Authenticate the bot over REST (`POST /v2/apps/authenticate/token`, JSON body,
   protobuf `Session` response).
2. Decode the JWT for expiry/vars, and re-target host/ws from the session's
   `api_url`/`ws_url`.
3. Open the protobuf WebSocket (`/ws?...&format=protobuf`), then join every clan
   chat and build the clan/channel/user caches.

## Sending messages and UTF-16

> **Message offsets are UTF-16 code units.** The Mezon web/JS clients are
> JavaScript, so message-content length and mention `s`/`e` (start/end) offsets
> are measured in UTF-16 code units, not bytes or runes. This port does the
> same:
>
> - The 8000-character content limit is checked with `UTF16Len(JSON(content))`,
>   exactly matching the JS `JSON.stringify(content).length` guard.
> - Use `mezon.MentionSpan(text, substr)` to compute a mention's `S`/`E` so a
>   leading emoji or CJK text shifts the offset by the right amount.

```go
reply := "👋 @bob welcome!"
s, e, _ := mezon.MentionSpan(reply, "@bob") // s=3, e=7 (the emoji is 2 units)
ch.Send(mezon.Text(reply), &mezon.SendOptions{
    Mentions: []mezon.Mention{{UserID: senderID, Username: "bob", S: s, E: e}},
})
```

Helpers: `UTF16Len`, `UTF16Encode`, `RuneIndexToUTF16`, `MentionSpan`.

## Message actions

`TextChannel.Send` / `SendEphemeral`, and on a cached `Message`:
`Reply`, `Update`, `React`, `Delete`. `User.SendDM` sends a direct message.
All writes are serialized through an `AsyncThrottleQueue` (80/sec, like the TS SDK).

## Events

Register with `client.On(mezon.Event<Name>, handler)` or the typed
`client.OnChannelMessage`. `channel_message` delivers a friendly
`*mezon.ChannelMessage` (content/mentions/reactions parsed); other events deliver
the decoded protobuf message pointer from the `rtapi`/`api` packages.

## Protobuf code generation

`api/*.pb.go` and `rtapi/*.pb.go` are generated, not hand-written. The `.proto`
files under `proto/` are reconstructed from the ts-proto output in the TS SDK by
`tools/tsproto2proto.js`, then compiled with `protoc`:

```bash
node tools/tsproto2proto.js
protoc -I proto -I <wkt-include> --go_out=. --go_opt=paths=source_relative \
    proto/api/api.proto proto/rtapi/realtime.proto
```

Re-run both steps after the TS SDK's protobuf changes.

## Not yet ported

These depend on external packages outside this repo and are intentionally left
out of the first port:

- **Token transfers / ZK proofs (MMN)** — the TS SDK uses the external
  `mmn-client-js` library (`MmnClient`, `ZkClient`) for `sendToken`,
  `getZkProofs`, ephemeral keypairs and nonces. Porting requires a Go port of
  that crypto library.
- **AI-agent SSE stream** — the `EventSourceManager` / agent session events.
- **Local SQLite message persistence** (`MessageDatabase`); messages are cached
  in memory only.
