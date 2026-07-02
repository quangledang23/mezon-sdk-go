package mezon

// MessageStore is an optional persistent store for inbound chat messages, port
// of src/sqlite/MessageDatabase.ts. When configured on ClientConfig, the client
// saves every inbound ChannelMessage and a channel's Messages cache falls back
// to it on a miss, so message lookups survive process restarts and eviction
// from the bounded in-memory cache.
//
// The core module does not ship an implementation to avoid forcing a storage
// dependency on every consumer; a SQLite-backed implementation lives in the
// messagedb submodule (github.com/quangledang23/mezon-sdk-go/messagedb).
// Implementations must be safe for concurrent use. GetMessageByID returns
// (nil, nil) or a non-nil error for a miss; both are treated as "not found".
type MessageStore interface {
	// SaveMessage persists (inserts or replaces) a message keyed by
	// (message id, channel id, clan id).
	SaveMessage(m *ChannelMessage) error
	// GetMessageByID loads a previously saved message, or returns (nil, nil)
	// when absent.
	GetMessageByID(messageID, channelID, clanID string) (*ChannelMessage, error)
}
