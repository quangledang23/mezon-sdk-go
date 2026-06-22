// Command example is a minimal Mezon bot using the Go SDK. It logs in, then
// replies to every "ping" message with "pong", and demonstrates a mention whose
// offsets are computed in UTF-16 code units (so they line up with the JS/web
// clients even when the text contains emoji or CJK characters).
//
// Usage:
//
//	MEZON_BOT_ID=... MEZON_BOT_TOKEN=... go run ./example
package main

import (
	"log"
	"os"
	"strings"

	mezon "github.com/quangledang23/mezon-sdk-go"
)

func main() {
	botID := os.Getenv("MEZON_BOT_ID")
	token := os.Getenv("MEZON_BOT_TOKEN")
	if botID == "" || token == "" {
		log.Fatal("set MEZON_BOT_ID and MEZON_BOT_TOKEN")
	}

	client, err := mezon.NewMezonClient(mezon.ClientConfig{
		BotID: botID,
		Token: token,
	})
	if err != nil {
		log.Fatal(err)
	}

	client.OnReady(func() {
		log.Printf("logged in as %s; %d clans cached", client.ClientID, client.Clans.Size())
	})

	client.OnChannelMessage(func(m *mezon.ChannelMessage) {
		// Ignore our own messages.
		if m.SenderID == client.ClientID {
			return
		}
		text := m.ContentText()
		log.Printf("[%s] %s: %s", m.ChannelID, m.Username, text)

		channel, err := client.Channels.Fetch(m.ChannelID)
		if err != nil {
			log.Printf("fetch channel: %v", err)
			return
		}

		switch {
		case strings.EqualFold(strings.TrimSpace(text), "ping"):
			if _, err := channel.Send(mezon.Text("pong"), nil); err != nil {
				log.Printf("send: %v", err)
			}

		case strings.HasPrefix(text, "!hello"):
			// Build a mention of the sender. The S/E offsets MUST be UTF-16
			// code-unit indices into the reply text — here we compute them with
			// MentionSpan so a leading emoji shifts the offset correctly.
			handle := "@" + firstNonEmptyName(m)
			reply := "👋 " + handle + " welcome!"
			s, e, ok := mezon.MentionSpan(reply, handle)
			var opts *mezon.SendOptions
			if ok {
				opts = &mezon.SendOptions{Mentions: []mezon.Mention{{
					UserID:   m.SenderID,
					Username: firstNonEmptyName(m),
					S:        s,
					E:        e,
				}}}
			}
			if _, err := channel.Send(mezon.Text(reply), opts); err != nil {
				log.Printf("send: %v", err)
			}
		}
	})

	if err := client.Login(); err != nil {
		log.Fatalf("login: %v", err)
	}

	// Block forever; the SDK runs its read/heartbeat loops in goroutines.
	select {}
}

func firstNonEmptyName(m *mezon.ChannelMessage) string {
	for _, v := range []string{m.ClanNick, m.DisplayName, m.Username} {
		if v != "" {
			return v
		}
	}
	return m.SenderID
}
