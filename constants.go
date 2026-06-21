// Package mezonlightsdk is a lightweight Go SDK for Mezon chat, ported from the
// TypeScript package mezon-sdk (packages/mezon-sdk in mezonai/mezon-js),
// following the design of the mezon-light-sdk-go reference: methods take a
// context.Context and return errors, optional parameters live in option
// structs, snowflake IDs are kept as decimal strings, and every
// message-content offset is a UTF-16 code unit (JavaScript string index) so
// it lines up with how the Mezon clients count them.
package mezonlightsdk

import "time"

const (
	// MezonGWURL is the default Mezon Gateway URL.
	MezonGWURL = "https://gw.mezon.ai"

	// MezonWSHost is the default WebSocket host, used when authentication
	// does not return a user-specific one.
	MezonWSHost = "gw.mezon.ai"

	// SocketReadyMaxRetry is the maximum number of retries when waiting for
	// the socket to be ready.
	SocketReadyMaxRetry = 20

	// SocketReadyRetryDelay is the initial delay between socket ready
	// retries (uses exponential backoff).
	SocketReadyRetryDelay = 100 * time.Millisecond

	// ClanDM is the clan ID used for Direct Messages.
	ClanDM = "0"

	// ChannelTypeDM is the channel type for Direct Messages.
	ChannelTypeDM = 3

	// ChannelTypeGroup is the channel type for group DMs.
	ChannelTypeGroup = 2

	// StreamModeDM is the stream mode for Direct Messages.
	StreamModeDM = 4

	// StreamModeGroup is the stream mode for group DMs.
	StreamModeGroup = 3

	// DefaultServerKey is the default server key if none is provided.
	DefaultServerKey = "DefaultServerKey"

	// MentionHereUserID is the sentinel user ID the official clients put in a
	// mention's user_id for "@here" (ID_MENTION_HERE in the Mezon web app).
	// Without it the clients render the mention as a role mention (green)
	// instead of a user mention (blue).
	MentionHereUserID = "1775731111020111321"

	// MentionHereTitle is the literal mention text that must appear in the
	// message content at the mention's s/e offsets.
	MentionHereTitle = "@here"
)
