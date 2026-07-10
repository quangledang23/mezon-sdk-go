package mezon

import (
	"bytes"
	"testing"
)

// rawFrame builds a 0xFF-prefixed API response frame as produced by the
// server: cid uint16 BE, then code uint32 BE (high 16 bits response code, low
// 16 bits fin flag), then payload.
func rawFrame(cid uint16, responseCode, finFlag uint16, payload []byte) []byte {
	frame := make([]byte, rawHeaderLength, rawHeaderLength+len(payload))
	frame[0] = rawFramePrefix
	frame[1] = byte(cid >> 8)
	frame[2] = byte(cid)
	frame[3] = byte(responseCode >> 8)
	frame[4] = byte(responseCode)
	frame[5] = byte(finFlag >> 8)
	frame[6] = byte(finFlag)
	return append(frame, payload...)
}

func newTestSocket() *DefaultSocket {
	return NewDefaultSocket("", "example.com", "", true, func(string, any) {})
}

func TestHandleRawFrameSingle(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	s.cids[7] = ch
	streams := make(map[int32][][]byte)

	s.handleRawFrame(rawFrame(7, 0, rawCodeFin, []byte("hello")), streams)

	select {
	case resp := <-ch:
		if !resp.apiResponse {
			t.Fatal("expected apiResponse")
		}
		if resp.code != 0 {
			t.Fatalf("code = %d, want 0", resp.code)
		}
		if !bytes.Equal(resp.body, []byte("hello")) {
			t.Fatalf("body = %q, want %q", resp.body, "hello")
		}
	default:
		t.Fatal("no response delivered")
	}
	if len(streams) != 0 {
		t.Fatal("stream buffer not cleaned up")
	}
}

func TestHandleRawFrameChunked(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	s.cids[9] = ch
	streams := make(map[int32][][]byte)

	s.handleRawFrame(rawFrame(9, 0, 0, []byte("foo")), streams)
	s.handleRawFrame(rawFrame(9, 0, 0, []byte("bar")), streams)
	if len(streams[9]) != 2 {
		t.Fatalf("buffered chunks = %d, want 2", len(streams[9]))
	}
	s.handleRawFrame(rawFrame(9, 0, rawCodeFin, []byte("baz")), streams)

	resp := <-ch
	if !bytes.Equal(resp.body, []byte("foobarbaz")) {
		t.Fatalf("body = %q, want %q", resp.body, "foobarbaz")
	}
	if len(streams) != 0 {
		t.Fatal("stream buffer not cleaned up")
	}
}

func TestHandleRawFrameErrorCode(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	s.cids[3] = ch
	streams := make(map[int32][][]byte)

	s.handleRawFrame(rawFrame(3, 13, rawCodeFin, nil), streams)

	resp := <-ch
	if resp.code != 13 {
		t.Fatalf("code = %d, want 13", resp.code)
	}
}

func TestHandleRawFrameTooShort(t *testing.T) {
	s := newTestSocket()
	streams := make(map[int32][][]byte)
	// Must not panic or buffer anything.
	s.handleRawFrame([]byte{rawFramePrefix, 0, 1}, streams)
	if len(streams) != 0 {
		t.Fatal("short frame should be ignored")
	}
}

func TestTakeRawCidTruncatedFallback(t *testing.T) {
	s := newTestSocket()
	ch := make(chan *socketResponse, 1)
	// cid counter past 65535: raw frames only carry the low 16 bits.
	s.cids[65536+42] = ch

	got, ok := s.takeRawCid(42)
	if !ok || got != ch {
		t.Fatal("expected truncated-cid fallback to find the pending request")
	}
	if len(s.cids) != 0 {
		t.Fatal("pending cid not removed")
	}
}
