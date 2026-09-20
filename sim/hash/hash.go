// Package hash implements the FNV-1a 64-bit state hash used to detect
// lockstep divergence. Byte order on the hash stream is explicitly
// little-endian, so the digest is identical on every platform.
package hash

const (
	offset64 = 14695981039346656037
	prime64  = 1099511628211
)

// FNV1a64 is an incremental FNV-1a 64 hasher.
type FNV1a64 struct {
	h uint64
}

// New returns a hasher seeded with the FNV offset basis.
func New() *FNV1a64 { return &FNV1a64{h: offset64} }

// Write folds raw bytes into the digest. Implements io.Writer.
func (f *FNV1a64) Write(p []byte) (int, error) {
	for _, b := range p {
		f.h ^= uint64(b)
		f.h *= prime64
	}
	return len(p), nil
}

// WriteString folds a string label into the digest.
func (f *FNV1a64) WriteString(s string) {
	for i := 0; i < len(s); i++ {
		f.h ^= uint64(s[i])
		f.h *= prime64
	}
}

// WriteU32 folds an unsigned 32-bit value, little-endian.
func (f *FNV1a64) WriteU32(v uint32) {
	f.h ^= uint64(v & 0xff)
	f.h *= prime64
	f.h ^= uint64((v >> 8) & 0xff)
	f.h *= prime64
	f.h ^= uint64((v >> 16) & 0xff)
	f.h *= prime64
	f.h ^= uint64((v >> 24) & 0xff)
	f.h *= prime64
}

// WriteU64 folds an unsigned 64-bit value, little-endian.
func (f *FNV1a64) WriteU64(v uint64) {
	for i := 0; i < 8; i++ {
		f.h ^= (v >> (8 * i)) & 0xff
		f.h *= prime64
	}
}

// Sum returns the current digest.
func (f *FNV1a64) Sum() uint64 { return f.h }

// Sum64 hashes a byte slice in one shot.
func Sum64(data []byte) uint64 {
	h := New()
	h.Write(data)
	return h.Sum()
}
