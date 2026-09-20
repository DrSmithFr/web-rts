package fixed

import "testing"

func raw(v int32) F16 { return F16(v) }

func TestFromInt(t *testing.T) {
	tests := []struct {
		in   int32
		want F16
	}{
		{0, 0},
		{1, 65536},
		{-1, -65536},
		{32767, 2147418112},
		{32768, -2147483648}, // documented wrap
		{65536, 0},           // documented wrap
	}
	for _, tt := range tests {
		if got := FromInt(tt.in); got != tt.want {
			t.Errorf("FromInt(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name string
		in   F16
		want int32
	}{
		{"one", One, 1},
		{"minus one", -One, -1},
		{"floor positive", 65535, 0},
		{"floor negative one below", -65535, -1},
		{"floor negative", -65537, -2},
		{"half", 32768, 0},
	}
	for _, tt := range tests {
		if got := tt.in.ToInt(); got != tt.want {
			t.Errorf("%s: ToInt() = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestRound(t *testing.T) {
	tests := []struct {
		name string
		in   F16
		want int32
	}{
		{"just below half", 32767, 0},
		{"half rounds up", 32768, 1},
		{"minus half rounds toward plus infinity", -32768, 0},
		{"just above minus half", -32767, 0},
		{"minus 0.51", -33424, -1},
	}
	for _, tt := range tests {
		if got := tt.in.Round(); got != tt.want {
			t.Errorf("%s: Round() = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestMul(t *testing.T) {
	tests := []struct {
		name string
		a, b F16
		want F16
	}{
		{"1 x 1", One, One, One},
		{"0.5 x 0.5", 32768, 32768, 16384},
		{"2 x 3", FromInt(2), FromInt(3), FromInt(6)},
		{"-0.5 x 0.5", -32768, 32768, -16384},
		{"trunc toward minus infinity", -65535, 32768, -32768},
		{"wrap overflow", FromInt(65535), FromInt(2), FromInt(-2)},
	}
	for _, tt := range tests {
		if got := tt.a.Mul(tt.b); got != tt.want {
			t.Errorf("%s: %d.Mul(%d) = %d, want %d", tt.name, tt.a, tt.b, got, tt.want)
		}
	}
}

func TestMulRound(t *testing.T) {
	tests := []struct {
		name string
		a, b F16
		want F16
	}{
		{"exact quarter", 32768, 32768, 16384},
		{"1 ulp x 0 rounds down", 1, 98303, 1},
		{"1 ulp x 1.5 rounds up", 1, 98304, 2},
		{"minus 1 ulp x 1.5 half toward plus infinity", -1, 98304, -1},
		{"1 ulp x 1 ulp rounds to zero", 1, 1, 0},
	}
	for _, tt := range tests {
		if got := tt.a.MulRound(tt.b); got != tt.want {
			t.Errorf("%s: MulRound = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestDiv(t *testing.T) {
	tests := []struct {
		name string
		a, b F16
		want F16
	}{
		{"1 / 2", One, FromInt(2), 32768},
		{"1 / 3 truncates", One, FromInt(3), 21845},
		{"minus 1 / 3 truncates toward zero", -One, FromInt(3), -21845},
		{"zero divisor yields zero", FromInt(7), 0, 0},
		{"6 / 3", FromInt(6), FromInt(3), FromInt(2)},
	}
	for _, tt := range tests {
		if got := tt.a.Div(tt.b); got != tt.want {
			t.Errorf("%s: Div = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestMinMax(t *testing.T) {
	if got := Min(raw(-5), raw(3)); got != raw(-5) {
		t.Errorf("Min = %d, want -5", got)
	}
	if got := Max(raw(-5), raw(3)); got != raw(3) {
		t.Errorf("Max = %d, want 3", got)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		in   F16
		want string
	}{
		{Zero, "0.00000"},
		{One, "1.00000"},
		{-One, "-1.00000"},
		{1, "0.00002"},
		{21845, "0.33333"},
		{-32768, "-0.50000"},
		{0x7FFFFFFF, "32767.99998"},
		{-2147483648, "-32768.00000"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("String(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
