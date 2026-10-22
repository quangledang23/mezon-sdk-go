package mezon

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

// Raw API frames and TCP ping frames carry the cid as a uint16, so the
// counter must wrap back to 1 instead of running past 65535 (port of
// MezonTransport.generatecid).
func TestGenerateCidWrapsAt65535(t *testing.T) {
	s := newTestSocket()
	s.nextCid = maxCid - 1
	if got := s.generateCid(); got != maxCid-1 {
		t.Fatalf("cid = %d, want %d", got, maxCid-1)
	}
	if got := s.generateCid(); got != maxCid {
		t.Fatalf("cid = %d, want %d", got, maxCid)
	}
	if got := s.generateCid(); got != 1 {
		t.Fatalf("cid after wrap = %d, want 1", got)
	}
}

func TestProtoMessageLength(t *testing.T) {
	msg, _ := proto.Marshal(&rtapi.ChannelMessageAck{ChannelId: 7, MessageId: 9, Code: 1})
	cases := []struct {
		name string
		in   []byte
		want int
	}{
		{"exact", msg, len(msg)},
		{"padded", append(append([]byte{}, msg...), 0, 0, 0), len(msg)},
		{"truncated field kept whole", msg[:len(msg)-1], len(msg) - 1},
		{"empty", nil, 0},
		{"all padding", []byte{0, 0}, 0},
	}
	for _, c := range cases {
		if got := protoMessageLength(c.in); got != c.want {
			t.Errorf("%s: protoMessageLength = %d, want %d", c.name, got, c.want)
		}
	}
	// A padded ack must now decode where a strict parse of the padded bytes
	// would fail.
	padded := append(append([]byte{}, msg...), 0, 0)
	ack := &rtapi.ChannelMessageAck{}
	if err := proto.Unmarshal(padded[:protoMessageLength(padded)], ack); err != nil || ack.MessageId != 9 {
		t.Fatalf("decode padded: err=%v ack=%v", err, ack)
	}
}

// An edit answered by the pushed channel_message (same message id, no cid)
// must resolve the waiting sender and still be delivered as an event.
func TestResolvePendingUpdateMatchesPush(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	s.cids[5] = ch
	s.pendingUpdates[5] = pendingUpdate{messageID: 100, channelID: 20}

	// Different message id: ignored.
	if s.resolvePendingUpdate(&rtapi.Envelope{ChannelMessage: &api.ChannelMessage{MessageId: 101, ChannelId: 20}}) {
		t.Fatal("resolved a push for another message")
	}
	push := &rtapi.Envelope{ChannelMessage: &api.ChannelMessage{MessageId: 100, ChannelId: 20, UpdateTimeSeconds: 42}}
	if !s.resolvePendingUpdate(push) {
		t.Fatal("matching push not resolved")
	}
	select {
	case resp := <-ch:
		if resp.env != push {
			t.Fatal("sender received a different envelope")
		}
	default:
		t.Fatal("sender was not handed the push")
	}
	if len(s.cids) != 0 || len(s.pendingUpdates) != 0 {
		t.Fatal("pending bookkeeping not cleared")
	}
	// Nothing pending: a second identical push is just an event.
	if s.resolvePendingUpdate(push) {
		t.Fatal("resolved with nothing pending")
	}
}

// A channel mismatch is only forgiven when the edit is the sole one in flight
// (the TS single-pending fallback).
func TestResolvePendingUpdateChannelMismatch(t *testing.T) {
	s := newTestSocket()
	s.cids[1] = make(chan *socketResponse, 1)
	s.cids[2] = make(chan *socketResponse, 1)
	s.pendingUpdates[1] = pendingUpdate{messageID: 100, channelID: 20}
	s.pendingUpdates[2] = pendingUpdate{messageID: 200, channelID: 20}
	if s.resolvePendingUpdate(&rtapi.Envelope{ChannelMessageAck: &rtapi.ChannelMessageAck{MessageId: 100, ChannelId: 99}}) {
		t.Fatal("channel mismatch with several edits pending must not resolve")
	}
	delete(s.cids, 2)
	delete(s.pendingUpdates, 2)
	if !s.resolvePendingUpdate(&rtapi.Envelope{ChannelMessageAck: &rtapi.ChannelMessageAck{MessageId: 100, ChannelId: 99}}) {
		t.Fatal("sole pending edit with matching message id must resolve")
	}
}

func TestAwaitingUpdateMeta(t *testing.T) {
	if awaitingUpdateMeta(&rtapi.Envelope{Ping: &rtapi.Ping{}}) != nil {
		t.Fatal("ping is not an edit")
	}
	got := awaitingUpdateMeta(&rtapi.Envelope{ChannelMessageUpdate: &rtapi.ChannelMessageUpdate{MessageId: 1, ChannelId: 2}})
	if got == nil || got.messageID != 1 || got.channelID != 2 {
		t.Fatalf("envelope edit meta = %+v", got)
	}
	body, _ := proto.Marshal(&rtapi.ChannelMessageUpdate{MessageId: 3, ChannelId: 4})
	got = awaitingUpdateMeta(&rtapi.Envelope{ApiRequestEvent: &rtapi.ApiRequestEvent{ApiName: "UpdateChannelMessage", Body: body}})
	if got == nil || got.messageID != 3 || got.channelID != 4 {
		t.Fatalf("api edit meta = %+v", got)
	}
	if awaitingUpdateMeta(&rtapi.Envelope{ApiRequestEvent: &rtapi.ApiRequestEvent{ApiName: "DeleteChannelMessage", Body: body}}) != nil {
		t.Fatal("other API requests are not edits")
	}
}

func TestAckFromUpdateResponseShapes(t *testing.T) {
	// Raw API body: echoed ChannelMessageUpdate, with abridged padding.
	echo, _ := proto.Marshal(&rtapi.ChannelMessageUpdate{ChannelId: 20, MessageId: 100, CreateTimeSeconds: 1700})
	ack, err := ackFromUpdateResponse(&socketResponse{apiResponse: true, body: append(echo, 0, 0, 0)}, 0, 0)
	if err != nil || ack.ChannelId != 20 || ack.MessageId != 100 || ack.CreateTimeSeconds != 1700 || ack.Code != 1 {
		t.Fatalf("raw: err=%v ack=%v", err, ack)
	}
	// Request ids win over the echoed ones when set.
	ack, _ = ackFromUpdateResponse(&socketResponse{apiResponse: true, body: echo}, 21, 101)
	if ack.ChannelId != 21 || ack.MessageId != 101 {
		t.Fatalf("raw ids = %d/%d, want 21/101", ack.ChannelId, ack.MessageId)
	}
	// Envelope with channel_message_ack: passed through.
	direct := &rtapi.ChannelMessageAck{ChannelId: 1, MessageId: 2, Code: 1}
	if ack, _ = ackFromUpdateResponse(&socketResponse{env: &rtapi.Envelope{ChannelMessageAck: direct}}, 0, 0); ack != direct {
		t.Fatal("channel_message_ack not passed through")
	}
	// Pushed channel_message matched by resolvePendingUpdate.
	pushed := &rtapi.Envelope{ChannelMessage: &api.ChannelMessage{ChannelId: 20, MessageId: 100, CreateTimeSeconds: 1700, UpdateTimeSeconds: 1800}}
	ack, _ = ackFromUpdateResponse(&socketResponse{env: pushed}, 0, 0)
	if ack.ChannelId != 20 || ack.MessageId != 100 || ack.UpdateTimeSeconds != 1800 || ack.Code != 1 {
		t.Fatalf("pushed: ack=%v", ack)
	}
	// Anything else: synthesized from the request.
	ack, _ = ackFromUpdateResponse(&socketResponse{env: &rtapi.Envelope{}}, 5, 6)
	if ack.ChannelId != 5 || ack.MessageId != 6 || ack.Code != 1 {
		t.Fatalf("fallback: ack=%v", ack)
	}
}

// A WebSocket-framed push that is a bare api.ChannelMessage (legacy server
// shape) must be wrapped into an Envelope so it dispatches as channel_message.
func TestUnwrapBareChannelMessage(t *testing.T) {
	bare, _ := proto.Marshal(&api.ChannelMessage{MessageId: 100, ChannelId: 20, Content: `{"t":"hi"}`})
	env := &rtapi.Envelope{}
	_ = proto.Unmarshal(bare, env) // lenient parse: no cid, no event field
	if envelopeHasEvent(env) {
		t.Skip("bare message happened to populate an Envelope field; shape not representative")
	}
	got := unwrapBareChannelMessage(env, bare)
	if got.ChannelMessage == nil || got.ChannelMessage.MessageId != 100 || got.ChannelMessage.Content != `{"t":"hi"}` {
		t.Fatalf("not unwrapped: %v", got)
	}
	// A real envelope is left alone.
	real := &rtapi.Envelope{ChannelMessage: &api.ChannelMessage{MessageId: 1, ChannelId: 2}}
	realBytes, _ := proto.Marshal(real)
	if unwrapBareChannelMessage(real, realBytes) != real {
		t.Fatal("real envelope must be returned unchanged")
	}
	// cid-only envelopes (responses) are not rewritten either.
	withCid := &rtapi.Envelope{Cid: 3}
	if unwrapBareChannelMessage(withCid, nil) != withCid {
		t.Fatal("cid envelope must be returned unchanged")
	}
}

func TestDecodeGenerateMeetTokenResponse(t *testing.T) {
	protoBody, _ := proto.Marshal(&api.GenerateMeetTokenResponse{Token: "tok", Url: "wss://sfu"})
	if got := decodeGenerateMeetTokenResponse(append(protoBody, 0, 0)); got.Token != "tok" || got.Url != "wss://sfu" {
		t.Fatalf("proto body: %v", got)
	}
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig"
	if got := decodeGenerateMeetTokenResponse([]byte(jwt)); got.Token != jwt || got.Url != "" {
		t.Fatalf("raw jwt: %v", got)
	}
	if got := decodeGenerateMeetTokenResponse(nil); got.Token != "" {
		t.Fatalf("empty: %v", got)
	}
}

// Disconnecting clears request bookkeeping so a reconnected socket starts
// clean (the TS rejects every executor in markDisconnected).
func TestMarkDisconnectedClearsPending(t *testing.T) {
	s := newTestSocket()
	done := make(chan struct{})
	s.done = done
	s.closeDone = func() { close(done) }
	s.connected.Store(true)
	s.cids[1] = make(chan *socketResponse, 1)
	s.pendingUpdates[1] = pendingUpdate{messageID: 1}
	s.markDisconnected(done, "test")
	if len(s.cids) != 0 || len(s.pendingUpdates) != 0 {
		t.Fatal("pending requests not cleared on disconnect")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("done not closed")
	}
}
