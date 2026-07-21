// Command logintest checks whether the high-level client.Login() path works
// for the bot (it used to 403 on ListClanDescs during connectSocket), then
// exercises Clan.CreateChannel/DeleteChannel through the high-level API.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	mezon "github.com/quangledang23/mezon-sdk-go"
)

func main() {
	client, err := mezon.NewMezonClient(mezon.ClientConfig{
		BotID: os.Getenv("MEZON_BOT_ID"),
		Token: os.Getenv("MEZON_BOT_TOKEN"),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := client.Login(); err != nil {
		log.Fatalf("Login: %v", err)
	}
	defer client.Close()
	log.Printf("Login OK: %d clans cached", client.Clans.Size())
	for _, c := range client.Clans.Values() {
		log.Printf("  clan %s %q welcome=%s", c.ID, c.Name, c.WelcomeChannelID)
	}

	clanID := os.Getenv("MEZON_TEST_CLAN")
	if clanID == "" {
		clanID = "1975864750969458688"
	}
	clan, ok := client.Clans.Get(clanID)
	if !ok {
		log.Fatalf("clan %s not cached", clanID)
	}

	// The server rejects category-less channels (code 13), so borrow the
	// category of the known test channel.
	refChannel := os.Getenv("MEZON_TEST_CHANNEL")
	if refChannel == "" {
		refChannel = "2073273073817096192"
	}
	ref, err := client.Channels.Fetch(refChannel)
	if err != nil {
		log.Fatalf("fetch reference channel: %v", err)
	}
	log.Printf("reference channel %q category=%s", ref.Name, ref.CategoryID)

	label := fmt.Sprintf("go-sdk-hltest-%d", time.Now().Unix())
	created, err := clan.CreateChannel(mezon.CreateChannelData{
		Label:      label,
		CategoryID: ref.CategoryID,
	})
	if err != nil {
		log.Fatalf("Clan.CreateChannel: %v", err)
	}
	chID := fmt.Sprint(created.GetChannelId())
	log.Printf("CREATED via Clan.CreateChannel: id=%s label=%q", chID, created.GetChannelLabel())
	if _, ok := clan.Channels.Get(chID); !ok {
		log.Printf("WARNING: created channel not in clan cache")
	}

	if err := clan.DeleteChannel(chID); err != nil {
		log.Fatalf("Clan.DeleteChannel: %v (channel %s LEFT BEHIND, label %q)", err, chID, label)
	}
	log.Printf("DELETED via Clan.DeleteChannel: id=%s", chID)
	if _, ok := clan.Channels.Get(chID); ok {
		log.Printf("WARNING: deleted channel still in clan cache")
	}
	log.Print("logintest OK")
}
