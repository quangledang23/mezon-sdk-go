package mezon

import "github.com/quangledang23/mezon-sdk-go/rtapi"

// dispatch routes an inbound (cid-less) envelope to the registered event
// handlers, mirroring the InternalEventsSocket fan-out in src/socket.ts. The
// channel_message event carries a friendly *ChannelMessage; all other events
// carry their decoded protobuf message pointer (from the rtapi/api packages).
func (s *DefaultSocket) dispatch(env *rtapi.Envelope) {
	if s.emit == nil {
		return
	}
	switch {
	case env.ChannelMessage != nil:
		s.emit(EventChannelMessage, channelMessageFromProto(env.ChannelMessage))
	case env.MessageReactionEvent != nil:
		s.emit(EventMessageReaction, env.MessageReactionEvent)
	case env.ChannelCreatedEvent != nil:
		s.emit(EventChannelCreated, env.ChannelCreatedEvent)
	case env.ChannelDeletedEvent != nil:
		s.emit(EventChannelDeleted, env.ChannelDeletedEvent)
	case env.ChannelUpdatedEvent != nil:
		s.emit(EventChannelUpdated, env.ChannelUpdatedEvent)
	case env.UserChannelAddedEvent != nil:
		s.emit(EventUserChannelAdded, env.UserChannelAddedEvent)
	case env.UserChannelRemovedEvent != nil:
		s.emit(EventUserChannelRemoved, env.UserChannelRemovedEvent)
	case env.UserClanRemovedEvent != nil:
		s.emit(EventUserClanRemoved, env.UserClanRemovedEvent)
	case env.AddClanUserEvent != nil:
		s.emit(EventAddClanUser, env.AddClanUserEvent)
	case env.RoleEvent != nil:
		s.emit(EventRole, env.RoleEvent)
	case env.RoleAssignEvent != nil:
		s.emit(EventRoleAssign, env.RoleAssignEvent)
	case env.GiveCoffeeEvent != nil:
		s.emit(EventGiveCoffee, env.GiveCoffeeEvent)
	case env.TokenSentEvent != nil:
		s.emit(EventTokenSend, env.TokenSentEvent)
	case env.ClanEventCreated != nil:
		s.emit(EventClanEventCreated, env.ClanEventCreated)
	case env.MessageButtonClicked != nil:
		s.emit(EventMessageButtonClick, env.MessageButtonClicked)
	case env.StreamingJoinedEvent != nil:
		s.emit(EventStreamingJoined, env.StreamingJoinedEvent)
	case env.StreamingLeavedEvent != nil:
		s.emit(EventStreamingLeaved, env.StreamingLeavedEvent)
	case env.DropdownBoxSelected != nil:
		s.emit(EventDropdownBoxSelected, env.DropdownBoxSelected)
	case env.WebrtcSignalingFwd != nil:
		s.emit(EventWebrtcSignalingFwd, env.WebrtcSignalingFwd)
	case env.VoiceStartedEvent != nil:
		s.emit(EventVoiceStarted, env.VoiceStartedEvent)
	case env.VoiceEndedEvent != nil:
		s.emit(EventVoiceEnded, env.VoiceEndedEvent)
	case env.VoiceJoinedEvent != nil:
		s.emit(EventVoiceJoined, env.VoiceJoinedEvent)
	case env.VoiceLeavedEvent != nil:
		s.emit(EventVoiceLeaved, env.VoiceLeavedEvent)
	case env.Notifications != nil:
		s.emit(EventNotifications, env.Notifications)
	case env.QuickMenuEvent != nil:
		s.emit(EventQuickMenu, env.QuickMenuEvent)
	case env.AiagentEnabledEvent != nil:
		s.emit(EventAIAgentEnable, env.AiagentEnabledEvent)
	case env.ClanUpdatedEvent != nil:
		s.emit(EventClanUpdated, env.ClanUpdatedEvent)
	case env.ClanProfileUpdatedEvent != nil:
		s.emit(EventClanProfileUpdated, env.ClanProfileUpdatedEvent)
	case env.UserProfileUpdatedEvent != nil:
		s.emit(EventUserProfileUpdated, env.UserProfileUpdatedEvent)
	case env.ClanDeletedEvent != nil:
		s.emit(EventClanDeleted, env.ClanDeletedEvent)
	}
}
