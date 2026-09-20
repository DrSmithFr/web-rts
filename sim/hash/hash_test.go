package hash

import "testing"

func TestKnownVectors(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
	}{
		{"", 0xcbf29ce484222325},
		{"a", 0xaf63dc4c8601ec8c},
		{"foobar", 0x85944171f73967e8},
	}
	for _, tt := range tests {
		if got := Sum64([]byte(tt.in)); got != tt.want {
			t.Errorf("Sum64(%q) = %#016x, want %#016x", tt.in, got, tt.want)
		}
	}
}

func TestIncrementalEqualsOneShot(t *testing.T) {
	a := New()
	a.WriteString("rts.world/v1")
	a.WriteU32(0xdeadbeef)
	a.WriteU64(0x0123456789abcdef)

	b := New()
	b.Write([]byte("rts.world/v1"))
	b.Write([]byte{0xef, 0xbe, 0xad, 0xde})
	b.Write([]byte{0xef, 0xcd, 0xab, 0x89, 0x67, 0x45, 0x23, 0x01})

	if a.Sum() != b.Sum() {
		t.Fatalf("incremental %#016x != one-shot %#016x", a.Sum(), b.Sum())
	}
}

func TestDistinctInputs(t *testing.T) {
	seen := make(map[uint64]string)
	cases := []struct {
		s32 uint32
		s64 uint64
		tag string
	}{
		{0, 0, "zero"},
		{1, 0, "u32=1"},
		{0, 1, "u64=1"},
		{0, 0x100, "u64=256"},
	}
	for _, c := range cases {
		h := New()
		h.WriteU32(c.s32)
		h.WriteU64(c.s64)
		sum := h.Sum()
		if prev, dup := seen[sum]; dup {
			t.Fatalf("%s collides with %s", c.tag, prev)
		}
		seen[sum] = c.tag
	}
}
