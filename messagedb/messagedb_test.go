package messagedb

import (
	"encoding/json"
	"path/filepath"
	"testing"

	mezon "github.com/quangledang23/mezon-sdk-go"
)

func TestSaveAndGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "msgs.db")
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	msg := &mezon.ChannelMessage{
		MessageID:         "111",
		ChannelID:         "222",
		ClanID:            "333",
		SenderID:          "444",
		Content:           json.RawMessage(`{"t":"hello"}`),
		Mentions:          []mezon.Mention{{UserID: "444", S: 0, E: 5}},
		Attachments:       []mezon.Attachment{{Filename: "a.png", URL: "http://x/a.png"}},
		References:        []mezon.MessageRef{{MessageRefID: "999"}},
		TopicID:           "0",
		CreateTimeSeconds: 1234,
	}
	if err := db.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	got, err := db.GetMessageByID("111", "222", "333")
	if err != nil {
		t.Fatalf("GetMessageByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected message, got nil")
	}
	if got.MessageID != "111" || got.ChannelID != "222" || got.ClanID != "333" || got.SenderID != "444" {
		t.Fatalf("id fields mismatch: %+v", got)
	}
	if string(got.Content) != `{"t":"hello"}` {
		t.Fatalf("content mismatch: %s", got.Content)
	}
	if got.CreateTimeSeconds != 1234 {
		t.Fatalf("create_time_seconds mismatch: %d", got.CreateTimeSeconds)
	}
	if len(got.Mentions) != 1 || got.Mentions[0].UserID != "444" || got.Mentions[0].E != 5 {
		t.Fatalf("mentions mismatch: %+v", got.Mentions)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "a.png" {
		t.Fatalf("attachments mismatch: %+v", got.Attachments)
	}
	if len(got.References) != 1 || got.References[0].MessageRefID != "999" {
		t.Fatalf("references mismatch: %+v", got.References)
	}
}

func TestGetMissingReturnsNil(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "msgs.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	got, err := db.GetMessageByID("nope", "x", "y")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing, got %+v", got)
	}
}

func TestSaveReplace(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "msgs.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	base := &mezon.ChannelMessage{MessageID: "1", ChannelID: "c", ClanID: "g", Content: json.RawMessage(`{"t":"v1"}`)}
	if err := db.SaveMessage(base); err != nil {
		t.Fatalf("save v1: %v", err)
	}
	base.Content = json.RawMessage(`{"t":"v2"}`)
	if err := db.SaveMessage(base); err != nil {
		t.Fatalf("save v2: %v", err)
	}
	got, err := db.GetMessageByID("1", "c", "g")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if string(got.Content) != `{"t":"v2"}` {
		t.Fatalf("expected replaced content, got %s", got.Content)
	}
}
