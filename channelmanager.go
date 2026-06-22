package mezon

import "github.com/quangledang23/mezon-sdk-go/api"

// ChannelManager handles DM-channel discovery and creation, port of
// src/mezon-client/manager/channel_manager.ts.
type ChannelManager struct {
	apiClient *MezonApi
	socket    *DefaultSocket
	client    *MezonClient
	dmByUser  map[string]string // user_id -> dm channel id
}

func newChannelManager(apiClient *MezonApi, socket *DefaultSocket, client *MezonClient) *ChannelManager {
	return &ChannelManager{apiClient: apiClient, socket: socket, client: client, dmByUser: map[string]string{}}
}

// InitAllDMChannels loads existing DM channels, port of initAllDmChannels.
func (m *ChannelManager) InitAllDMChannels(sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	channels, err := m.apiClient.ListChannelDescs(sessionToken, int32(ChannelTypeDM), "", 0, 0, "", false)
	if err != nil {
		return err
	}
	for _, ch := range channels.GetChanneldesc() {
		if ch.Type != int32(ChannelTypeDM) || len(ch.UserIds) == 0 {
			continue
		}
		m.dmByUser[itoaID(ch.UserIds[0])] = itoaID(ch.ChannelId)
	}
	return nil
}

// GetAllDMChannels returns the discovered DM channel map (user id -> channel id).
func (m *ChannelManager) GetAllDMChannels() map[string]string { return m.dmByUser }

// CreateDMChannel creates (or returns) a DM channel with the user, port of
// createDMchannel. It joins the channel realtime chat on success.
func (m *ChannelManager) CreateDMChannel(userID string) (*api.ChannelDescription, error) {
	if !IsValidUserID(userID) {
		return nil, ErrNotFound
	}
	session := m.client.session
	if session == nil {
		return nil, ErrNotFound
	}
	req := &api.CreateChannelDescRequest{
		ClanId:         0,
		ChannelId:      0,
		CategoryId:     0,
		Type:           int32(ChannelTypeDM),
		UserIds:        []int64{atoiID(userID)},
		ChannelPrivate: 1,
	}
	ch, err := m.apiClient.CreateChannelDesc(session.Token, req)
	if err != nil || ch == nil {
		return nil, err
	}
	if _, err := m.socket.JoinChat(itoaID(ch.ClanId), itoaID(ch.ChannelId), int(ch.Type), false); err != nil {
		return ch, nil
	}
	return ch, nil
}
