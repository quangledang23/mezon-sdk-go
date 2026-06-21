package mezonlightsdk

import (
	"github.com/quangledang23/mezon-sdk-go/proto"
)

// Aliases re-exporting the wire-level types used in the public API, mirroring
// the Api* names of the TypeScript mezon-sdk.
type (
	ApiSession                  = proto.Session
	ApiSessionRefreshRequest    = proto.SessionRefreshRequest
	ApiChannelDescription       = proto.ChannelDescription
	ApiCreateChannelDescRequest = proto.CreateChannelDescRequest
	ApiMessageAttachment        = proto.MessageAttachment
	ApiMessageMention           = proto.MessageMention
	ApiUploadAttachment         = proto.UploadAttachment
	ApiUploadAttachmentRequest  = proto.UploadAttachmentRequest
	ApiUser                     = proto.User
	ApiClanUser                 = proto.ClanUser
	ApiClanUserList             = proto.ClanUserList
	ApiChannelUser              = proto.ChannelUser
	ApiChannelUserList          = proto.ChannelUserList
)

// ClientInitConfig configures a LightClient created from existing tokens.
type ClientInitConfig struct {
	// Token is the authentication token.
	Token string `json:"token"`
	// RefreshToken is used for session renewal.
	RefreshToken string `json:"refresh_token"`
	// APIURL is the API URL for the Mezon server.
	APIURL string `json:"api_url"`
	// WSURL is the WebSocket host for session connectivity.
	WSURL string `json:"ws_url"`
	// UserID is the user ID associated with the session.
	UserID string `json:"user_id"`
	// ServerKey is the server key for authentication (optional, uses
	// DefaultServerKey if empty).
	ServerKey string `json:"serverkey,omitempty"`
}

// AuthenticateConfig configures authentication of a new user.
type AuthenticateConfig struct {
	// IDToken is the ID token from an identity provider.
	IDToken string `json:"id_token"`
	// UserID is the user ID to associate with the account.
	UserID string `json:"user_id"`
	// Username is the username for the account.
	Username string `json:"username"`
	// ServerKey is the server key for authentication (optional).
	ServerKey string `json:"serverkey,omitempty"`
	// GatewayURL is a custom gateway URL (optional, uses MezonGWURL if empty).
	GatewayURL string `json:"gateway_url,omitempty"`
}

// SendMessagePayload describes a message to send.
type SendMessagePayload struct {
	// ChannelID is the channel to send the message to.
	ChannelID string
	// Content is the message content; it is JSON-encoded before sending.
	// Plain strings and {"t": ...} maps get "lk" markup added automatically
	// for any URLs so clients render them as clickable links.
	Content any
	// Mentions holds user/role mention entries pointing into the content
	// text (see ContentBuilder).
	Mentions []*ApiMessageMention
	// Attachments holds optional file/media attachments.
	Attachments []*ApiMessageAttachment
	// MentionEveryone notifies everyone in the channel.
	MentionEveryone bool
	// HideLink, when true, leaves URLs in the content as plain text instead
	// of marking them up as clickable links.
	HideLink bool
	// Code is the optional message code.
	Code int32
}

// AuthenticateBotConfig configures authentication of a bot (app) using the
// bot ID and API key from the Mezon developer portal.
type AuthenticateBotConfig struct {
	// BotID is the application/bot ID.
	BotID string `json:"bot_id"`
	// APIKey is the bot token from the developer portal.
	APIKey string `json:"api_key"`
	// GatewayURL is a custom gateway URL (optional, uses MezonGWURL if empty).
	GatewayURL string `json:"gateway_url,omitempty"`
}

// ApiAppAccount identifies a bot/app in an authentication request.
type ApiAppAccount struct {
	// AppID is the application/bot ID.
	AppID string `json:"appid"`
	// Token is the bot API key.
	Token string `json:"token"`
}

// ApiAuthenticateAppRequest is the request body for bot/app authentication.
type ApiAuthenticateAppRequest struct {
	Account ApiAppAccount `json:"account"`
}

// ApiAuthenticationIdToken is the request body for ID-token authentication.
type ApiAuthenticationIdToken struct {
	// IDToken is the ID token from an identity provider.
	IDToken string `json:"id_token"`
	// UserID is the user ID associated with the token.
	UserID string `json:"user_id"`
	// Username is the username associated with the token.
	Username string `json:"username"`
}

// AuthenticationIdTokenResponse is the response of ID-token authentication.
type AuthenticationIdTokenResponse struct {
	// Token is the authentication token.
	Token string `json:"token"`
	// RefreshToken is used for session renewal.
	RefreshToken string `json:"refresh_token"`
	// APIURL is the API URL for the authenticated user.
	APIURL string `json:"api_url"`
	// WSURL is the WS host for the authenticated user.
	WSURL string `json:"ws_url"`
	// UserID is the user ID of the authenticated user.
	UserID string `json:"user_id"`
}
