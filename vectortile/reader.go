package vectortile

import (
	"encoding/binary"
	"fmt"
)

// wireType is a protobuf wire format type as used by the (small) subset
// of the format needed to decode Mapbox Vector Tiles: varint,
// length-delimited and the two fixed-width encodings. Groups (wire types
// 3 and 4) are deprecated and never used by the vector tile schema, so
// they are intentionally unsupported.
type wireType int

const (
	wireVarint  wireType = 0
	wireFixed64 wireType = 1
	wireBytes   wireType = 2
	wireFixed32 wireType = 5
)

// pbReader is a minimal streaming protobuf wire-format decoder. It only
// implements what is needed to walk the small, fixed schema of a vector
// tile (see https://github.com/mapbox/vector-tile-spec), avoiding a
// dependency on a full protobuf runtime or generated code.
type pbReader struct {
	buf []byte
	pos int
}

func newReader(buf []byte) *pbReader {
	return &pbReader{buf: buf}
}

func (r *pbReader) done() bool {
	return r.pos >= len(r.buf)
}

func (r *pbReader) readVarint() (uint64, error) {
	var result uint64
	var shift uint
	for {
		if r.pos >= len(r.buf) {
			return 0, fmt.Errorf("vectortile: unexpected end of buffer reading varint")
		}
		b := r.buf[r.pos]
		r.pos++
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, fmt.Errorf("vectortile: varint exceeds 64 bits")
		}
	}
}

func (r *pbReader) readTag() (fieldNum int, wt wireType, err error) {
	v, err := r.readVarint()
	if err != nil {
		return 0, 0, err
	}
	return int(v >> 3), wireType(v & 0x7), nil
}

func (r *pbReader) advance(n int) error {
	if n < 0 || r.pos+n > len(r.buf) {
		return fmt.Errorf("vectortile: unexpected end of buffer")
	}
	r.pos += n
	return nil
}

func (r *pbReader) readBytes() ([]byte, error) {
	n, err := r.readVarint()
	if err != nil {
		return nil, err
	}
	if n > uint64(len(r.buf)-r.pos) {
		return nil, fmt.Errorf("vectortile: length-delimited field exceeds buffer")
	}
	b := r.buf[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return b, nil
}

func (r *pbReader) readString() (string, error) {
	b, err := r.readBytes()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *pbReader) readFixed64() (uint64, error) {
	if err := checkAvail(r, 8); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint64(r.buf[r.pos:])
	r.pos += 8
	return v, nil
}

func (r *pbReader) readFixed32() (uint32, error) {
	if err := checkAvail(r, 4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return v, nil
}

func checkAvail(r *pbReader, n int) error {
	if r.pos+n > len(r.buf) {
		return fmt.Errorf("vectortile: unexpected end of buffer")
	}
	return nil
}

// skip discards the value following a tag of the given wire type,
// without interpreting it.
func (r *pbReader) skip(wt wireType) error {
	switch wt {
	case wireVarint:
		_, err := r.readVarint()
		return err
	case wireFixed64:
		return r.advance(8)
	case wireBytes:
		_, err := r.readBytes()
		return err
	case wireFixed32:
		return r.advance(4)
	default:
		return fmt.Errorf("vectortile: unsupported wire type %d", wt)
	}
}

// readPackedVarints reads a length-delimited field as a sequence of
// packed varints, as used for MVT's packed repeated uint32 fields.
func (r *pbReader) readPackedVarints() ([]uint32, error) {
	b, err := r.readBytes()
	if err != nil {
		return nil, err
	}
	sub := newReader(b)
	var out []uint32
	for !sub.done() {
		v, err := sub.readVarint()
		if err != nil {
			return nil, err
		}
		out = append(out, uint32(v))
	}
	return out, nil
}

// zigzagDecode decodes a zigzag-encoded signed integer, as used for the
// parameter values of vector tile geometry commands.
func zigzagDecode(v uint32) int32 {
	return int32(v>>1) ^ -int32(v&1)
}
