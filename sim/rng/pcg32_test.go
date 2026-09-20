package rng

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestPCG32ReferenceVectors(t *testing.T) {
	f, err := os.Open("testdata/pcg32_vectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for line := 0; sc.Scan(); {
		line++
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Split(text, ",")
		if len(fields) != 7 {
			t.Fatalf("line %d: want 7 fields, got %d", line, len(fields))
		}
		var vals [7]uint64
		for i, fs := range fields {
			v, err := strconv.ParseUint(fs, 16, 64)
			if err != nil {
				t.Fatalf("line %d field %d: %v", line, i, err)
			}
			vals[i] = v
		}
		p := Seed(vals[0], vals[1])
		for i := 2; i < 7; i++ {
			if got := p.Next(); uint64(got) != vals[i] {
				t.Errorf("line %d draw %d: got %08x, want %08x", line, i-2, got, vals[i])
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSeedDeterminism(t *testing.T) {
	a := Seed(123, 456)
	b := Seed(123, 456)
	for i := 0; i < 1000; i++ {
		if a.Next() != b.Next() {
			t.Fatalf("draw %d diverged", i)
		}
	}
}

func TestNextUint64UsesBothWords(t *testing.T) {
	p := Seed(7, 7)
	seenLow, seenHigh := false, false
	for i := 0; i < 50; i++ {
		v := p.NextUint64()
		if v&0xffffffff != 0 {
			seenLow = true
		}
		if v>>32 != 0 {
			seenHigh = true
		}
	}
	if !seenLow || !seenHigh {
		t.Fatal("NextUint64 did not produce both words")
	}
}

func TestStreamsDeterminismAndIndependence(t *testing.T) {
	sa := NewStreams(99)
	sb := NewStreams(99)
	sc := NewStreams(100)
	differs := false
	for i := 0; i < 100; i++ {
		if sa.Next(DomainMapGen) != sb.Next(DomainMapGen) {
			t.Fatal("same seed diverged")
		}
		if sa.Next(DomainCombat) != sc.Next(DomainMapGen) {
			differs = true
		}
	}
	if !differs {
		t.Fatal("different seeds produced identical combat stream")
	}
}

func TestStreamsAppendState(t *testing.T) {
	s := NewStreams(5)
	before := s.AppendState(nil)
	if len(before) != 4*16 {
		t.Fatalf("state length = %d, want %d", len(before), 64)
	}
	s.Next(DomainMisc)
	after := s.AppendState(nil)
	if string(before) == string(after) {
		t.Fatal("state unchanged after draw")
	}
}

func TestStreamsInvalidDomainPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	s := NewStreams(1)
	s.Next(Domain(9))
}
