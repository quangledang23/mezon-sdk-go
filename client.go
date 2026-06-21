package mezonlightsdk

import (
	"context"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

// LightClient provides a simplified interface for Mezon authentication and
// channel management.
//
//	// Initialize from existing tokens:
//	client, err := mezonlightsdk.InitClient(mezonlightsdk.ClientInitConfig{
//		Token:        "your-token",
//		RefreshToken: "your-refresh-token",
//		APIURL:       "https://api.mezon.ai",
//		WSURL:        "gw.mezon.ai",
//		UserID:       "user-123",
//	})
//
//	// Or authenticate with an ID token:
//	client, err := mezonlightsdk.Authenticate(ctx, mezonlightsdk.AuthenticateConfig{
//		IDToken:  "id-token-from-provider",
//		UserID:   "user-123",
//		Username: "johndoe",
//	})
type LightClient struct {
	session *Session
	client  *MezonApi
	userID  string

	// OnRefreshSession, if set, is called after each successful token
	// refresh.
	OnRefreshSession func(session *ApiSession)

	refreshMu   sync.Mutex
	refreshDone chan struct{}
	refreshErr  error
}

// parseBaseURL extracts "scheme://host[:port]" from a URL, mirroring
// parseBaseUrl in the TypeScript SDK.
func parseBaseURL(apiURL string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", err
	}
	scheme := "http"
	if u.Scheme == "https" {
		scheme = "https"
	}
	return scheme + "://" + u.Host, nil
}

// InitClient initializes a LightClient from existing session tokens. Use
// this when you have stored tokens from a previous authentication.
func InitClient(config ClientInitConfig) (*LightClient, error) {
	if config.Token == "" || config.RefreshToken == "" || config.APIURL == "" || config.WSURL == "" || config.UserID == "" {
		return nil, &SessionError{Message: "missing required fields: Token, RefreshToken, APIURL, WSURL, and UserID are all required"}
	}

	session, err := RestoreSession(config.Token, config.RefreshToken, config.APIURL, config.WSURL, true)
	if err != nil {
		return nil, err
	}

	serverKey := config.ServerKey
	if serverKey == "" {
		serverKey = DefaultServerKey
	}
	basePath, err := parseBaseURL(config.APIURL)
	if err != nil {
		return nil, &SessionError{Message: "invalid APIURL: " + err.Error()}
	}

	return &LightClient{
		session: session,
		client:  NewMezonApi(serverKey, 7*time.Second, basePath),
		userID:  config.UserID,
	}, nil
}

// Authenticate authenticates a user with an ID token from an identity
// provider.
func Authenticate(ctx context.Context, config AuthenticateConfig) (*LightClient, error) {
	serverKey := config.ServerKey
	if serverKey == "" {
		serverKey = DefaultServerKey
	}
	gatewayURL := config.GatewayURL
	if gatewayURL == "" {
		gatewayURL = MezonGWURL
	}

	basePath, err := parseBaseURL(gatewayURL)
	if err != nil {
		return nil, &AuthenticationError{Message: "invalid gateway URL: " + err.Error()}
	}
	client := NewMezonApi(serverKey, 7*time.Second, basePath)

	response, err := client.AuthenticateIdToken(ctx, serverKey, "", &ApiAuthenticationIdToken{
		IDToken:  config.IDToken,
		UserID:   config.UserID,
		Username: config.Username,
	})
	if err != nil {
		return nil, err
	}
	if response.Token == "" || response.RefreshToken == "" || response.APIURL == "" || response.WSURL == "" || response.UserID == "" {
		return nil, &AuthenticationError{Message: "invalid authentication response: missing required fields"}
	}

	session, err := RestoreSession(response.Token, response.RefreshToken, response.APIURL, response.WSURL, true)
	if err != nil {
		return nil, err
	}
	apiBase, err := parseBaseURL(response.APIURL)
	if err != nil {
		return nil, &AuthenticationError{Message: "invalid api_url in authentication response: " + err.Error()}
	}
	client.SetBasePath(apiBase)

	return &LightClient{
		session: session,
		client:  client,
		userID:  response.UserID,
	}, nil
}

// AuthenticateBot authenticates a bot (app) with its bot ID and API key from
// the Mezon developer portal, mirroring SessionManager.authenticate in the
// TypeScript mezon-sdk.
func AuthenticateBot(ctx context.Context, config AuthenticateBotConfig) (*LightClient, error) {
	if config.BotID == "" || config.APIKey == "" {
		return nil, &AuthenticationError{Message: "missing required fields: BotID and APIKey are required"}
	}
	gatewayURL := config.GatewayURL
	if gatewayURL == "" {
		gatewayURL = MezonGWURL
	}

	basePath, err := parseBaseURL(gatewayURL)
	if err != nil {
		return nil, &AuthenticationError{Message: "invalid gateway URL: " + err.Error()}
	}
	// The API key doubles as the basic-auth username for session refreshes.
	client := NewMezonApi(config.APIKey, 7*time.Second, basePath)

	apiSession, err := client.AuthenticateApp(ctx, config.APIKey, "", &ApiAuthenticateAppRequest{
		Account: ApiAppAccount{AppID: config.BotID, Token: config.APIKey},
	})
	if err != nil {
		return nil, err
	}
	if apiSession.Token == "" || apiSession.RefreshToken == "" {
		return nil, &AuthenticationError{Message: "invalid authentication response: missing tokens"}
	}

	apiURL := apiSession.APIURL
	if apiURL == "" {
		apiURL = gatewayURL
	}
	wsURL := apiSession.WsURL
	if wsURL == "" {
		wsURL = MezonWSHost
	}

	session, err := RestoreSession(apiSession.Token, apiSession.RefreshToken, apiURL, wsURL, true)
	if err != nil {
		return nil, err
	}
	apiBase, err := parseBaseURL(apiURL)
	if err != nil {
		return nil, &AuthenticationError{Message: "invalid api_url in authentication response: " + err.Error()}
	}
	client.SetBasePath(apiBase)

	userID := apiSession.UserID
	if userID == "" || userID == "0" {
		userID = session.UserID // fall back to the JWT uid claim
	}

	return &LightClient{
		session: session,
		client:  client,
		userID:  userID,
	}, nil
}

// UserID returns the current user ID.
func (c *LightClient) UserID() string { return c.userID }

// Session returns the underlying Mezon session.
func (c *LightClient) Session() *Session { return c.session }

// Client returns the underlying Mezon API client.
func (c *LightClient) Client() *MezonApi { return c.client }

// CreateDM creates a direct message channel with a single user.
func (c *LightClient) CreateDM(ctx context.Context, peerID string) (*ApiChannelDescription, error) {
	return c.client.CreateChannelDesc(ctx, c.session.Token, &ApiCreateChannelDescRequest{
		Type:           ChannelTypeDM,
		ChannelPrivate: 1,
		UserIDs:        []string{peerID},
	})
}

// CreateGroupDM creates a group direct message channel with multiple users.
func (c *LightClient) CreateGroupDM(ctx context.Context, userIDs []string) (*ApiChannelDescription, error) {
	if len(userIDs) == 0 {
		return nil, &SessionError{Message: "at least one user ID is required for a group DM"}
	}
	return c.client.CreateChannelDesc(ctx, c.session.Token, &ApiCreateChannelDescRequest{
		Type:           ChannelTypeGroup,
		ChannelPrivate: 1,
		UserIDs:        userIDs,
	})
}

// ListClanUsers lists all users that are members of a clan.
//
// Note: bot sessions are not permitted to call this (the gateway answers
// HTTP 403); bots learn member names from incoming channel messages instead.
func (c *LightClient) ListClanUsers(ctx context.Context, clanID string) (*ApiClanUserList, error) {
	return c.client.ListClanUsers(ctx, c.session.Token, clanID)
}

// GetClanUser finds a clan member by user ID. It returns nil if the user is
// not a member of the clan.
func (c *LightClient) GetClanUser(ctx context.Context, clanID, userID string) (*ApiClanUser, error) {
	list, err := c.ListClanUsers(ctx, clanID)
	if err != nil {
		return nil, err
	}
	for _, cu := range list.ClanUsers {
		if cu.User != nil && cu.User.ID == userID {
			return cu, nil
		}
	}
	return nil, nil
}

// GetChannelDetail fetches the description of a single channel.
//
// Note: bot sessions are not permitted to call this (HTTP 403).
func (c *LightClient) GetChannelDetail(ctx context.Context, channelID string) (*ApiChannelDescription, error) {
	return c.client.GetChannelDetail(ctx, c.session.Token, channelID)
}

// ListChannelDescs lists the channels visible to the current user/bot.
//
// Note: bot sessions are not permitted to call this (HTTP 403).
func (c *LightClient) ListChannelDescs(ctx context.Context, req *proto.ListChannelDescsRequest) (*proto.ChannelDescList, error) {
	return c.client.ListChannelDescs(ctx, c.session.Token, req)
}

// ListChannelUsers lists all users that are members of a channel.
//
// Note: bot sessions are not permitted to call this (HTTP 403).
func (c *LightClient) ListChannelUsers(ctx context.Context, clanID, channelID string, channelType int32) (*ApiChannelUserList, error) {
	return c.client.ListChannelUsers(ctx, c.session.Token, &proto.ListChannelUsersRequest{
		ClanID:      clanID,
		ChannelID:   channelID,
		ChannelType: channelType,
	})
}

// GetChannelUser finds a channel member by user ID. It returns nil if the
// user is not a member of the channel.
func (c *LightClient) GetChannelUser(ctx context.Context, clanID, channelID string, channelType int32, userID string) (*ApiChannelUser, error) {
	list, err := c.ListChannelUsers(ctx, clanID, channelID, channelType)
	if err != nil {
		return nil, err
	}
	for _, cu := range list.ChannelUsers {
		if cu.UserID == userID {
			return cu, nil
		}
	}
	return nil, nil
}

// UploadAttachment uploads an attachment file to the Mezon server and
// returns the URL of the uploaded file, which can be used in messages.
func (c *LightClient) UploadAttachment(ctx context.Context, request *ApiUploadAttachmentRequest) (*ApiUploadAttachment, error) {
	return c.client.UploadAttachmentFile(ctx, c.session.Token, request)
}

// RefreshSession refreshes the current session using the refresh token.
// Call this before the session expires to maintain connectivity. Concurrent
// callers share a single in-flight refresh.
func (c *LightClient) RefreshSession(ctx context.Context) (*Session, error) {
	if c.session.Created && c.session.ExpiresAt-c.session.CreatedAt < 70 {
		log.Println("Session lifetime too short, please set '--session.token_expiry_sec' option. See the documentation for more info: https://mezon.vn/docs/mezon/getting-started/configuration/#session")
	}
	if c.session.Created && c.session.RefreshExpiresAt-c.session.CreatedAt < 3700 {
		log.Println("Session refresh lifetime too short, please set '--session.refresh_token_expiry_sec' option. See the documentation for more info: https://mezon.vn/docs/mezon/getting-started/configuration/#session")
	}

	c.refreshMu.Lock()
	if c.refreshDone != nil {
		done := c.refreshDone
		c.refreshMu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.refreshMu.Lock()
		err := c.refreshErr
		c.refreshMu.Unlock()
		return c.session, err
	}
	done := make(chan struct{})
	c.refreshDone = done
	c.refreshMu.Unlock()

	serverKey := c.client.ServerKey
	if serverKey == "" {
		serverKey = DefaultServerKey
	}
	apiSession, err := c.client.SessionRefresh(ctx, serverKey, "", &ApiSessionRefreshRequest{
		Token:      c.session.RefreshToken,
		Vars:       c.session.Vars,
		IsRemember: c.session.IsRemember,
	})
	if err == nil {
		err = c.session.Update(apiSession.Token, apiSession.RefreshToken, apiSession.IsRemember)
		if err == nil && c.OnRefreshSession != nil {
			c.OnRefreshSession(apiSession)
		}
	}
	if err != nil {
		log.Printf("Session refresh failed: %v", err)
	}

	c.refreshMu.Lock()
	c.refreshErr = err
	c.refreshDone = nil
	close(done)
	c.refreshMu.Unlock()

	return c.session, err
}

// CreateSocket creates a socket with the client's configuration.
func (c *LightClient) CreateSocket(verbose bool) *DefaultSocket {
	return NewDefaultSocket(c.session.WSURL, "443", true, verbose)
}

// IsSessionExpired reports whether the current session token has expired.
func (c *LightClient) IsSessionExpired() bool {
	return c.session.IsExpired(time.Now())
}

// IsRefreshSessionExpired reports whether the refresh token has expired. If
// it returns true, the user needs to re-authenticate.
func (c *LightClient) IsRefreshSessionExpired() bool {
	return c.session.IsRefreshExpired(time.Now())
}

// Token returns the authentication token for external use.
func (c *LightClient) Token() string { return c.session.Token }

// RefreshToken returns the refresh token for storage.
func (c *LightClient) RefreshToken() string { return c.session.RefreshToken }

// ExportSession exports session data for storage and later restoration via
// InitClient.
func (c *LightClient) ExportSession() ClientInitConfig {
	return ClientInitConfig{
		Token:        c.session.Token,
		RefreshToken: c.session.RefreshToken,
		APIURL:       c.session.APIURL,
		WSURL:        c.session.WSURL,
		UserID:       c.userID,
	}
}
