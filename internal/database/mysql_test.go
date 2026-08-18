package database

import (
	"math"
	"testing"
)

func TestBilledTrafficUsesDocumentedRounding(t *testing.T) {
	for _, fixture := range []struct {
		raw      int64
		rate     float64
		expected int64
	}{
		{1, 1.5, 2},
		{3, 0.5, 2},
		{100, 0.01, 1},
	} {
		actual, err := billed(fixture.raw, fixture.rate)
		if err != nil || actual != fixture.expected {
			t.Fatalf("billed(%d, %v) = %d, %v", fixture.raw, fixture.rate, actual, err)
		}
	}
	if _, err := billed(math.MaxInt64, 2); err == nil {
		t.Fatal("overflow was accepted")
	}
}

func TestCompileUserFailsClosed(t *testing.T) {
	if _, err := compileUser(1, "passwd", 0, 0, "not-an-ip", "", ""); err == nil {
		t.Fatal("invalid forbidden IP was accepted")
	}
	if _, err := compileUser(1, "passwd", 0, 0, "", "443-80", ""); err == nil {
		t.Fatal("invalid port range was accepted")
	}
	if _, err := compileUser(1, "passwd", 0, 0, "", "", "not-an-ip"); err == nil {
		t.Fatal("invalid disconnect IP was accepted")
	}
}
