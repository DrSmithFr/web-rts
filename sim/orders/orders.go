// Package orders defines the binary payloads of gameplay orders carried in
// proto.Order. Payloads are little-endian uvarint/u16 streams; the exact
// bytes are pinned by testdata/order_vectors.txt, which both the Go side
// and the TypeScript client encode/decode against.
package orders

// Gameplay order kinds (payload kinds; proto.KindNoop stays 0). They are
// untyped so both proto.Kind and uint16 contexts accept them.
const (
	Move   = 1
	Gather = 2
	Build  = 3
	Stop   = 4
	Train  = 5
)

// MoveItem is one unit destination in a group move.
type MoveItem struct {
	ID       uint32
	X16, Y16 uint16 // tile units in 1/16 of a tile
}

// Writer appends little-endian fields to a payload buffer.
type Writer struct {
	buf []byte
}

// NewWriter returns an empty payload writer.
func NewWriter() *Writer { return &Writer{} }

// Uvarint appends an unsigned varint.
func (w *Writer) Uvarint(v uint64) { w.buf = appendUvarint(w.buf, v) }

// U16 appends a 16-bit little-endian value.
func (w *Writer) U16(v uint16) { w.buf = append(w.buf, byte(v), byte(v>>8)) }

// Bytes returns the finished payload.
func (w *Writer) Bytes() []byte { return w.buf }

// MovePayload encodes a group move.
func MovePayload(items []MoveItem) []byte {
	w := NewWriter()
	w.Uvarint(uint64(len(items)))
	for _, it := range items {
		w.Uvarint(uint64(it.ID))
		w.U16(it.X16)
		w.U16(it.Y16)
	}
	return w.Bytes()
}

// GatherPayload encodes "unit harvests node".
func GatherPayload(unit, node uint32) []byte {
	w := NewWriter()
	w.Uvarint(uint64(unit))
	w.Uvarint(uint64(node))
	return w.Bytes()
}

// BuildPayload encodes "unit builds def at tile (x, y)".
func BuildPayload(unit uint32, def uint16, x, y uint16) []byte {
	w := NewWriter()
	w.Uvarint(uint64(unit))
	w.U16(def)
	w.U16(x)
	w.U16(y)
	return w.Bytes()
}

// StopPayload encodes a group stop.
func StopPayload(ids []uint32) []byte {
	w := NewWriter()
	w.Uvarint(uint64(len(ids)))
	for _, id := range ids {
		w.Uvarint(uint64(id))
	}
	return w.Bytes()
}

// TrainPayload encodes "queue one villager at building".
func TrainPayload(building uint32) []byte {
	w := NewWriter()
	w.Uvarint(uint64(building))
	return w.Bytes()
}

// Reader consumes little-endian fields; malformed input flips OK false
// once and returns zeros afterwards.
type Reader struct {
	src []byte
	pos int
	ok  bool
}

// NewReader wraps a payload buffer.
func NewReader(src []byte) *Reader { return &Reader{src: src, ok: true} }

// Uvarint reads the next unsigned varint.
func (r *Reader) Uvarint() uint64 {
	if !r.ok {
		return 0
	}
	v, n := readUvarint(r.src[r.pos:])
	if n <= 0 {
		r.ok = false
		return 0
	}
	r.pos += n
	return v
}

// U16 reads the next 16-bit little-endian value.
func (r *Reader) U16() uint16 {
	if !r.ok || r.pos+2 > len(r.src) {
		r.ok = false
		return 0
	}
	v := uint16(r.src[r.pos]) | uint16(r.src[r.pos+1])<<8
	r.pos += 2
	return v
}

// OK reports whether every read so far succeeded.
func (r *Reader) OK() bool { return r.ok }

func appendUvarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

func readUvarint(src []byte) (uint64, int) {
	var v uint64
	var shift uint
	for i := 0; i < 10; i++ {
		if i >= len(src) {
			return 0, 0
		}
		b := src[i]
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, -1
}
