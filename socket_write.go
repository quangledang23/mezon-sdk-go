package mezon

import (
	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

// WriteChatMessage sends a chat message, port of socket.writeChatMessage (with
// the content-length guard from socket_manager.writeChatMessage). Content is
// JSON-serialized and its length validated in UTF-16 code units.
func (s *DefaultSocket) WriteChatMessage(d ReplyMessageData) (*rtapi.ChannelMessageAck, error) {
	content, err := marshalContent(d.Content)
	if err != nil {
		return nil, err
	}
	if err := validateContentLength(content); err != nil {
		return nil, err
	}
	env := &rtapi.Envelope{ChannelMessageSend: &rtapi.ChannelMessageSend{
		ClanId:           atoiID(d.ClanID),
		ChannelId:        atoiID(d.ChannelID),
		Mode:             int32(d.Mode),
		IsPublic:         d.IsPublic,
		Content:          content,
		Mentions:         mentionsToProto(d.Mentions),
		Attachments:      attachmentsToProto(d.Attachments),
		References:       refsToProto(d.References),
		AnonymousMessage: d.AnonymousMessage,
		MentionEveryone:  d.MentionEveryone,
		Avatar:           d.Avatar,
		Code:             int32(d.Code),
		TopicId:          atoiID(d.TopicID),
	}}
	resp, err := s.send(env, 0)
	if err != nil {
		return nil, err
	}
	return resp.ChannelMessageAck, nil
}

// WriteEphemeralMessage sends an ephemeral message visible only to receivers,
// port of socket.writeEphemeralMessage.
func (s *DefaultSocket) WriteEphemeralMessage(d EphemeralMessageData) (*api.ChannelMessage, error) {
	content, err := marshalContent(d.Content)
	if err != nil {
		return nil, err
	}
	if err := validateContentLength(content); err != nil {
		return nil, err
	}
	receivers := make([]int64, 0, len(d.ReceiverIDs))
	for _, r := range d.ReceiverIDs {
		receivers = append(receivers, atoiID(r))
	}
	env := &rtapi.Envelope{EphemeralMessageSend: &rtapi.EphemeralMessageSend{
		ReceiverIds: receivers,
		Message: &rtapi.ChannelMessageSend{
			ClanId:           atoiID(d.ClanID),
			ChannelId:        atoiID(d.ChannelID),
			Mode:             int32(d.Mode),
			IsPublic:         d.IsPublic,
			Content:          content,
			Mentions:         mentionsToProto(d.Mentions),
			Attachments:      attachmentsToProto(d.Attachments),
			References:       refsToProto(d.References),
			AnonymousMessage: d.AnonymousMessage,
			MentionEveryone:  d.MentionEveryone,
			Avatar:           d.Avatar,
			Code:             int32(d.Code),
			TopicId:          atoiID(d.TopicID),
			Id:               atoiID(d.MessageID),
		},
	}}
	resp, err := s.send(env, 0)
	if err != nil {
		return nil, err
	}
	return resp.ChannelMessage, nil
}

// UpdateChatMessage edits a previously sent message, port of socket.updateChatMessage.
func (s *DefaultSocket) UpdateChatMessage(d UpdateMessageData) (*rtapi.ChannelMessageAck, error) {
	content, err := marshalContent(d.Content)
	if err != nil {
		return nil, err
	}
	if err := validateContentLength(content); err != nil {
		return nil, err
	}
	env := &rtapi.Envelope{ChannelMessageUpdate: &rtapi.ChannelMessageUpdate{
		ClanId:            atoiID(d.ClanID),
		ChannelId:         atoiID(d.ChannelID),
		MessageId:         atoiID(d.MessageID),
		Content:           content,
		Mentions:          mentionsToProto(d.Mentions),
		Attachments:       attachmentsToProto(d.Attachments),
		Mode:              int32(d.Mode),
		IsPublic:          d.IsPublic,
		HideEditted:       d.HideEditted,
		TopicId:           atoiID(d.TopicID),
		IsUpdateMsgTopic:  d.IsUpdateMsgTopic,
		CreateTimeSeconds: d.CreateTimeSeconds,
	}}
	resp, err := s.send(env, 0)
	if err != nil {
		return nil, err
	}
	return resp.ChannelMessageAck, nil
}

// WriteMessageReaction adds or removes a reaction, port of socket.writeMessageReaction.
func (s *DefaultSocket) WriteMessageReaction(d ReactMessageData) (*api.MessageReaction, error) {
	env := &rtapi.Envelope{MessageReactionEvent: &api.MessageReaction{
		Id:              atoiID(d.ID),
		ClanId:          atoiID(d.ClanID),
		ChannelId:       atoiID(d.ChannelID),
		Mode:            int32(d.Mode),
		IsPublic:        d.IsPublic,
		MessageId:       atoiID(d.MessageID),
		EmojiId:         atoiID(d.EmojiID),
		Emoji:           d.Emoji,
		Count:           int32(d.Count),
		MessageSenderId: atoiID(d.MessageSenderID),
		Action:          d.ActionDelete,
	}}
	resp, err := s.send(env, 0)
	if err != nil {
		return nil, err
	}
	return resp.MessageReactionEvent, nil
}

// RemoveChatMessage deletes a message, port of socket.removeChatMessage.
func (s *DefaultSocket) RemoveChatMessage(d RemoveMessageData) (*rtapi.ChannelMessageAck, error) {
	env := &rtapi.Envelope{ChannelMessageRemove: &rtapi.ChannelMessageRemove{
		ClanId:    atoiID(d.ClanID),
		ChannelId: atoiID(d.ChannelID),
		Mode:      int32(d.Mode),
		IsPublic:  d.IsPublic,
		MessageId: atoiID(d.MessageID),
		TopicId:   atoiID(d.TopicID),
	}}
	resp, err := s.send(env, 0)
	if err != nil {
		return nil, err
	}
	return resp.ChannelMessageAck, nil
}

// JoinClanChat joins a clan's realtime chat, port of socket.joinClanChat.
func (s *DefaultSocket) JoinClanChat(clanID string) (*rtapi.ClanJoin, error) {
	resp, err := s.send(&rtapi.Envelope{ClanJoin: &rtapi.ClanJoin{ClanId: atoiID(clanID)}}, 0)
	if err != nil {
		return nil, err
	}
	return resp.ClanJoin, nil
}

// JoinChat joins a channel's realtime chat, port of socket.joinChat.
func (s *DefaultSocket) JoinChat(clanID, channelID string, channelType int, isPublic bool) (*rtapi.Channel, error) {
	resp, err := s.send(&rtapi.Envelope{ChannelJoin: &rtapi.ChannelJoin{
		ClanId:      atoiID(clanID),
		ChannelId:   atoiID(channelID),
		ChannelType: int32(channelType),
		IsPublic:    isPublic,
	}}, 0)
	if err != nil {
		return nil, err
	}
	return resp.Channel, nil
}

// LeaveChat leaves a channel's realtime chat, port of socket.leaveChat.
func (s *DefaultSocket) LeaveChat(clanID, channelID string, channelType int, isPublic bool) error {
	_, err := s.send(&rtapi.Envelope{ChannelLeave: &rtapi.ChannelLeave{
		ClanId:      atoiID(clanID),
		ChannelId:   atoiID(channelID),
		ChannelType: int32(channelType),
		IsPublic:    isPublic,
	}}, 0)
	return err
}
