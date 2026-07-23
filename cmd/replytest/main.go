// Command replytest is a live probe for sending/replying into a PRIVATE
// channel over the default abridged-TCP transport. It uses the low-level auth
// path (MezonAuthenticate → socket) because client.Login() 403s on
// ListClanDescs for the test bot.
//
// It sends WITHOUT pre-joining the channel first, so the "error 3: Must join
// channel before sending messages" retry path (join + resend) is exercised,
// then replies to its own message.
//
// Usage:
//
//	MEZON_BOT_ID=... MEZON_BOT_TOKEN=... [MEZON_TEST_CLAN=...] [MEZON_TEST_CHANNEL=...] go run ./cmd/replytest
package main

import (
	"fmt"
	"log"
	"os"

	mezon "github.com/quangledang23/mezon-sdk-go"
	"github.com/quangledang23/mezon-sdk-go/api"
)

func main() {
	botID := os.Getenv("MEZON_BOT_ID")
	token := os.Getenv("MEZON_BOT_TOKEN")
	clanID := os.Getenv("MEZON_TEST_CLAN")
	channelID := os.Getenv("MEZON_TEST_CHANNEL")
	if clanID == "" {
		clanID = "1975864750969458688" // test-bot clan
	}
	if channelID == "" {
		channelID = "2073273073817096192" // test-bot channel (private)
	}
	if botID == "" || token == "" {
		log.Fatal("set MEZON_BOT_ID and MEZON_BOT_TOKEN")
	}

	loginApi := mezon.NewMezonApi(token, "https://gw.mezon.ai:443", 0)
	sessApi, err := loginApi.MezonAuthenticate(botID, token)
	if err != nil {
		log.Fatalf("authenticate: %v", err)
	}
	log.Printf("authenticated: user=%d api_url=%s tcp_url=%s", sessApi.UserId, sessApi.ApiUrl, sessApi.TcpUrl)

	sess, err := mezon.NewSession(sessApi.Token, sessApi.RefreshToken, fmt.Sprint(sessApi.UserId), sessApi.ApiUrl, sessApi.IdToken, sessApi.WsUrl)
	if err != nil {
		log.Fatalf("session: %v", err)
	}
	sess.TcpURL = sessApi.TcpUrl

	host, port, useSSL, err := mezon.ParseURLToHostAndSSL(sessApi.ApiUrl)
	if err != nil {
		log.Fatalf("parse api_url: %v", err)
	}

	// Default transport = abridged TCP; TcpURL from the session is preferred.
	sock := mezon.NewDefaultSocket(sessApi.WsUrl, host, port, useSSL, func(string, any) {})
	sock.TcpURL = sess.TcpURL
	if err := sock.Connect(sess, true); err != nil {
		log.Fatalf("socket connect (TCP): %v", err)
	}
	defer sock.Close()
	if _, err := sock.JoinClanChat(clanID); err != nil {
		log.Printf("JoinClanChat(%s): %v (continuing)", clanID, err)
	}

	// Debug: what does the channel look like, and does an explicit join work?
	restApi := mezon.NewMezonApi(token, "https://"+host+":"+port, 0)
	restApi.AttachSocket(sock)
	if detail, err := restApi.ListChannelDetail(sess.Token, channelID); err != nil {
		log.Printf("ListChannelDetail(%s): %v", channelID, err)
	} else {
		log.Printf("channel detail: id=%d label=%q type=%d private=%d parent=%d",
			detail.GetChannelId(), detail.GetChannelLabel(), detail.GetType(), detail.GetChannelPrivate(), detail.GetParentId())
	}
	// Try to add the bot itself to the private channel, then join. Skip with
	// MEZON_SKIP_JOIN=1 to exercise WriteChatMessage's must-join auto-retry.
	if os.Getenv("MEZON_SKIP_JOIN") == "" {
		if err := restApi.Call(sess.Token, "AddChannelUsers", &api.AddChannelUsersRequest{
			ChannelId: mustID(channelID),
			UserIds:   []int64{mustID(botID)},
		}, nil); err != nil {
			log.Printf("AddChannelUsers(self): %v", err)
		} else {
			log.Printf("AddChannelUsers(self) OK")
		}
		if ch, err := sock.JoinChat(clanID, channelID, 1, false); err != nil {
			log.Printf("JoinChat(private) after add: %v", err)
		} else {
			log.Printf("JoinChat(private) OK: %+v", ch)
		}
	} else {
		log.Print("MEZON_SKIP_JOIN set — relying on WriteChatMessage auto-join retry")
	}

	// Control test: find a PUBLIC channel in the clan and send+reply there, so
	// TCP send/reply mechanics are verified independently of private-channel
	// membership.
	if descs, err := restApi.ListChannelDescs(sess.Token, 1, clanID, 0, 0, "", false); err != nil {
		log.Printf("ListChannelDescs(clan): %v", err)
	} else {
		for _, d := range descs.GetChanneldesc() {
			if d.GetChannelId() == 0 || d.GetChannelPrivate() != 0 {
				continue
			}
			pubID := fmt.Sprint(d.GetChannelId())
			log.Printf("control: public channel id=%s label=%q", pubID, d.GetChannelLabel())
			ack, err := sock.WriteChatMessage(mezon.ReplyMessageData{
				ClanID: clanID, ChannelID: pubID, Mode: int(mezon.StreamModeChannel), IsPublic: true,
				Content: mezon.Text("replytest: control send to public channel over TCP"),
			})
			if err != nil {
				log.Printf("control send: %v", err)
				break
			}
			log.Printf("control SENT ok: message_id=%d", ack.MessageId)
			rack, err := sock.WriteChatMessage(mezon.ReplyMessageData{
				ClanID: clanID, ChannelID: pubID, Mode: int(mezon.StreamModeChannel), IsPublic: true,
				Content: mezon.Text("replytest: control reply over TCP"),
				References: []mezon.MessageRef{{
					MessageRefID:    fmt.Sprint(ack.MessageId),
					MessageSenderID: botID,
					Content:         `{"t":"replytest: control send to public channel over TCP"}`,
				}},
			})
			if err != nil {
				log.Printf("control reply: %v", err)
			} else {
				log.Printf("control REPLIED ok: message_id=%d", rack.MessageId)
			}
			break
		}
	}

	// 1. Send into the private channel WITHOUT JoinChat first: exercises the
	// error-3 join-then-retry path inside WriteChatMessage.
	send := mezon.ReplyMessageData{
		ClanID:    clanID,
		ChannelID: channelID,
		Mode:      int(mezon.StreamModeChannel),
		IsPublic:  false, // private channel
		Content:   mezon.Text("replytest: send over TCP into private channel"),
	}
	ack, err := sock.WriteChatMessage(send)
	if err != nil {
		log.Fatalf("SEND FAILED (private channel over TCP): %v", err)
	}
	if ack == nil || ack.MessageId == 0 {
		log.Fatalf("send returned empty ack: %+v", ack)
	}
	msgID := fmt.Sprint(ack.MessageId)
	log.Printf("SENT ok: message_id=%s channel=%d", msgID, ack.ChannelId)

	// 2. Reply to that message (references set, still is_public=false).
	reply := mezon.ReplyMessageData{
		ClanID:    clanID,
		ChannelID: channelID,
		Mode:      int(mezon.StreamModeChannel),
		IsPublic:  false,
		Content:   mezon.Text("replytest: reply over TCP"),
		References: []mezon.MessageRef{{
			MessageRefID:    msgID,
			MessageSenderID: fmt.Sprint(sessApi.UserId),
			Content:         `{"t":"replytest: send over TCP into private channel"}`,
		}},
	}
	rack, err := sock.WriteChatMessage(reply)
	if err != nil {
		log.Fatalf("REPLY FAILED (private channel over TCP): %v", err)
	}
	if rack == nil || rack.MessageId == 0 {
		log.Fatalf("reply returned empty ack: %+v", rack)
	}
	log.Printf("REPLIED ok: message_id=%d", rack.MessageId)
	log.Print("replytest OK — private channel send+reply works over TCP")
}

func mustID(s string) int64 {
	var v int64
	if _, err := fmt.Sscan(s, &v); err != nil {
		log.Fatalf("bad id %q: %v", s, err)
	}
	return v
}
