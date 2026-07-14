package mezon

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/gorilla/websocket"
)

// TransportKind selects the wire transport used by DefaultSocket.
type TransportKind string

const (
	// TransportTCP is the raw TLS/TCP transport with MTProto-style "abridged"
	// framing, port of MezonNetworkTcpTransporter (.NET) and MezonNetworkAdapter
	// (mezon-js-protobuf). This is the default.
	TransportTCP TransportKind = "tcp"
	// TransportWebSocket is the protobuf-over-WebSocket transport, port of
	// WebSocketAdapterPb in the TS SDK.
	TransportWebSocket TransportKind = "websocket"
)

// Abridged TCP framing constants, port of MezonTransportFrameCodec.cs and
// abridged_tcp_adapter.ts.
const (
	// tcpHandshakePrefix is the magic byte opening the post-TLS handshake.
	tcpHandshakePrefix = 0xef
	// tcpPongPrefix marks a 3-byte ping/pong frame: prefix + cid uint16 BE.
	tcpPongPrefix = 0x00
	// abridgedExtendedPrefix escapes the 1-byte length header when the padded
	// payload is >= 127*4 bytes; a 3-byte little-endian length/4 follows.
	abridgedExtendedPrefix = 0x7f
	// maxTCPFrameSize bounds a single inbound frame so a corrupt length field
	// cannot trigger an absurd allocation.
	maxTCPFrameSize = 64 << 20
)

type frameKind int

const (
	frameEnvelope frameKind = iota
	frameAPIChunk
	framePong
)

// wireFrame is one inbound message normalized across transports.
type wireFrame struct {
	kind    frameKind
	cid     int32  // API chunk / pong frames carry only 16 cid bits
	code    uint32 // API chunk response code (high 16 bits of the code word)
	fin     bool   // API chunk fin flag (low 16 bits == 0xFF)
	payload []byte // envelope bytes or API chunk body
}

// transportConn is one established connection: a framed, message-oriented view
// of the wire shared by the WebSocket and TCP transports. Writes are
// serialized by DefaultSocket.writeMu; readFrame is called from a single read
// loop goroutine.
type transportConn interface {
	// readFrame blocks for the next inbound frame, reassembling from the
	// underlying stream as needed.
	readFrame() (*wireFrame, error)
	// writeEnvelope writes a proto-marshaled rtapi.Envelope.
	writeEnvelope(data []byte) error
	// nativePing reports whether heartbeats use a dedicated wire frame instead
	// of an Envelope{Ping}; when true, writePing must be used.
	nativePing() bool
	writePing(cid int32) error
	close() error
}

// wsConn adapts *websocket.Conn to transportConn, port of
// WebSocketAdapterPb framing.
type wsConn struct {
	conn *websocket.Conn
	logf func(format string, args ...any)
}

func (w *wsConn) readFrame() (*wireFrame, error) {
	for {
		_, data, err := w.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		if len(data) > 0 && data[0] == rawFramePrefix {
			f, ok := parseWSRawFrame(data)
			if !ok {
				w.logf("mezon: raw frame too small to contain headers")
				continue
			}
			return f, nil
		}
		return &wireFrame{kind: frameEnvelope, payload: data}, nil
	}
}

// parseWSRawFrame decodes a 0xFF-prefixed API-response WebSocket message:
// prefix, cid uint16 BE, code uint32 BE (high 16 bits response code, low 16
// bits fin flag). Unlike TCP there is no length field — the rest of the
// message is one chunk.
func parseWSRawFrame(data []byte) (*wireFrame, bool) {
	if len(data) < rawHeaderLength {
		return nil, false
	}
	code := binary.BigEndian.Uint32(data[3:7])
	return &wireFrame{
		kind:    frameAPIChunk,
		cid:     int32(binary.BigEndian.Uint16(data[1:3])),
		code:    (code >> 16) & 0xffff,
		fin:     code&0xffff == rawCodeFin,
		payload: data[rawHeaderLength:],
	}, true
}

func (w *wsConn) writeEnvelope(data []byte) error {
	return w.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (w *wsConn) nativePing() bool { return false }

func (w *wsConn) writePing(int32) error {
	return fmt.Errorf("mezon: websocket transport has no native ping frame")
}

func (w *wsConn) close() error { return w.conn.Close() }

// tcpConn is the abridged TCP transport. The server multiplexes three inbound
// frame shapes on the first byte: 0x00 pong, 0xFF API-response chunk, anything
// else an abridged-framed Envelope.
type tcpConn struct {
	conn net.Conn
	r    *bufio.Reader
}

// dialTCP connects, performs the TLS handshake when useSSL is set, and sends
// the 0xEF token handshake, port of ConnectAsync + HandshakeAsync in
// MezonNetworkTcpTransporter.cs.
func dialTCP(host, port string, useSSL, insecureSkipVerify bool, token string) (*tcpConn, error) {
	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: defaultConnectTimeout}
	var conn net.Conn
	var err error
	if useSSL {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: insecureSkipVerify,
		})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(defaultConnectTimeout))
	if _, err := conn.Write(tcpHandshakeFrame(token)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Time{})
	return &tcpConn{conn: conn, r: bufio.NewReader(conn)}, nil
}

// tcpHandshakeFrame builds 0xEF + abridged-framed UTF-8 token.
func tcpHandshakeFrame(token string) []byte {
	framed := encodeAbridgedFrame([]byte(token))
	return append([]byte{tcpHandshakePrefix}, framed...)
}

// encodeAbridgedFrame prepends the abridged length header and zero-pads the
// payload to a multiple of 4: header is padded-length/4 as one byte, or 0x7F +
// 3-byte little-endian length/4 when it does not fit.
func encodeAbridgedFrame(payload []byte) []byte {
	padding := (4 - len(payload)%4) & 3
	lenDiv4 := (len(payload) + padding) / 4
	var header []byte
	if lenDiv4 < 127 {
		header = []byte{byte(lenDiv4)}
	} else {
		header = []byte{abridgedExtendedPrefix, byte(lenDiv4), byte(lenDiv4 >> 8), byte(lenDiv4 >> 16)}
	}
	frame := make([]byte, 0, len(header)+len(payload)+padding)
	frame = append(frame, header...)
	frame = append(frame, payload...)
	return append(frame, make([]byte, padding)...)
}

// trimAbridgedPadding strips the up-to-3 zero bytes appended by the abridged
// framing, port of MezonTransportFrameCodec.TrimPadding.
func trimAbridgedPadding(b []byte) []byte {
	maxPadding := 3
	if len(b) < maxPadding {
		maxPadding = len(b)
	}
	trimmed := 0
	for trimmed < maxPadding && b[len(b)-1-trimmed] == 0x00 {
		trimmed++
	}
	return b[:len(b)-trimmed]
}

func (t *tcpConn) readFrame() (*wireFrame, error) {
	for {
		f, err := t.readFrameOnce()
		if err != nil || f != nil {
			return f, err
		}
		// nil, nil: a control frame was consumed; keep reading.
	}
}

// readFrameOnce reads one wire frame; it returns (nil, nil) after consuming a
// frame that carries no envelope (WebSocket control frames).
func (t *tcpConn) readFrameOnce() (*wireFrame, error) {
	prefix, err := t.r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch {
	case prefix == tcpPongPrefix:
		var b [2]byte
		if _, err := io.ReadFull(t.r, b[:]); err != nil {
			return nil, err
		}
		return &wireFrame{kind: framePong, cid: int32(binary.BigEndian.Uint16(b[:]))}, nil
	case prefix == rawFramePrefix:
		// cid uint16 BE, code uint32 BE, payload length uint32 BE. The explicit
		// length is the TCP addition over the WebSocket raw frame.
		var hdr [10]byte
		if _, err := io.ReadFull(t.r, hdr[:]); err != nil {
			return nil, err
		}
		code := binary.BigEndian.Uint32(hdr[2:6])
		payloadLen := binary.BigEndian.Uint32(hdr[6:10])
		if payloadLen > maxTCPFrameSize {
			return nil, fmt.Errorf("mezon: API frame length %d exceeds limit", payloadLen)
		}
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(t.r, payload); err != nil {
			return nil, err
		}
		return &wireFrame{
			kind:    frameAPIChunk,
			cid:     int32(binary.BigEndian.Uint16(hdr[0:2])),
			code:    (code >> 16) & 0xffff,
			fin:     code&0xffff == rawCodeFin,
			payload: payload,
		}, nil
	case prefix&0x80 != 0:
		// The gateway mixes framings on one stream: cid-correlated responses
		// are abridged, but server-pushed envelopes (cid-0 events) arrive as
		// standard unmasked WebSocket frames (0x82 = FIN+binary).
		return t.readWebSocketFrame(prefix)
	default:
		payloadLen, err := readAbridgedLength(t.r, prefix)
		if err != nil {
			return nil, err
		}
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(t.r, payload); err != nil {
			return nil, err
		}
		return &wireFrame{kind: frameEnvelope, payload: trimAbridgedPadding(payload)}, nil
	}
}

// readWebSocketFrame consumes an RFC 6455 frame whose first byte has already
// been read. Data frames yield an envelope; control frames are consumed and
// yield (nil, nil), except close, which ends the stream.
func (t *tcpConn) readWebSocketFrame(first byte) (*wireFrame, error) {
	const (
		wsOpText   = 0x1
		wsOpBinary = 0x2
		wsOpClose  = 0x8
	)
	opcode := first & 0x0f
	b2, err := t.r.ReadByte()
	if err != nil {
		return nil, err
	}
	masked := b2&0x80 != 0
	length := uint64(b2 & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(t.r, ext[:]); err != nil {
			return nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(t.r, ext[:]); err != nil {
			return nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxTCPFrameSize {
		return nil, fmt.Errorf("mezon: websocket frame length %d exceeds limit", length)
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(t.r, maskKey[:]); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(t.r, payload); err != nil {
		return nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	switch opcode {
	case wsOpBinary, wsOpText:
		return &wireFrame{kind: frameEnvelope, payload: payload}, nil
	case wsOpClose:
		return nil, fmt.Errorf("mezon: server sent websocket close frame")
	default:
		return nil, nil // ping/pong/continuation noise: skip
	}
}

// readAbridgedLength decodes the abridged length header whose first byte has
// already been consumed. Any prefix above 0x7F (other than the pong/API
// prefixes handled by the caller) means the stream is desynced — fatal on TCP.
func readAbridgedLength(r *bufio.Reader, prefix byte) (int, error) {
	if prefix < abridgedExtendedPrefix {
		return int(prefix) * 4, nil
	}
	if prefix != abridgedExtendedPrefix {
		return 0, fmt.Errorf("mezon: unexpected abridged frame prefix 0x%02x", prefix)
	}
	var b [3]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	n := int(b[0]) | int(b[1])<<8 | int(b[2])<<16
	if n*4 > maxTCPFrameSize {
		return 0, fmt.Errorf("mezon: abridged frame length %d exceeds limit", n*4)
	}
	return n * 4, nil
}

func (t *tcpConn) writeEnvelope(data []byte) error {
	_, err := t.conn.Write(encodeAbridgedFrame(data))
	return err
}

func (t *tcpConn) nativePing() bool { return true }

func (t *tcpConn) writePing(cid int32) error {
	frame := [3]byte{tcpPongPrefix, byte(uint16(cid) >> 8), byte(uint16(cid))}
	_, err := t.conn.Write(frame[:])
	return err
}

func (t *tcpConn) close() error { return t.conn.Close() }
