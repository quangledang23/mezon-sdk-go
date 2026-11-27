package mezon

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

// The server identifies a socket API request by its ApiNameEnum index, so the
// canonical indexes from mezon-sdk-parity.spec.ts must hold.
func TestApiNameIndexes(t *testing.T) {
	want := map[string]int32{
		"ListChannelDescs":     0,
		"ListQuickMenuAccess":  12,
		"GenerateMeetToken":    165,
		"SearchCtrlK":          210,
		"GenerateCDNSignature": 212,
		"GetStreamPlayback":    213,
		"LinkCDNFile":          214,
		"SfuBreakoutRoom":      215,
		"RedeemProPlanCoupon":  222,
	}
	for name, idx := range want {
		if got, ok := apiIndexFromName(name); !ok || got != idx {
			t.Errorf("apiIndexFromName(%q) = %d, %v; want %d", name, got, ok, idx)
		}
	}
	if len(apiNames) != 223 {
		t.Errorf("len(apiNames) = %d, want 223", len(apiNames))
	}
}

// serverUnpad mimics the gateway: it strips every trailing zero byte of an
// abridged frame before decoding.
func serverUnpad(t *testing.T, payload []byte) *rtapi.Envelope {
	t.Helper()
	env := &rtapi.Envelope{}
	if err := proto.Unmarshal(bytes.TrimRight(payload, "\x00"), env); err != nil {
		t.Fatalf("server-side decode failed: %v", err)
	}
	return env
}

// Port of abridged-tcp-adapter-cid-last.spec.ts: a request whose body ends in
// a zero byte must survive the gateway's trailing-zero strip.
func TestMarshalEnvelopeCidLast(t *testing.T) {
	// An empty packed user_ids is what ts-proto emits ("42 00"); build the
	// same trailing zero byte with an explicit zero varint field.
	body := []byte{0x08, 0x01, 0x20, 0x00}
	for _, cid := range []int32{1, 127, 128, 65535} {
		env := &rtapi.Envelope{Cid: cid, ApiRequestEvent: &rtapi.ApiRequestEvent{ApiIndex: 66, ApiName: "CreateChannelDesc", Body: body}}

		plain, err := marshalEnvelope(env, false)
		if err != nil {
			t.Fatal(err)
		}
		if plain[len(plain)-1] != 0x00 {
			t.Fatalf("cid %d: test body no longer ends the envelope in 0x00", cid)
		}

		data, err := marshalEnvelope(env, true)
		if err != nil {
			t.Fatal(err)
		}
		if env.Cid != cid {
			t.Fatalf("cid %d: marshalEnvelope left env.Cid = %d", cid, env.Cid)
		}
		if data[len(data)-1] == 0x00 {
			t.Fatalf("cid %d: cid-last envelope ends in 0x00", cid)
		}
		got := serverUnpad(t, encodeAbridgedFrame(data)[1:])
		if got.Cid != cid || !bytes.Equal(got.ApiRequestEvent.GetBody(), body) {
			t.Errorf("cid %d: round trip = cid %d body %x", cid, got.Cid, got.ApiRequestEvent.GetBody())
		}
	}

	// Without a cid the envelope is encoded exactly as before.
	noCid := &rtapi.Envelope{ChannelMessageSend: &rtapi.ChannelMessageSend{ClanId: 1, ChannelId: 2, Content: `{"t":"hi"}`}}
	want, _ := proto.Marshal(noCid)
	if got, _ := marshalEnvelope(noCid, true); !bytes.Equal(got, want) {
		t.Errorf("envelope without cid changed: %x != %x", got, want)
	}
}

func TestTransportsChooseCidLast(t *testing.T) {
	if !(&tcpConn{}).cidLast() {
		t.Error("abridged TCP must marshal the cid last")
	}
	if (&wsConn{}).cidLast() {
		t.Error("WebSocket frames are not padded and keep the plain encoding")
	}
}

// Port of generate-meet-token.spec.ts: an unset current_endpoint_id stays off
// the wire, an explicit 0 is sent as field 4 varint 0.
func TestGenerateMeetTokenEndpointIDPresence(t *testing.T) {
	unset, err := proto.Marshal(&api.GenerateMeetTokenRequest{ChannelId: 1840651530236071936, RoomName: "voice-1", Metadata: "bot;"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(unset, []byte{0x20, 0x00}) {
		t.Errorf("unset current_endpoint_id reached the wire: %x", unset)
	}

	zero, err := proto.Marshal(&api.GenerateMeetTokenRequest{ChannelId: 1840651530236071936, RoomName: "voice-1", Metadata: "bot;", CurrentEndpointId: proto.Int32(0)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(zero, []byte{0x20, 0x00}) {
		t.Errorf("explicit current_endpoint_id 0 not encoded as 20 00: %x", zero)
	}

	four := &api.GenerateMeetTokenRequest{}
	data, _ := proto.Marshal(&api.GenerateMeetTokenRequest{CurrentEndpointId: proto.Int32(4)})
	if err := proto.Unmarshal(data, four); err != nil || four.GetCurrentEndpointId() != 4 {
		t.Errorf("current_endpoint_id 4 round trip = %d, %v", four.GetCurrentEndpointId(), err)
	}
}

// GetStreamPlayback and GenerateCDNSignature post their request proto and
// decode the response (here over the HTTP fallback).
func TestStreamPlaybackAndCDNSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer jwt" {
			t.Errorf("Authorization = %q", got)
		}
		var out []byte
		switch r.URL.Path {
		case "/mezon.api.Mezon/GetStreamPlayback":
			out, _ = proto.Marshal(&api.GetStreamPlaybackResponse{Token: "grant", WhipUrl: "https://stream.test/whip", ExpiresAt: 1793088000})
		case "/mezon.api.Mezon/GenerateCDNSignature":
			out, _ = proto.Marshal(&api.GenerateCDNSignatureResponse{Signature: "1793088000-abcdef"})
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(out)
	}))
	defer srv.Close()
	a := NewMezonApi("key", srv.URL, time.Second)

	playback, err := a.GetStreamPlayback("jwt", &api.GetStreamPlaybackRequest{ChannelId: 12345, Publish: true})
	if err != nil || playback.Token != "grant" || playback.WhipUrl != "https://stream.test/whip" || playback.ExpiresAt != 1793088000 {
		t.Errorf("GetStreamPlayback = %+v, %v", playback, err)
	}
	sig, err := a.GenerateCDNSignature("jwt", &api.GenerateCDNSignatureRequest{ChannelId: 12345})
	if err != nil || sig.Signature != "1793088000-abcdef" {
		t.Errorf("GenerateCDNSignature = %+v, %v", sig, err)
	}
	if _, err := a.GetStreamPlayback("jwt", nil); err == nil {
		t.Error("GetStreamPlayback(nil) must fail")
	}
	if _, err := a.GenerateCDNSignature("jwt", nil); err == nil {
		t.Error("GenerateCDNSignature(nil) must fail")
	}
}

func TestGetHealthyEndpoint(t *testing.T) {
	status := http.StatusOK
	body := `{"api_url":"https://node2.test:443","ws_url":"node2.test","tcp_url":"node2.test:8443","session_id":"sess-9","user_id":1840651530236071940}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/healthy/endpoint" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sess-1" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	// A trailing slash on the base path must not double up.
	a := NewMezonApi("key", srv.URL+"/", time.Second)

	payload, got, err := a.GetHealthyEndpoint("sess-1")
	if err != nil || got != http.StatusOK || payload == nil {
		t.Fatalf("GetHealthyEndpoint = %+v, %d, %v", payload, got, err)
	}
	if payload.APIURL != "https://node2.test:443" || payload.WsURL != "node2.test" || payload.TcpURL != "node2.test:8443" ||
		payload.SessionID != "sess-9" || payload.UserID != "1840651530236071940" {
		t.Errorf("payload = %+v", payload)
	}

	status = http.StatusForbidden
	if payload, got, err := a.GetHealthyEndpoint("sess-1"); payload != nil || got != http.StatusForbidden || err != nil {
		t.Errorf("403 = %+v, %d, %v; want nil payload, 403, nil", payload, got, err)
	}

	status, body = http.StatusOK, "not json"
	if payload, got, err := a.GetHealthyEndpoint("sess-1"); payload != nil || got != http.StatusOK || err != nil {
		t.Errorf("undecodable body = %+v, %d, %v; want nil payload, 200, nil", payload, got, err)
	}

	srv.Close()
	if payload, got, err := a.GetHealthyEndpoint("sess-1"); payload != nil || got != 0 || err == nil {
		t.Errorf("unreachable gateway = %+v, %d, %v; want nil payload, 0, error", payload, got, err)
	}
}

func TestUpdateEndpoint(t *testing.T) {
	s := NewDefaultSocket("ws.test", "api.test", "443", true, nil)
	s.TcpURL = "tcp.test:8443"

	if s.UpdateEndpoint("ws.test", "api.test", "443", true, "tcp.test:8443") {
		t.Error("same endpoint reported as changed")
	}
	if s.UpdateEndpoint("", "", "", false, "") {
		t.Error("empty update must keep the endpoint")
	}
	// The TCP transport dials tcp_url, so a new ws_url alone changes nothing.
	if s.UpdateEndpoint("ws2.test", "api.test", "443", true, "") {
		t.Error("ws_url change must not move the TCP transport")
	}
	if !s.UpdateEndpoint("", "api.test", "443", true, "tcp2.test:9443") {
		t.Error("tcp_url change not reported")
	}
	if host, port := s.tcpAddr(); host != "tcp2.test" || port != "9443" {
		t.Errorf("tcpAddr = %s:%s, want tcp2.test:9443", host, port)
	}

	s.Transport = TransportWebSocket
	if !s.UpdateEndpoint("ws3.test", "api.test", "443", true, "") {
		t.Error("ws_url change not reported for the WebSocket transport")
	}
}

// Port of friend-state-updated.spec.ts (Envelope field 107).
func TestDispatchFriendStateUpdated(t *testing.T) {
	want := &rtapi.FriendStateUpdated{UserId: 1840651530236071940, Username: "bob", DisplayName: "Bob", Avatar: "https://cdn.test/bob.png", State: 2}
	wire, err := proto.Marshal(&rtapi.Envelope{FriendStateUpdated: want})
	if err != nil {
		t.Fatal(err)
	}
	env := &rtapi.Envelope{}
	if err := proto.Unmarshal(wire, env); err != nil {
		t.Fatal(err)
	}

	var got *rtapi.FriendStateUpdated
	s := NewDefaultSocket("", "h", "443", true, func(event string, payload any) {
		if event == EventFriendStateUpdated {
			got, _ = payload.(*rtapi.FriendStateUpdated)
		}
	})
	s.dispatch(env)
	if got == nil || !proto.Equal(got, want) {
		t.Errorf("friend_state_updated dispatched %+v, want %+v", got, want)
	}
}

func TestContentFromWire(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"", ""},
		{`{"t":"hi"}`, `{"t":"hi"}`},
		{`[{"t":"hi"}]`, `[{"t":"hi"}]`},
		{"hello", `{"t":"hello"}`},
		{"  ", `{"t":"  "}`},
		{"[]", `{"t":"[]"}`},
		{`say "hi"`, `{"t":"say \"hi\""}`},
	}
	for _, c := range cases {
		if got := string(contentFromWire(c.raw)); got != c.want {
			t.Errorf("contentFromWire(%q) = %s, want %s", c.raw, got, c.want)
		}
	}
	msg := channelMessageFromProto(&api.ChannelMessage{MessageId: 1, ChannelId: 2, Content: "plain text"})
	if msg.ContentText() != "plain text" {
		t.Errorf("ContentText of plain-text content = %q", msg.ContentText())
	}
}
