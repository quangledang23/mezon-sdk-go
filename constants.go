package mezon

// ChannelType mirrors src/constants/enum.ts ChannelType.
type ChannelType int

const (
	ChannelTypeChannel      ChannelType = 1
	ChannelTypeGroup        ChannelType = 2
	ChannelTypeDM           ChannelType = 3
	ChannelTypeGmeetVoice   ChannelType = 4
	ChannelTypeForum        ChannelType = 5
	ChannelTypeStreaming    ChannelType = 6
	ChannelTypeThread       ChannelType = 7
	ChannelTypeApp          ChannelType = 8
	ChannelTypeAnnouncement ChannelType = 9
	ChannelTypeMezonVoice   ChannelType = 10
)

// ChannelStreamMode mirrors src/constants/enum.ts ChannelStreamMode.
type ChannelStreamMode int

const (
	StreamModeChannel ChannelStreamMode = 2
	StreamModeGroup   ChannelStreamMode = 3
	StreamModeDM      ChannelStreamMode = 4
	StreamModeClan    ChannelStreamMode = 5
	StreamModeThread  ChannelStreamMode = 6
)

// TypeMessage mirrors src/constants/enum.ts TypeMessage (the message "code").
type TypeMessage int

const (
	TypeMessageChat               TypeMessage = 0
	TypeMessageChatUpdate         TypeMessage = 1
	TypeMessageChatRemove         TypeMessage = 2
	TypeMessageTyping             TypeMessage = 3
	TypeMessageIndicator          TypeMessage = 4
	TypeMessageWelcome            TypeMessage = 5
	TypeMessageCreateThread       TypeMessage = 6
	TypeMessageCreatePin          TypeMessage = 7
	TypeMessageMessageBuzz        TypeMessage = 8
	TypeMessageTopic              TypeMessage = 9
	TypeMessageAuditLog           TypeMessage = 10
	TypeMessageSendToken          TypeMessage = 11
	TypeMessageEphemeral          TypeMessage = 12
	TypeMessageUpcomingEvent      TypeMessage = 13
	TypeMessageUpdateEphemeralMsg TypeMessage = 14
	TypeMessageDeleteEphemeralMsg TypeMessage = 15
	TypeMessageContact            TypeMessage = 16
	TypeMessageLocation           TypeMessage = 17
	TypeMessagePoll               TypeMessage = 18
)

// Event names. These are the string keys emitted by the client, matching the
// Events / InternalEventsSocket enums in src/constants/enum.ts. Register a
// handler with MezonClient.On(<Event>, handler).
const (
	EventChannelMessage      = "channel_message"
	EventMessageReaction     = "message_reaction_event"
	EventUserChannelRemoved  = "user_channel_removed_event"
	EventUserClanRemoved     = "user_clan_removed_event"
	EventUserChannelAdded    = "user_channel_added_event"
	EventChannelCreated      = "channel_created_event"
	EventChannelDeleted      = "channel_deleted_event"
	EventChannelUpdated      = "channel_updated_event"
	EventChannelArchive      = "channel_archive_event"
	EventRole                = "role_event"
	EventGiveCoffee          = "give_coffee_event"
	EventRoleAssign          = "role_assign_event"
	EventAddClanUser         = "add_clan_user_event"
	EventTokenSend           = "token_sent_event"
	EventClanEventCreated    = "clan_event_created"
	EventMessageButtonClick  = "message_button_clicked"
	EventStreamingJoined     = "streaming_joined_event"
	EventStreamingLeaved     = "streaming_leaved_event"
	EventDropdownBoxSelected = "dropdown_box_selected"
	EventWebrtcSignalingFwd  = "webrtc_signaling_fwd"
	EventVoiceStarted        = "voice_started_event"
	EventVoiceEnded          = "voice_ended_event"
	EventVoiceJoined         = "voice_joined_event"
	EventVoiceLeaved         = "voice_leaved_event"
	EventNotifications       = "notifications"
	EventQuickMenu           = "quick_menu_event"
	EventAIAgentEnable       = "aiagent_enabled_event"
	EventClanUpdated         = "clan_updated_event"
	EventClanProfileUpdated  = "clan_profile_updated_event"
	EventUserProfileUpdated  = "user_profile_updated_event"
	EventClanDeleted         = "clan_deleted_event"
	EventReady               = "ready"
)
