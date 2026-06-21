package mezonlightsdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/quangledang23/mezon-sdk-go/proto"
)

// MezonApi is a low-level HTTP client for the Mezon gateway, the Go
// counterpart of MezonApi in api.gen.ts. Request/response bodies use the
// protobuf wire format except for ID-token authentication, which uses JSON.
type MezonApi struct {
	// ServerKey authenticates server-to-server calls (basic auth username).
	ServerKey string
	// Timeout bounds each request.
	Timeout time.Duration
	// HTTPClient performs the requests; defaults to a plain http.Client.
	HTTPClient *http.Client

	mu       sync.RWMutex
	basePath string
}

// NewMezonApi creates a client. If timeout is zero, 7 seconds is used,
// matching the TypeScript SDK.
func NewMezonApi(serverKey string, timeout time.Duration, basePath string) *MezonApi {
	if timeout <= 0 {
		timeout = 7 * time.Second
	}
	return &MezonApi{
		ServerKey:  serverKey,
		Timeout:    timeout,
		HTTPClient: &http.Client{},
		basePath:   basePath,
	}
}

// BasePath returns the current base URL.
func (a *MezonApi) BasePath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.basePath
}

// SetBasePath replaces the base URL, e.g. after authentication returns a
// user-specific endpoint.
func (a *MezonApi) SetBasePath(basePath string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.basePath = basePath
}

// AuthenticateIdToken authenticates a user with an ID token from an identity
// provider.
func (a *MezonApi) AuthenticateIdToken(ctx context.Context, basicAuthUsername, basicAuthPassword string, body *ApiAuthenticationIdToken) (*AuthenticationIdTokenResponse, error) {
	if body == nil {
		return nil, errors.New("'body' is a required parameter but is nil")
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	data, err := a.post(ctx, "/v2/account/authenticate/idtoken", payload, basicAuthHeader(basicAuthUsername, basicAuthPassword))
	if err != nil {
		return nil, err
	}

	out := &AuthenticationIdTokenResponse{}
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

// AuthenticateApp authenticates a bot (app) with its API key, mirroring
// mezonAuthenticate in the TypeScript mezon-sdk. The request body is JSON and
// the response is a protobuf-encoded Session.
func (a *MezonApi) AuthenticateApp(ctx context.Context, basicAuthUsername, basicAuthPassword string, body *ApiAuthenticateAppRequest) (*ApiSession, error) {
	if body == nil {
		return nil, errors.New("'body' is a required parameter but is nil")
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	data, err := a.post(ctx, "/v2/apps/authenticate/token", payload, basicAuthHeader(basicAuthUsername, basicAuthPassword))
	if err != nil {
		return nil, err
	}

	out := &proto.Session{}
	// The gateway answers this endpoint with JSON; fall back to the protobuf
	// wire format for servers that honor the Accept header.
	if len(data) > 0 && data[0] == '{' {
		if err := json.Unmarshal(data, out); err != nil {
			return nil, err
		}
		return out, nil
	}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// SessionRefresh refreshes a user's session using a refresh token retrieved
// from a previous authentication request.
func (a *MezonApi) SessionRefresh(ctx context.Context, basicAuthUsername, basicAuthPassword string, body *ApiSessionRefreshRequest) (*ApiSession, error) {
	if body == nil {
		return nil, errors.New("'body' is a required parameter but is nil")
	}

	data, err := a.post(ctx, "/mezon.api.Mezon/SessionRefresh", body.Marshal(), basicAuthHeader(basicAuthUsername, basicAuthPassword))
	if err != nil {
		return nil, err
	}

	out := &proto.Session{}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateChannelDesc creates a new channel with the current user as the owner.
func (a *MezonApi) CreateChannelDesc(ctx context.Context, bearerToken string, body *ApiCreateChannelDescRequest) (*ApiChannelDescription, error) {
	if body == nil {
		return nil, errors.New("'body' is a required parameter but is nil")
	}

	data, err := a.post(ctx, "/mezon.api.Mezon/CreateChannelDesc", body.Marshal(), bearerAuthHeader(bearerToken))
	if err != nil {
		return nil, err
	}

	out := &proto.ChannelDescription{}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// ListClanUsers lists all users that are members of a clan.
func (a *MezonApi) ListClanUsers(ctx context.Context, bearerToken, clanID string) (*ApiClanUserList, error) {
	if clanID == "" {
		return nil, errors.New("'clanID' is a required parameter but is empty")
	}

	req := &proto.ListClanUsersRequest{ClanID: clanID}
	data, err := a.post(ctx, "/mezon.api.Mezon/ListClanUsers", req.Marshal(), bearerAuthHeader(bearerToken))
	if err != nil {
		return nil, err
	}

	out := &proto.ClanUserList{}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// ListChannelDescs lists the channels visible to the current user/bot.
func (a *MezonApi) ListChannelDescs(ctx context.Context, bearerToken string, req *proto.ListChannelDescsRequest) (*proto.ChannelDescList, error) {
	if req == nil {
		req = &proto.ListChannelDescsRequest{}
	}

	data, err := a.post(ctx, "/mezon.api.Mezon/ListChannelDescs", req.Marshal(), bearerAuthHeader(bearerToken))
	if err != nil {
		return nil, err
	}

	out := &proto.ChannelDescList{}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// GetChannelDetail fetches the description of a single channel.
func (a *MezonApi) GetChannelDetail(ctx context.Context, bearerToken, channelID string) (*ApiChannelDescription, error) {
	if channelID == "" {
		return nil, errors.New("'channelID' is a required parameter but is empty")
	}

	req := &proto.ListChannelDetailRequest{ChannelID: channelID}
	data, err := a.post(ctx, "/mezon.api.Mezon/ListChannelDetail", req.Marshal(), bearerAuthHeader(bearerToken))
	if err != nil {
		return nil, err
	}

	out := &proto.ChannelDescription{}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// ListChannelUsers lists all users that are members of a channel.
func (a *MezonApi) ListChannelUsers(ctx context.Context, bearerToken string, req *proto.ListChannelUsersRequest) (*ApiChannelUserList, error) {
	if req == nil {
		return nil, errors.New("'req' is a required parameter but is nil")
	}

	data, err := a.post(ctx, "/mezon.api.Mezon/ListChannelUsers", req.Marshal(), bearerAuthHeader(bearerToken))
	if err != nil {
		return nil, err
	}

	out := &proto.ChannelUserList{}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// UploadAttachmentFile registers an attachment upload and returns the file
// URL that can be used in messages.
func (a *MezonApi) UploadAttachmentFile(ctx context.Context, bearerToken string, body *ApiUploadAttachmentRequest) (*ApiUploadAttachment, error) {
	if body == nil {
		return nil, errors.New("'body' is a required parameter but is nil")
	}

	data, err := a.post(ctx, "/mezon.api.Mezon/UploadAttachmentFile", body.Marshal(), bearerAuthHeader(bearerToken))
	if err != nil {
		return nil, err
	}

	out := &proto.UploadAttachment{}
	if err := out.Unmarshal(data); err != nil {
		return nil, err
	}
	return out, nil
}

// post performs a POST request and returns the raw response body. A 204
// response yields a nil body; non-2xx responses yield an *APIError.
func (a *MezonApi) post(ctx context.Context, urlPath string, body []byte, authHeader string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BasePath()+urlPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/proto")
	req.Header.Set("Content-Type", "application/proto")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return data, nil
}

func basicAuthHeader(username, password string) string {
	if username == "" {
		return ""
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func bearerAuthHeader(token string) string {
	if token == "" {
		return ""
	}
	return "Bearer " + token
}
