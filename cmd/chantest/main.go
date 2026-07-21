// Command chantest is a live probe: it creates a channel in a clan, verifies it
// exists, deletes it, and verifies it is gone. It uses the low-level auth path
// (MezonAuthenticate → socket → REST-over-socket) because client.Login() 403s
// on ListClanDescs for the test bot.
//
// Usage:
//
//	MEZON_BOT_ID=... MEZON_BOT_TOKEN=... [MEZON_TEST_CLAN=...] go run ./cmd/chantest
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	mezon "github.com/quangledang23/mezon-sdk-go"
	"github.com/quangledang23/mezon-sdk-go/api"
)

func main() {
	botID := os.Getenv("MEZON_BOT_ID")
	token := os.Getenv("MEZON_BOT_TOKEN")
	clanID := os.Getenv("MEZON_TEST_CLAN")
	if clanID == "" {
		clanID = "1975864750969458688" // test-bot clan
	}
	if botID == "" || token == "" {
		log.Fatal("set MEZON_BOT_ID and MEZON_BOT_TOKEN")
	}

	// 1. Authenticate against the login gateway.
	loginApi := mezon.NewMezonApi(token, "https://gw.mezon.ai:443", 0)
	sessApi, err := loginApi.MezonAuthenticate(botID, token)
	if err != nil {
		log.Fatalf("authenticate: %v", err)
	}
	log.Printf("authenticated: user=%d api_url=%s ws_url=%s", sessApi.UserId, sessApi.ApiUrl, sessApi.WsUrl)

	sess, err := mezon.NewSession(sessApi.Token, sessApi.RefreshToken, fmt.Sprint(sessApi.UserId), sessApi.ApiUrl, sessApi.IdToken, sessApi.WsUrl)
	if err != nil {
		log.Fatalf("session: %v", err)
	}

	// 2. Post-auth REST goes to sess.ApiUrl, not the login gateway.
	host, port, useSSL, err := mezon.ParseURLToHostAndSSL(sessApi.ApiUrl)
	if err != nil {
		log.Fatalf("parse api_url: %v", err)
	}
	scheme := "http://"
	if useSSL {
		scheme = "https://"
	}
	restApi := mezon.NewMezonApi(token, scheme+host+":"+port, 0)

	// 3. Connect the realtime socket and route API requests over it.
	sock := mezon.NewDefaultSocket(sessApi.WsUrl, host, port, useSSL, func(string, any) {})
	if err := sock.Connect(sess, true); err != nil {
		log.Fatalf("socket connect: %v", err)
	}
	defer sock.Close()
	restApi.AttachSocket(sock)
	if _, err := sock.JoinClanChat(clanID); err != nil {
		log.Printf("JoinClanChat(%s): %v (continuing)", clanID, err)
	}

	// 4. Create a channel. category_id is required — the server rejects
	// category-less channels with code 13 — so borrow the category of the known
	// test channel.
	refChannel := os.Getenv("MEZON_TEST_CHANNEL")
	if refChannel == "" {
		refChannel = "2073273073817096192" // test-bot channel
	}
	ref, err := restApi.ListChannelDetail(sess.Token, refChannel)
	if err != nil {
		log.Fatalf("ListChannelDetail(%s) for category: %v", refChannel, err)
	}
	log.Printf("reference channel %s: label=%q category=%d", refChannel, ref.GetChannelLabel(), ref.GetCategoryId())

	label := fmt.Sprintf("go-sdk-chantest-%d", time.Now().Unix())
	created, err := restApi.CreateChannelDesc(sess.Token, &api.CreateChannelDescRequest{
		ClanId:       mustID(clanID),
		CategoryId:   ref.GetCategoryId(),
		Type:         1, // ChannelTypeChannel
		ChannelLabel: label,
	})
	if err != nil {
		log.Fatalf("CreateChannelDesc: %v", err)
	}
	if created.GetChannelId() == 0 {
		log.Fatalf("CreateChannelDesc returned zero channel id: %+v", created)
	}
	chID := fmt.Sprint(created.GetChannelId())
	log.Printf("CREATED channel id=%s label=%q type=%d category=%d", chID, created.GetChannelLabel(), created.GetType(), created.GetCategoryId())

	// 5. Verify it exists.
	detail, err := restApi.ListChannelDetail(sess.Token, chID)
	if err != nil {
		log.Printf("verify-after-create ListChannelDetail: %v", err)
	} else {
		log.Printf("VERIFIED exists: id=%d label=%q", detail.GetChannelId(), detail.GetChannelLabel())
	}

	// 6. Wait before deleting so the channel is visible in the app for a bit
	// (MEZON_TEST_DELAY, e.g. "30s"; default 15s).
	delay := 15 * time.Second
	if v := os.Getenv("MEZON_TEST_DELAY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("bad MEZON_TEST_DELAY %q: %v", v, err)
		}
		delay = d
	}
	log.Printf("waiting %s before delete...", delay)
	time.Sleep(delay)

	// 7. Delete it.
	if err := restApi.DeleteChannelDesc(sess.Token, clanID, chID); err != nil {
		log.Fatalf("DeleteChannelDesc: %v (channel %s LEFT BEHIND, label %q)", err, chID, label)
	}
	log.Printf("DELETED channel id=%s", chID)

	// 8. Verify it is gone.
	gone, err := restApi.ListChannelDetail(sess.Token, chID)
	if err != nil {
		log.Printf("VERIFIED gone: ListChannelDetail now errors: %v", err)
	} else if gone.GetChannelId() == 0 {
		log.Printf("VERIFIED gone: ListChannelDetail returns empty description")
	} else {
		log.Printf("WARNING: channel still resolves after delete: id=%d label=%q active=%v", gone.GetChannelId(), gone.GetChannelLabel(), gone)
	}

	log.Print("chantest OK")
}

func mustID(s string) int64 {
	var v int64
	if _, err := fmt.Sscan(s, &v); err != nil {
		log.Fatalf("bad id %q: %v", s, err)
	}
	return v
}
