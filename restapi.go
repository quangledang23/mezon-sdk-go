package mezon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/quangledang23/mezon-sdk-go/api"
	"google.golang.org/protobuf/proto"
)

func protoUnmarshal(b []byte, m proto.Message) error { return proto.Unmarshal(b, m) }

// MezonApi is the REST client for the Mezon server, port of src/api.ts. Request
// and response bodies are protobuf (Content-Type/Accept: application/proto)
// posted to the /mezon.api.Mezon/<Method> endpoints, except authentication
// which posts JSON.
type MezonApi struct {
	APIKey   string
	BasePath string
	Timeout  time.Duration
	client   *http.Client

	// socket, when set and open, carries the /mezon.api.Mezon/ requests as
	// ApiRequestEvent frames over the realtime connection instead of HTTP (port
	// of MezonTransport in mezon-js transport.ts). HTTP remains the fallback
	// when the socket is down (port of the client's autoFallbackHttp).
	socket *DefaultSocket
}

// NewMezonApi creates a REST client.
func NewMezonApi(apiKey, basePath string, timeout time.Duration) *MezonApi {
	if timeout <= 0 {
		timeout = 7 * time.Second
	}
	return &MezonApi{
		APIKey:   apiKey,
		BasePath: basePath,
		Timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
	}
}

// doProto sends a protobuf request to path and decodes the protobuf response,
// preferring the realtime socket and falling back to HTTP POST.
func (a *MezonApi) doProto(bearer, path string, req, resp proto.Message) error {
	var body []byte
	if req != nil {
		b, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		body = b
	}
	if handled, err := a.doProtoSocket(path, body, resp); handled {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BasePath+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/proto")
	httpReq.Header.Set("Accept", "application/proto")
	if bearer != "" {
		httpReq.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := a.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		return nil
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("mezon api %s: status %d: %s", path, res.StatusCode, string(data))
	}
	if resp != nil && len(data) > 0 {
		return proto.Unmarshal(data, resp)
	}
	return nil
}

// AttachSocket routes this client's /mezon.api.Mezon/ requests over the given
// realtime socket whenever it is open (see doProtoSocket). MezonClient wires
// this up automatically; it is exported for low-level socket users.
func (a *MezonApi) AttachSocket(s *DefaultSocket) { a.socket = s }

// doProtoSocket routes a /mezon.api.Mezon/ request over the realtime socket.
// handled=false means the caller must fall back to HTTP: the socket is unset
// or down, the endpoint has no ApiNameEnum index, or the send failed at the
// transport level. An API-level failure (non-zero response code) is a real
// server answer and is returned as the final error, not retried over HTTP.
func (a *MezonApi) doProtoSocket(path string, body []byte, resp proto.Message) (bool, error) {
	s := a.socket
	if s == nil || !s.IsOpen() {
		return false, nil
	}
	name := strings.TrimPrefix(path, "/mezon.api.Mezon/")
	if name == path {
		return false, nil
	}
	if _, ok := apiIndexFromName(name); !ok {
		return false, nil
	}
	respBody, err := s.sendApiRequest(name, body)
	if err != nil {
		if errors.Is(err, ErrSocketClosed) || errors.Is(err, ErrSendTimeout) {
			return false, nil
		}
		return true, err
	}
	if resp != nil && len(respBody) > 0 {
		return true, proto.Unmarshal(respBody, resp)
	}
	return true, nil
}

// MezonAuthenticate authenticates an app/bot and returns its session, port of
// mezonAuthenticate. The body is JSON, the response protobuf.
func (a *MezonApi) MezonAuthenticate(botID, apiKey string) (*api.Session, error) {
	bodyObj := map[string]any{
		"account": map[string]any{"appid": botID, "token": apiKey},
	}
	body, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BasePath+"/v2/apps/authenticate/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-protobuf")
	// Basic auth: username is the token, password is empty, matching the TS SDK
	// (sessionManager.authenticate calls mezonAuthenticate(apiKey, "", ...), and
	// api.ts builds "Basic " + base64(basicAuthUsername + ":" + basicAuthPassword)).
	if apiKey != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(apiKey + ":"))
		req.Header.Set("Authorization", "Basic "+cred)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("authenticate: status %d: %s", res.StatusCode, string(data))
	}
	sess := &api.Session{}
	if err := proto.Unmarshal(data, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// CreateChannelDesc creates a channel, port of createChannelDesc.
func (a *MezonApi) CreateChannelDesc(bearer string, req *api.CreateChannelDescRequest) (*api.ChannelDescription, error) {
	resp := &api.ChannelDescription{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/CreateChannelDesc", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteChannelDesc deletes a channel, port of deleteChannelDesc.
func (a *MezonApi) DeleteChannelDesc(bearer, clanID, channelID string) error {
	req := &api.DeleteChannelDescRequest{
		ClanId:    atoiID(clanID),
		ChannelId: atoiID(channelID),
	}
	return a.doProto(bearer, "/mezon.api.Mezon/DeleteChannelDesc", req, nil)
}

// ListClanDescs lists the clans the bot belongs to, port of listClanDescs.
func (a *MezonApi) ListClanDescs(bearer string, limit, state int32, cursor string) (*api.ClanDescList, error) {
	req := &api.ListClanDescRequest{Limit: limit, State: state, Cursor: cursor}
	resp := &api.ClanDescList{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/ListClanDescs", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListChannelDetail returns a channel's detail, port of listChannelDetail.
func (a *MezonApi) ListChannelDetail(bearer, channelID string) (*api.ChannelDescription, error) {
	req := &api.ListChannelDetailRequest{ChannelId: atoiID(channelID)}
	resp := &api.ChannelDescription{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/ListChannelDetail", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListChannelDescs lists channels, port of listChannelDescs.
func (a *MezonApi) ListChannelDescs(bearer string, channelType int32, clanID string, limit, state int32, cursor string, isMobile bool) (*api.ChannelDescList, error) {
	req := &api.ListChannelDescsRequest{
		Limit:       limit,
		State:       state,
		Cursor:      cursor,
		ClanId:      atoiID(clanID),
		ChannelType: channelType,
		IsMobile:    isMobile,
	}
	resp := &api.ChannelDescList{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/ListChannelDescs", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListChannelVoiceUsers lists users in voice channels, port of listChannelVoiceUsers.
func (a *MezonApi) ListChannelVoiceUsers(bearer, clanID string, limit int32) (*api.VoiceChannelUserList, error) {
	req := &api.ListChannelUsersRequest{ClanId: atoiID(clanID), Limit: limit}
	resp := &api.VoiceChannelUserList{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/ListChannelVoiceUsers", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListRoles lists clan roles, port of listRoles.
func (a *MezonApi) ListRoles(bearer, clanID string, limit, state int32, cursor string) (*api.RoleListEventResponse, error) {
	req := &api.RoleListEventRequest{ClanId: atoiID(clanID), Limit: limit, State: state, Cursor: cursor}
	resp := &api.RoleListEventResponse{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/ListRoles", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateRole updates a role, port of updateRole.
func (a *MezonApi) UpdateRole(bearer string, req *api.UpdateRoleRequest) error {
	return a.doProto(bearer, "/mezon.api.Mezon/UpdateRole", req, nil)
}

// AddQuickMenuAccess adds a quick-menu entry, port of addQuickMenuAccess.
func (a *MezonApi) AddQuickMenuAccess(bearer string, req *api.QuickMenuAccess) error {
	return a.doProto(bearer, "/mezon.api.Mezon/AddQuickMenuAccess", req, nil)
}

// ListQuickMenuAccess lists quick-menu entries, port of listQuickMenuAccess.
func (a *MezonApi) ListQuickMenuAccess(bearer, botID, channelID string, menuType int32) (*api.QuickMenuAccessList, error) {
	req := &api.ListQuickMenuAccessRequest{BotId: atoiID(botID), ChannelId: atoiID(channelID), MenuType: menuType}
	resp := &api.QuickMenuAccessList{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/ListQuickMenuAccess", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// PlayMedia posts a play-media request to the streaming server, port of playMedia.
func (a *MezonApi) PlayMedia(bearer string, body map[string]any) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://stn.mezon.ai/api/playmedia", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &out)
	}
	return out, nil
}
