package proto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// Reference payloads produced by the original ts-proto generated code of
// mezon-light-sdk (protobufjs), used to verify wire compatibility of this
// hand-written implementation. Each case must decode correctly AND re-encode
// to the identical bytes (both encoders write fields in field-number order).

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	return b
}

func TestWireCompatChannelJoinEnvelope(t *testing.T) {
	data := mustHex(t, "0801220c1080a080b6a8f7ffd8181803")

	env := &Envelope{}
	if err := env.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if env.Cid != 1 || env.ChannelJoin == nil {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	cj := env.ChannelJoin
	if cj.ChannelID != "1779484504377790464" || cj.ChannelType != 3 || cj.ClanID != "" || cj.IsPublic {
		t.Fatalf("unexpected channel_join: %+v", cj)
	}
	if got := env.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatChannelMessageSendEnvelope(t *testing.T) {
	data := mustHex(t, "080742711080a080b6a8f7ffd8181a0d7b2274223a2268656c6c6f227d221210959aef3a1a07736f6d656f6e65380140082a1f0a09696d6167652e706e671080082209696d6167652f706e6728a00630d80438044801521968747470733a2f2f6578616d706c652e636f6d2f612e706e6760016863")

	env := &Envelope{}
	if err := env.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	send := env.ChannelMessageSend
	if send == nil {
		t.Fatalf("expected channel_message_send, got: %+v", env)
	}
	if send.ChannelID != "1779484504377790464" || send.Content != `{"t":"hello"}` || send.Mode != 4 ||
		!send.MentionEveryone || send.Avatar != "https://example.com/a.png" || send.Code != 1 || send.TopicID != "99" {
		t.Fatalf("unexpected channel_message_send: %+v", send)
	}
	if len(send.Mentions) != 1 || send.Mentions[0].UserID != "123456789" || send.Mentions[0].Username != "someone" ||
		send.Mentions[0].S != 1 || send.Mentions[0].E != 8 {
		t.Fatalf("unexpected mentions: %+v", send.Mentions)
	}
	if len(send.Attachments) != 1 || send.Attachments[0].Filename != "image.png" ||
		send.Attachments[0].Filetype != "image/png" || send.Attachments[0].Size != 1024 ||
		send.Attachments[0].Width != 800 || send.Attachments[0].Height != 600 {
		t.Fatalf("unexpected attachments: %+v", send.Attachments)
	}
	if got := env.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatPingEnvelope(t *testing.T) {
	data := mustHex(t, "0802b20100")

	env := &Envelope{}
	if err := env.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if env.Cid != 2 || env.Ping == nil {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if got := env.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatChannelMessageAck(t *testing.T) {
	data := mustHex(t, "087b10c80318012203626f6228c1aeddb20630c2aeddb2063a02080142046c6f676f4a03636174")

	ack := &ChannelMessageAck{}
	if err := ack.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if ack.ChannelID != "123" || ack.MessageID != "456" || ack.Code != 1 || ack.Username != "bob" ||
		ack.CreateTimeSeconds != 1717000001 || ack.UpdateTimeSeconds != 1717000002 ||
		ack.Persistent == nil || !*ack.Persistent || ack.ClanLogo != "logo" || ack.CategoryName != "cat" {
		t.Fatalf("unexpected ack: %+v", ack)
	}
	if got := ack.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatSessionRefreshRequest(t *testing.T) {
	data := mustHex(t, "0a0d726566726573682d746f6b656e12060a01611201311801")

	req := &SessionRefreshRequest{}
	if err := req.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if req.Token != "refresh-token" || !req.IsRemember || req.Vars["a"] != "1" {
		t.Fatalf("unexpected request: %+v", req)
	}
	if got := req.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatCreateChannelDescRequest(t *testing.T) {
	data := mustHex(t, "28033801420a8080d48895a5ac8f172a")

	req := &CreateChannelDescRequest{}
	if err := req.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if req.Type != 3 || req.ChannelPrivate != 1 ||
		len(req.UserIDs) != 2 || req.UserIDs[0] != "1665963703185768448" || req.UserIDs[1] != "42" {
		t.Fatalf("unexpected request: %+v", req)
	}
	if got := req.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatChannelDescription(t *testing.T) {
	data := mustHex(t, "08081880a080b6a8f7ffd818300338094202646d480152026131520261325a020102620e080510c0aeddb206180222026869720201009a010275319a01027532ba01026431ba01026432f00101f80102")

	desc := &ChannelDescription{}
	if err := desc.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if desc.ClanID != "8" || desc.ChannelID != "1779484504377790464" || desc.Type != 3 ||
		desc.CreatorID != "9" || desc.ChannelLabel != "dm" || desc.ChannelPrivate != 1 ||
		desc.E2ee != 1 || desc.MemberCount != 2 {
		t.Fatalf("unexpected description: %+v", desc)
	}
	if len(desc.Avatars) != 2 || desc.Avatars[0] != "a1" ||
		len(desc.UserIDs) != 2 || desc.UserIDs[0] != "1" || desc.UserIDs[1] != "2" ||
		len(desc.Onlines) != 2 || !desc.Onlines[0] || desc.Onlines[1] ||
		len(desc.Usernames) != 2 || desc.Usernames[1] != "u2" ||
		len(desc.DisplayNames) != 2 || desc.DisplayNames[0] != "d1" {
		t.Fatalf("unexpected repeated fields: %+v", desc)
	}
	if desc.LastSentMessage == nil || desc.LastSentMessage.ID != "5" ||
		desc.LastSentMessage.TimestampSeconds != 1717000000 ||
		desc.LastSentMessage.SenderID != "2" || desc.LastSentMessage.Content != "hi" {
		t.Fatalf("unexpected last_sent_message: %+v", desc.LastSentMessage)
	}
	if got := desc.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatUploadAttachmentRequest(t *testing.T) {
	data := mustHex(t, "0a09696d6167652e706e671209696d6167652f706e6718800820a00628d804")

	req := &UploadAttachmentRequest{}
	if err := req.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if req.Filename != "image.png" || req.Filetype != "image/png" || req.Size != 1024 ||
		req.Width != 800 || req.Height != 600 {
		t.Fatalf("unexpected request: %+v", req)
	}
	if got := req.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}

func TestWireCompatMessageAttachmentList(t *testing.T) {
	data := mustHex(t, "0a200a05612e706e6710011a0f68747470733a2f2f782f612e706e672802300340040a230a05622e6d70342209766964656f2f6d70343a0f68747470733a2f2f782f622e6a7067")

	list := &MessageAttachmentList{}
	if err := list.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if len(list.Attachments) != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}
	a, b := list.Attachments[0], list.Attachments[1]
	if a.Filename != "a.png" || a.URL != "https://x/a.png" || a.Size != 1 || a.Width != 2 || a.Height != 3 || a.Duration != 4 {
		t.Fatalf("unexpected first attachment: %+v", a)
	}
	if b.Filename != "b.mp4" || b.Filetype != "video/mp4" || b.Thumbnail != "https://x/b.jpg" {
		t.Fatalf("unexpected second attachment: %+v", b)
	}
	if got := list.Marshal(); !bytes.Equal(got, data) {
		t.Fatalf("re-encode mismatch:\n got %x\nwant %x", got, data)
	}
}
