// Package proto implements the protobuf wire format for the subset of
// mezon.api and mezon.realtime messages used by the light SDK.
//
// Field numbers mirror the ts-proto generated code in
// mezon-light-sdk/src/proto (source: rtapi/realtime.proto and api.proto,
// protoc v4.25.2).
//
// Note on IDs: the upstream .proto declares snowflake IDs as int64; the JS
// SDK represents them as decimal strings. This package keeps the string
// representation in Go structs and converts to/from int64 varints on the
// wire.
package proto

import (
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
)

// ---------------------------------------------------------------------------
// Encode helpers. Each helper omits the field when it holds the proto3
// default value, matching ts-proto's behavior.
// ---------------------------------------------------------------------------

func appendString(b []byte, num protowire.Number, v string) []byte {
	if v == "" {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendString(b, v)
}

func appendBytes(b []byte, num protowire.Number, v []byte) []byte {
	if len(v) == 0 {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}

func appendBool(b []byte, num protowire.Number, v bool) []byte {
	if !v {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, 1)
}

func appendInt32(b []byte, num protowire.Number, v int32) []byte {
	if v == 0 {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.VarintType)
	// Negative int32 values are sign-extended to 64 bits on the wire.
	return protowire.AppendVarint(b, uint64(int64(v)))
}

func appendUint32(b []byte, num protowire.Number, v uint32) []byte {
	if v == 0 {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, uint64(v))
}

// appendID encodes a decimal-string snowflake ID as an int64 varint field.
func appendID(b []byte, num protowire.Number, s string) []byte {
	v, ok := parseID(s)
	if !ok || v == 0 {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, uint64(v))
}

func parseID(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, true
	}
	// IDs above math.MaxInt64 still fit in the uint64 wire representation.
	if u, err := strconv.ParseUint(s, 10, 64); err == nil {
		return int64(u), true
	}
	return 0, false
}

func formatID(v uint64) string {
	return strconv.FormatInt(int64(v), 10)
}

type appender interface {
	MarshalAppend(b []byte) []byte
}

func appendMessage(b []byte, num protowire.Number, m appender) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, m.MarshalAppend(nil))
}

func appendStringMap(b []byte, num protowire.Number, m map[string]string) []byte {
	for k, v := range m {
		var inner []byte
		inner = appendString(inner, 1, k)
		inner = appendString(inner, 2, v)
		b = protowire.AppendTag(b, num, protowire.BytesType)
		b = protowire.AppendBytes(b, inner)
	}
	return b
}

func appendPackedIDs(b []byte, num protowire.Number, ids []string) []byte {
	if len(ids) == 0 {
		return b
	}
	var inner []byte
	for _, s := range ids {
		v, _ := parseID(s)
		inner = protowire.AppendVarint(inner, uint64(v))
	}
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, inner)
}

func appendPackedBools(b []byte, num protowire.Number, vs []bool) []byte {
	if len(vs) == 0 {
		return b
	}
	var inner []byte
	for _, v := range vs {
		if v {
			inner = protowire.AppendVarint(inner, 1)
		} else {
			inner = protowire.AppendVarint(inner, 0)
		}
	}
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, inner)
}

// google.protobuf well-known wrapper types (value field 1).

func appendInt32Value(b []byte, num protowire.Number, v *int32) []byte {
	if v == nil {
		return b
	}
	inner := appendInt32(nil, 1, *v)
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, inner)
}

func appendBoolValue(b []byte, num protowire.Number, v *bool) []byte {
	if v == nil {
		return b
	}
	inner := appendBool(nil, 1, *v)
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, inner)
}

func appendStringValue(b []byte, num protowire.Number, v *string) []byte {
	if v == nil {
		return b
	}
	inner := appendString(nil, 1, *v)
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, inner)
}

// ---------------------------------------------------------------------------
// Decode helpers. Unknown fields and wire-type mismatches are skipped, like
// ts-proto's generated decoders.
// ---------------------------------------------------------------------------

type decoder struct {
	b   []byte
	err error
}

func (d *decoder) next() (protowire.Number, protowire.Type, bool) {
	if d.err != nil || len(d.b) == 0 {
		return 0, 0, false
	}
	num, typ, n := protowire.ConsumeTag(d.b)
	if n < 0 {
		d.err = protowire.ParseError(n)
		return 0, 0, false
	}
	d.b = d.b[n:]
	return num, typ, true
}

func (d *decoder) skip(num protowire.Number, typ protowire.Type) {
	if d.err != nil {
		return
	}
	n := protowire.ConsumeFieldValue(num, typ, d.b)
	if n < 0 {
		d.err = protowire.ParseError(n)
		return
	}
	d.b = d.b[n:]
}

func (d *decoder) varint() uint64 {
	if d.err != nil {
		return 0
	}
	v, n := protowire.ConsumeVarint(d.b)
	if n < 0 {
		d.err = protowire.ParseError(n)
		return 0
	}
	d.b = d.b[n:]
	return v
}

func (d *decoder) str() string {
	if d.err != nil {
		return ""
	}
	v, n := protowire.ConsumeString(d.b)
	if n < 0 {
		d.err = protowire.ParseError(n)
		return ""
	}
	d.b = d.b[n:]
	return v
}

// raw returns the length-delimited payload without copying. Use only for
// immediate sub-message decoding.
func (d *decoder) raw() []byte {
	if d.err != nil {
		return nil
	}
	v, n := protowire.ConsumeBytes(d.b)
	if n < 0 {
		d.err = protowire.ParseError(n)
		return nil
	}
	d.b = d.b[n:]
	return v
}

// bytes returns a copy of the length-delimited payload, safe to retain.
func (d *decoder) bytes() []byte {
	v := d.raw()
	if v == nil {
		return nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out
}

func (d *decoder) id() string {
	return formatID(d.varint())
}

func (d *decoder) int32() int32 {
	return int32(d.varint())
}

func (d *decoder) uint32() uint32 {
	return uint32(d.varint())
}

func (d *decoder) bool() bool {
	return d.varint() != 0
}

func (d *decoder) sub(m interface{ Unmarshal([]byte) error }) {
	raw := d.raw()
	if d.err != nil {
		return
	}
	if err := m.Unmarshal(raw); err != nil {
		d.err = err
	}
}

func (d *decoder) stringMapEntry(m map[string]string) {
	raw := d.raw()
	if d.err != nil {
		return
	}
	sub := decoder{b: raw}
	var key, value string
	for {
		num, typ, ok := sub.next()
		if !ok {
			break
		}
		switch {
		case num == 1 && typ == protowire.BytesType:
			key = sub.str()
		case num == 2 && typ == protowire.BytesType:
			value = sub.str()
		default:
			sub.skip(num, typ)
		}
	}
	if sub.err != nil {
		d.err = sub.err
		return
	}
	m[key] = value
}

func (d *decoder) packedIDs(typ protowire.Type, out []string) []string {
	if typ == protowire.VarintType {
		return append(out, d.id())
	}
	raw := d.raw()
	if d.err != nil {
		return out
	}
	for len(raw) > 0 {
		v, n := protowire.ConsumeVarint(raw)
		if n < 0 {
			d.err = protowire.ParseError(n)
			return out
		}
		raw = raw[n:]
		out = append(out, formatID(v))
	}
	return out
}

func (d *decoder) packedBools(typ protowire.Type, out []bool) []bool {
	if typ == protowire.VarintType {
		return append(out, d.bool())
	}
	raw := d.raw()
	if d.err != nil {
		return out
	}
	for len(raw) > 0 {
		v, n := protowire.ConsumeVarint(raw)
		if n < 0 {
			d.err = protowire.ParseError(n)
			return out
		}
		raw = raw[n:]
		out = append(out, v != 0)
	}
	return out
}

func (d *decoder) int32Value() *int32 {
	raw := d.raw()
	if d.err != nil {
		return nil
	}
	sub := decoder{b: raw}
	var out int32
	for {
		num, typ, ok := sub.next()
		if !ok {
			break
		}
		if num == 1 && typ == protowire.VarintType {
			out = sub.int32()
		} else {
			sub.skip(num, typ)
		}
	}
	if sub.err != nil {
		d.err = sub.err
		return nil
	}
	return &out
}

func (d *decoder) boolValue() *bool {
	raw := d.raw()
	if d.err != nil {
		return nil
	}
	sub := decoder{b: raw}
	var out bool
	for {
		num, typ, ok := sub.next()
		if !ok {
			break
		}
		if num == 1 && typ == protowire.VarintType {
			out = sub.bool()
		} else {
			sub.skip(num, typ)
		}
	}
	if sub.err != nil {
		d.err = sub.err
		return nil
	}
	return &out
}

func (d *decoder) stringValue() *string {
	raw := d.raw()
	if d.err != nil {
		return nil
	}
	sub := decoder{b: raw}
	var out string
	for {
		num, typ, ok := sub.next()
		if !ok {
			break
		}
		if num == 1 && typ == protowire.BytesType {
			out = sub.str()
		} else {
			sub.skip(num, typ)
		}
	}
	if sub.err != nil {
		d.err = sub.err
		return nil
	}
	return &out
}
