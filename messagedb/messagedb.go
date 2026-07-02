// Package messagedb provides a SQLite-backed mezon.MessageStore, a Go port of
// the TypeScript SDK's src/sqlite/MessageDatabase.ts. It persists inbound chat
// messages so lookups survive process restarts and eviction from the bounded
// in-memory cache.
//
// It uses the pure-Go SQLite driver modernc.org/sqlite (no cgo). Wire it into a
// client with:
//
//	db, err := messagedb.New("") // default ./mezon-cache/mezon-messages-cache.db
//	if err != nil { ... }
//	defer db.Close()
//	client, _ := mezon.NewMezonClient(mezon.ClientConfig{
//		BotID: id, Token: tok, MessageStore: db,
//	})
package messagedb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	mezon "github.com/quangledang23/mezon-sdk-go"
	_ "modernc.org/sqlite"
)

// DefaultPath is the default database file, matching the TS SDK.
const DefaultPath = "./mezon-cache/mezon-messages-cache.db"

// MessageDatabase is a SQLite-backed mezon.MessageStore.
type MessageDatabase struct {
	db *sql.DB
	mu sync.Mutex // serializes writes; modernc sqlite handles concurrency but a single connection avoids "database is locked"
}

// compile-time assertion that MessageDatabase satisfies the core interface.
var _ mezon.MessageStore = (*MessageDatabase)(nil)

// New opens (creating if needed) the message database at dbPath, creating the
// parent directory and schema. An empty dbPath uses DefaultPath.
func New(dbPath string) (*MessageDatabase, error) {
	if dbPath == "" {
		dbPath = DefaultPath
	}
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("messagedb: create dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("messagedb: open: %w", err)
	}
	// A single open connection keeps SQLite writes serialized and avoids
	// "database is locked" under the client's concurrent message handling.
	db.SetMaxOpenConns(1)
	m := &MessageDatabase{db: db}
	if err := m.init(); err != nil {
		db.Close()
		return nil, err
	}
	return m, nil
}

func (m *MessageDatabase) init() error {
	if _, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS messages_v2 (
		id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		clan_id TEXT NOT NULL,
		sender_id TEXT,
		content TEXT,
		mentions TEXT,
		attachments TEXT,
		reactions TEXT,
		msg_references TEXT,
		topic_id TEXT,
		create_time_seconds INTEGER,
		PRIMARY KEY (id, channel_id, clan_id)
	)`); err != nil {
		return fmt.Errorf("messagedb: create table: %w", err)
	}
	if _, err := m.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_messages_v2_channel ON messages_v2(channel_id)`,
	); err != nil {
		return fmt.Errorf("messagedb: create index: %w", err)
	}
	return nil
}

// marshalJSON serializes v to a JSON string, returning fallback on error so a
// bad field never fails the whole save (mirroring the TS which never throws
// here).
func marshalJSON(v any, fallback string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(b)
}

// SaveMessage inserts or replaces a message, port of MessageDatabase.saveMessage.
func (m *MessageDatabase) SaveMessage(msg *mezon.ChannelMessage) error {
	if msg == nil {
		return nil
	}
	id := msg.MessageID
	if id == "" {
		id = msg.ID
	}
	content := "{}"
	if len(msg.Content) > 0 {
		content = string(msg.Content)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(
		`INSERT OR REPLACE INTO messages_v2 (
			id, channel_id, clan_id, sender_id,
			content, mentions, attachments, reactions,
			msg_references, topic_id, create_time_seconds
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		msg.ChannelID,
		msg.ClanID,
		msg.SenderID,
		content,
		marshalJSON(msg.Mentions, "[]"),
		marshalJSON(msg.Attachments, "[]"),
		marshalJSON(msg.Reactions, "[]"),
		marshalJSON(msg.References, "[]"),
		msg.TopicID,
		msg.CreateTimeSeconds,
	)
	if err != nil {
		return fmt.Errorf("messagedb: save %s: %w", id, err)
	}
	return nil
}

// GetMessageByID loads a saved message, port of MessageDatabase.getMessageById.
// It returns (nil, nil) when the message is absent.
func (m *MessageDatabase) GetMessageByID(messageID, channelID, clanID string) (*mezon.ChannelMessage, error) {
	row := m.db.QueryRow(
		`SELECT id, channel_id, clan_id, sender_id, content,
			mentions, attachments, reactions, msg_references,
			topic_id, create_time_seconds
		 FROM messages_v2
		 WHERE id = ? AND channel_id = ? AND clan_id = ?
		 LIMIT 1`,
		messageID, channelID, clanID,
	)
	var (
		id, chID, clID, senderID, topicID string
		content, mentions, attachments    string
		reactions, references             string
		createTimeSeconds                 sql.NullInt64
	)
	if err := row.Scan(
		&id, &chID, &clID, &senderID, &content,
		&mentions, &attachments, &reactions, &references,
		&topicID, &createTimeSeconds,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("messagedb: get %s: %w", messageID, err)
	}

	cm := &mezon.ChannelMessage{
		ID:                id,
		MessageID:         id,
		ChannelID:         chID,
		ClanID:            clID,
		SenderID:          senderID,
		TopicID:           topicID,
		CreateTimeSeconds: uint32(createTimeSeconds.Int64),
	}
	if content != "" {
		cm.Content = json.RawMessage(content)
	}
	_ = json.Unmarshal([]byte(mentions), &cm.Mentions)
	_ = json.Unmarshal([]byte(attachments), &cm.Attachments)
	_ = json.Unmarshal([]byte(reactions), &cm.Reactions)
	_ = json.Unmarshal([]byte(references), &cm.References)
	return cm, nil
}

// Close closes the underlying database.
func (m *MessageDatabase) Close() error { return m.db.Close() }
