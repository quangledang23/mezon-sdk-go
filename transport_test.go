package mezon

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/quangledang23/mezon-sdk-go/api"
	"github.com/quangledang23/mezon-sdk-go/rtapi"
)

func TestEncodeAbridgedFrameShort(t *testing.T) {
	frame := encodeAbridgedFrame([]byte{1, 2, 3, 4, 5})
	want := []byte{2, 1, 2, 3, 4, 5, 0, 0, 0} // len/4 header, payload, 3 pad bytes
	if !bytes.Equal(frame, want) {
		t.Fatalf("frame = %v, want %v", frame, want)
	}
}

func TestEncodeAbridgedFrameExtended(t *testing.T) {
	payload := bytes.Repeat([]byte{0xAB}, 127*4) // first size needing the 0x7F header
	frame := encodeAbridgedFrame(payload)
	if frame[0] != abridgedExtendedPrefix {
		t.Fatalf("header prefix = 0x%02x, want 0x7f", frame[0])
	}
	if got := int(frame[1]) | int(frame[2])<<8 | int(frame[3])<<16; got != 127 {
		t.Fatalf("length/4 = %d, want 127", got)
	}
	if !bytes.Equal(frame[4:], payload) {
		t.Fatal("payload mismatch")
	}
}

func TestTCPHandshakeFrame(t *testing.T) {
	frame := tcpHandshakeFrame("abc")
	want := []byte{tcpHandshakePrefix, 1, 'a', 'b', 'c', 0}
	if !bytes.Equal(frame, want) {
		t.Fatalf("handshake = %v, want %v", frame, want)
	}
}

// readerConn feeds readFrame from a fixed buffer without a real connection.
func readerConn(data []byte) *tcpConn {
	return &tcpConn{r: bufio.NewReader(bytes.NewReader(data))}
}

func TestAbridgedEnvelopeRoundTrip(t *testing.T) {
	// Sizes straddling the short/extended header boundary and the padding
	// cases; payloads end non-zero since abridged padding trim is lossy for
	// trailing zero bytes by design (TrimPadding in the reference codecs).
	for _, n := range []int{1, 3, 4, 5, 503, 504, 508, 1<<20 + 1} {
		payload := bytes.Repeat([]byte{0x5A}, n)
		f, err := readerConn(encodeAbridgedFrame(payload)).readFrame()
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if f.kind != frameEnvelope || !f.padded {
			t.Fatalf("n=%d: frame = %+v, want padded envelope", n, f)
		}
		if !bytes.Equal(trimAbridgedPadding(f.payload), payload) {
			t.Fatalf("n=%d: payload mismatch (got %d bytes)", n, len(f.payload))
		}
	}
}

// An envelope whose proto encoding ends with 0x00 (empty trailing submessage)
// must survive both directions: outbound the writer appends an unknown-field
// guard so the gateway's padding trim cannot eat the zero, inbound the
// decoder distinguishes padding from content by trial parsing.
func TestEnvelopeTrailingZeroBothDirections(t *testing.T) {
	env := &rtapi.Envelope{Cid: 1, ClanJoin: &rtapi.ClanJoin{}}
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if data[len(data)-1] != 0x00 {
		t.Fatalf("fixture no longer ends with 0x00: % x", data)
	}

	// Outbound: writeEnvelope must emit a payload that still decodes to the
	// same message after the server-side trim (trimAbridgedPadding).
	client, server := net.Pipe()
	go func() {
		c := &tcpConn{conn: client}
		_ = c.writeEnvelope(data)
		_ = client.Close()
	}()
	r := bufio.NewReader(server)
	prefix, err := r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	payloadLen, err := readAbridgedLength(r, prefix)
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatal(err)
	}
	got := &rtapi.Envelope{}
	if err := proto.Unmarshal(trimAbridgedPadding(payload), got); err != nil {
		t.Fatalf("server-side decode after trim: %v", err)
	}
	if got.Cid != 1 || got.ClanJoin == nil {
		t.Fatalf("server-side decode = %+v", got)
	}

	// Inbound: a padded frame whose proto content ends with 0x00.
	f, err := readerConn(encodeAbridgedFrame(data)).readFrame()
	if err != nil {
		t.Fatal(err)
	}
	dec, err := unmarshalEnvelope(f)
	if err != nil {
		t.Fatalf("unmarshalEnvelope: %v", err)
	}
	if dec.Cid != 1 || dec.ClanJoin == nil {
		t.Fatalf("decoded = %+v", dec)
	}
}

func TestReadFramePong(t *testing.T) {
	f, err := readerConn([]byte{tcpPongPrefix, 0x12, 0x34}).readFrame()
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != framePong || f.cid != 0x1234 {
		t.Fatalf("frame = %+v, want pong cid 0x1234", f)
	}
}

// tcpAPIFrame builds a server->client API response chunk: 0xFF, cid uint16 BE,
// code word uint32 BE (high 16 response code, low 16 fin flag), payload length
// uint32 BE, payload.
func tcpAPIFrame(cid uint16, respCode uint16, fin bool, payload []byte) []byte {
	frame := make([]byte, 11, 11+len(payload))
	frame[0] = rawFramePrefix
	binary.BigEndian.PutUint16(frame[1:3], cid)
	codeWord := uint32(respCode) << 16
	if fin {
		codeWord |= rawCodeFin
	}
	binary.BigEndian.PutUint32(frame[3:7], codeWord)
	binary.BigEndian.PutUint32(frame[7:11], uint32(len(payload)))
	return append(frame, payload...)
}

func TestReadFrameAPIChunks(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(tcpAPIFrame(9, 0, false, []byte("part1")))
	buf.Write(tcpAPIFrame(9, 7, true, []byte("part2")))
	c := readerConn(buf.Bytes())

	f1, err := c.readFrame()
	if err != nil {
		t.Fatal(err)
	}
	if f1.kind != frameAPIChunk || f1.fin || f1.cid != 9 || !bytes.Equal(f1.payload, []byte("part1")) {
		t.Fatalf("chunk 1 = %+v", f1)
	}
	f2, err := c.readFrame()
	if err != nil {
		t.Fatal(err)
	}
	if !f2.fin || f2.code != 7 || !bytes.Equal(f2.payload, []byte("part2")) {
		t.Fatalf("chunk 2 = %+v", f2)
	}
}

func TestReadFrameSplitAcrossWrites(t *testing.T) {
	// A TCP read may deliver a fraction of a frame; readFrame must reassemble.
	client, server := net.Pipe()
	defer client.Close()
	frame := encodeAbridgedFrame(bytes.Repeat([]byte{0x5A}, 600))
	go func() {
		defer server.Close()
		for i := 0; i < len(frame); i += 7 {
			end := i + 7
			if end > len(frame) {
				end = len(frame)
			}
			if _, err := server.Write(frame[i:end]); err != nil {
				return
			}
		}
	}()

	c := &tcpConn{conn: client, r: bufio.NewReader(client)}
	f, err := c.readFrame()
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameEnvelope || len(f.payload) != 600 {
		t.Fatalf("frame = kind %d, %d bytes; want envelope of 600", f.kind, len(f.payload))
	}
}

// wsServerFrame builds an unmasked RFC 6455 frame as the production gateway
// sends for server-pushed envelopes.
func wsServerFrame(opcode byte, payload []byte) []byte {
	frame := []byte{0x80 | opcode}
	switch {
	case len(payload) < 126:
		frame = append(frame, byte(len(payload)))
	case len(payload) <= 0xffff:
		frame = append(frame, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		ext := make([]byte, 9)
		ext[0] = 127
		binary.BigEndian.PutUint64(ext[1:], uint64(len(payload)))
		frame = append(frame, ext...)
	}
	return append(frame, payload...)
}

func TestReadFrameWebSocketPush(t *testing.T) {
	short := bytes.Repeat([]byte{0x11}, 5)
	long := bytes.Repeat([]byte{0x22}, 300) // needs the 126 extended length
	var buf bytes.Buffer
	buf.Write(wsServerFrame(0x9, []byte("ping"))) // control frame: skipped
	buf.Write(wsServerFrame(0x2, short))
	buf.Write(wsServerFrame(0x2, long))
	c := readerConn(buf.Bytes())

	f1, err := c.readFrame()
	if err != nil {
		t.Fatal(err)
	}
	if f1.kind != frameEnvelope || !bytes.Equal(f1.payload, short) {
		t.Fatalf("frame 1 = %+v", f1)
	}
	f2, err := c.readFrame()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f2.payload, long) {
		t.Fatalf("frame 2 payload = %d bytes, want 300", len(f2.payload))
	}
}

func TestReadFrameWebSocketClose(t *testing.T) {
	if _, err := readerConn(wsServerFrame(0x8, nil)).readFrame(); err == nil {
		t.Fatal("expected error on close frame")
	}
}

func TestReadFrameDesyncPrefix(t *testing.T) {
	// 0x80..0xFE cannot start a frame; the stream is desynced and must error
	// (which disconnects) rather than guess.
	if _, err := readerConn([]byte{0x80, 0, 0, 0}).readFrame(); err == nil {
		t.Fatal("expected error on invalid prefix")
	}
}

// fakeTCPServer speaks the abridged protocol on a loopback listener: it
// validates the 0xEF token handshake, answers pings with pongs, answers
// ApiRequestEvent envelopes with a chunked API response, echoes other
// cid-carrying envelopes, and pushes one server event after the handshake.
func fakeTCPServer(t *testing.T, wantToken string) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)

		if err := readHandshake(r, wantToken); err != nil {
			t.Error(err)
			return
		}

		// Server-pushed event (cid 0) right after the handshake.
		push, _ := proto.Marshal(&rtapi.Envelope{ChannelMessage: &api.ChannelMessage{ChannelId: 1}})
		_, _ = conn.Write(encodeAbridgedFrame(push))

		for {
			prefix, err := r.ReadByte()
			if err != nil {
				return
			}
			if prefix == tcpPongPrefix {
				var cid [2]byte
				if _, err := io.ReadFull(r, cid[:]); err != nil {
					return
				}
				_, _ = conn.Write([]byte{tcpPongPrefix, cid[0], cid[1]})
				continue
			}
			payloadLen, err := readAbridgedLength(r, prefix)
			if err != nil {
				t.Error(err)
				return
			}
			payload := make([]byte, payloadLen)
			if _, err := io.ReadFull(r, payload); err != nil {
				return
			}
			env := &rtapi.Envelope{}
			if err := proto.Unmarshal(trimAbridgedPadding(payload), env); err != nil {
				t.Errorf("server decode: %v", err)
				return
			}
			if env.ApiRequestEvent != nil {
				cid := uint16(env.Cid)
				if env.ApiRequestEvent.ApiName == "GetAccount" {
					// Chunked response to exercise reassembly.
					_, _ = conn.Write(tcpAPIFrame(cid, 0, false, []byte("resp:")))
					_, _ = conn.Write(tcpAPIFrame(cid, 0, true, env.ApiRequestEvent.Body))
				} else {
					// Echo the request body so proto payloads round-trip.
					_, _ = conn.Write(tcpAPIFrame(cid, 0, true, env.ApiRequestEvent.Body))
				}
				continue
			}
			if env.Cid != 0 {
				echo, _ := proto.Marshal(&rtapi.Envelope{Cid: env.Cid})
				_, _ = conn.Write(encodeAbridgedFrame(echo))
			}
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func readHandshake(r *bufio.Reader, wantToken string) error {
	prefix, err := r.ReadByte()
	if err != nil {
		return err
	}
	if prefix != tcpHandshakePrefix {
		return fmt.Errorf("handshake prefix = 0x%02x, want 0xef", prefix)
	}
	lenByte, err := r.ReadByte()
	if err != nil {
		return err
	}
	tok := make([]byte, int(lenByte)*4)
	if _, err := io.ReadFull(r, tok); err != nil {
		return err
	}
	if got := string(trimAbridgedPadding(tok)); got != wantToken {
		return fmt.Errorf("token = %q, want %q", got, wantToken)
	}
	return nil
}

func TestTCPSocketLoopback(t *testing.T) {
	addr, closeFn := fakeTCPServer(t, "test-token")
	defer closeFn()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}

	gotEvent := make(chan string, 8)
	s := NewDefaultSocket("", host, port, false, func(event string, _ any) {
		gotEvent <- event
	})
	if err := s.Connect(&Session{Token: "test-token"}, false); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer s.Close()

	// Native ping frame resolves as a pong envelope.
	resp, err := s.send(&rtapi.Envelope{Ping: &rtapi.Ping{}}, 2*time.Second)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if resp.Pong == nil {
		t.Fatalf("ping response = %+v, want pong", resp)
	}

	// API request over the socket, response reassembled from two chunks.
	body, err := s.sendApiRequest("GetAccount", []byte("hello"))
	if err != nil {
		t.Fatalf("sendApiRequest: %v", err)
	}
	if string(body) != "resp:hello" {
		t.Fatalf("api body = %q, want %q", body, "resp:hello")
	}

	// Envelope send-and-wait echoes by cid.
	if _, err := s.send(&rtapi.Envelope{}, 2*time.Second); err != nil {
		t.Fatalf("envelope send: %v", err)
	}

	// Server push dispatched as an event.
	select {
	case ev := <-gotEvent:
		if ev != EventChannelMessage {
			t.Fatalf("event = %q, want %q", ev, EventChannelMessage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server push event not dispatched")
	}
}
