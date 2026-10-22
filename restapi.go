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
	"github.com/quangledang23/mezon-sdk-go/rtapi"
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
	data, err := a.doRaw(bearer, path, req)
	if err != nil {
		return err
	}
	if resp != nil && len(data) > 0 {
		// An abridged API response may carry 0x00 alignment padding that a
		// strict decoder rejects; cut at the end of the well-formed message.
		return proto.Unmarshal(data[:protoMessageLength(data)], resp)
	}
	return nil
}

// doRaw sends a protobuf request to path and returns the undecoded response
// body, preferring the realtime socket and falling back to HTTP POST. It is
// the shared core of doProto, and used directly by endpoints whose body is
// not (always) a proto message (see GenerateMeetToken).
func (a *MezonApi) doRaw(bearer, path string, req proto.Message) ([]byte, error) {
	var body []byte
	if req != nil {
		b, err := proto.Marshal(req)
		if err != nil {
			return nil, err
		}
		body = b
	}
	if data, handled, err := a.doRawSocket(path, body); handled {
		return data, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BasePath+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/proto")
	httpReq.Header.Set("Accept", "application/proto")
	if bearer != "" {
		httpReq.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("mezon api %s: status %d: %s", path, res.StatusCode, string(data))
	}
	return data, nil
}

// AttachSocket routes this client's /mezon.api.Mezon/ requests over the given
// realtime socket whenever it is open (see doRawSocket). MezonClient wires
// this up automatically; it is exported for low-level socket users.
func (a *MezonApi) AttachSocket(s *DefaultSocket) { a.socket = s }

// doRawSocket routes a /mezon.api.Mezon/ request over the realtime socket.
// handled=false means the caller must fall back to HTTP: the socket is unset
// or down, the endpoint has no ApiNameEnum index, or the send failed at the
// transport level. An API-level failure (non-zero response code) is a real
// server answer and is returned as the final error, not retried over HTTP.
func (a *MezonApi) doRawSocket(path string, body []byte) ([]byte, bool, error) {
	s := a.socket
	if s == nil || !s.IsOpen() {
		return nil, false, nil
	}
	name := strings.TrimPrefix(path, "/mezon.api.Mezon/")
	if name == path {
		return nil, false, nil
	}
	if _, ok := apiIndexFromName(name); !ok {
		return nil, false, nil
	}
	respBody, err := s.sendApiRequest(name, body)
	if err != nil {
		if errors.Is(err, ErrSocketClosed) || errors.Is(err, ErrSendTimeout) {
			return nil, false, nil
		}
		return nil, true, err
	}
	return respBody, true, nil
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

// Call invokes an arbitrary /mezon.api.Mezon/ endpoint by API name (see
// apiname.go for the known names), for endpoints without a dedicated wrapper.
// req/resp may be nil for empty request/response bodies. Like every API call
// it rides the realtime socket when open and falls back to HTTP.
func (a *MezonApi) Call(bearer, apiName string, req, resp proto.Message) error {
	return a.doProto(bearer, "/mezon.api.Mezon/"+apiName, req, resp)
}

// SendChannelMessage sends a chat message via the API endpoint, port of
// client.sendChannelMessageApi (mezon-js v2.15.66). Like the other API
// methods it rides the realtime socket when open and falls back to HTTP —
// making it a way to send messages while the socket is down. req.Content
// must already be the JSON-serialized content string (see marshalContent).
func (a *MezonApi) SendChannelMessage(bearer string, req *rtapi.ChannelMessageSend) (*rtapi.ChannelMessageAck, error) {
	resp := &rtapi.ChannelMessageAck{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/SendChannelMessage", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
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

// CreateRole creates a clan role and returns it, port of createRole.
func (a *MezonApi) CreateRole(bearer string, req *api.CreateRoleRequest) (*api.Role, error) {
	resp := &api.Role{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/CreateRole", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteRole deletes a clan role, port of deleteRole.
func (a *MezonApi) DeleteRole(bearer, roleID, clanID string) error {
	req := &api.DeleteRoleRequest{RoleId: atoiID(roleID), ClanId: atoiID(clanID)}
	return a.doProto(bearer, "/mezon.api.Mezon/DeleteRole", req, nil)
}

// UpdateChannelPrivate toggles a channel between public and private, port of
// updateChannelPrivate. RoleIds/UserIds grant access when turning private.
func (a *MezonApi) UpdateChannelPrivate(bearer string, req *api.ChangeChannelPrivateRequest) error {
	return a.doProto(bearer, "/mezon.api.Mezon/UpdateChannelPrivate", req, nil)
}

// AddRolesChannelDesc grants roles access to a private channel, port of
// addRolesChannelDesc.
func (a *MezonApi) AddRolesChannelDesc(bearer string, req *api.AddRoleChannelDescRequest) error {
	return a.doProto(bearer, "/mezon.api.Mezon/AddRolesChannelDesc", req, nil)
}

// SetRoleChannelPermission sets per-channel permission overrides for a role or
// user, port of setRoleChannelPermission.
func (a *MezonApi) SetRoleChannelPermission(bearer string, req *api.UpdateRoleChannelRequest) error {
	return a.doProto(bearer, "/mezon.api.Mezon/SetRoleChannelPermission", req, nil)
}

// GetListPermission lists the permission definitions (id/slug/title) known to
// the server, port of getListPermission.
func (a *MezonApi) GetListPermission(bearer string) (*api.PermissionList, error) {
	resp := &api.PermissionList{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/GetListPermission", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
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

// CreatePoll creates a poll in a channel, port of createPoll (mezon-sdk
// v2.16). Zero ids are sent as 0, matching the TS "0" defaults. Bots may call
// every poll endpoint except VotePoll.
func (a *MezonApi) CreatePoll(bearer string, req *api.CreatePollRequest) (*api.CreatePollResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("mezon: CreatePoll requires a request body")
	}
	resp := &api.CreatePollResponse{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/CreatePoll", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ClosePoll closes a poll (creator only), port of closePoll. The poll is
// located by pollID, or by messageID + channelID when pollID is empty.
func (a *MezonApi) ClosePoll(bearer, pollID, messageID, channelID string) error {
	req := &api.ClosePollRequest{PollId: atoiID(pollID), MessageId: atoiID(messageID), ChannelId: atoiID(channelID)}
	return a.doProto(bearer, "/mezon.api.Mezon/ClosePoll", req, nil)
}

// GetPoll returns a poll with its current results and voter details, port of
// getPoll. The poll is located by pollID, or by messageID + channelID when
// pollID is empty.
func (a *MezonApi) GetPoll(bearer, pollID, messageID, channelID string) (*api.GetPollResponse, error) {
	req := &api.GetPollRequest{PollId: atoiID(pollID), MessageId: atoiID(messageID), ChannelId: atoiID(channelID)}
	resp := &api.GetPollResponse{}
	if err := a.doProto(bearer, "/mezon.api.Mezon/GetPoll", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GenerateMeetToken issues an SFU (voice/meeting) access token for a channel,
// port of generateMeetToken. The server answers either with a
// GenerateMeetTokenResponse proto or, on older deployments, with the bare JWT
// text; both shapes are normalized into the response struct
// (decodeGenerateMeetTokenResponse in api.ts).
func (a *MezonApi) GenerateMeetToken(bearer string, req *api.GenerateMeetTokenRequest) (*api.GenerateMeetTokenResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("mezon: GenerateMeetToken requires a request body")
	}
	data, err := a.doRaw(bearer, "/mezon.api.Mezon/GenerateMeetToken", req)
	if err != nil {
		return nil, err
	}
	return decodeGenerateMeetTokenResponse(data), nil
}

// decodeGenerateMeetTokenResponse treats the body as a proto when it starts
// with the tag of field 1 or 2 (0x0a / 0x12) and decodes cleanly; anything
// else is the raw token text.
func decodeGenerateMeetTokenResponse(data []byte) *api.GenerateMeetTokenResponse {
	body := trimAbridgedPadding(data)
	if len(body) == 0 {
		return &api.GenerateMeetTokenResponse{}
	}
	if body[0] == 0x0a || body[0] == 0x12 {
		resp := &api.GenerateMeetTokenResponse{}
		if proto.Unmarshal(body, resp) == nil {
			return resp
		}
	}
	return &api.GenerateMeetTokenResponse{Token: string(body)}
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
