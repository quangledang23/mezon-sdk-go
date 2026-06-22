package mezon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// doProto posts a protobuf request to path and decodes the protobuf response.
func (a *MezonApi) doProto(bearer, path string, req, resp proto.Message) error {
	var body []byte
	if req != nil {
		b, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		body = b
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
