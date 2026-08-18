package database

import (
	"math"
	"strings"
	"testing"

	"github.com/SadNoo/vlesshappy/internal/model"
)

const testPublicKey = "jUOUBrUHcUWzzDhp4L-6l3OwfjUhOajm8Y6yL6jU1zA"

func TestParseVLESSServer(t *testing.T) {
	var node model.Node
	raw := "node.example.com;443;0;tcp;reality;sni=www.example.com|pbk=" + testPublicKey + "|sid=0123456789abcdef|target=www.example.com:443|minver=1.8.0"
	if err := parseVLESSServer(raw, &node); err != nil {
		t.Fatal(err)
	}
	if node.PublicHost != "node.example.com" || node.PublicPort != 443 ||
		node.ServerName != "www.example.com" || node.RealityPublicKey != testPublicKey ||
		node.ShortID != "0123456789abcdef" || node.Target != "www.example.com:443" ||
		node.MinClientVersion != "1.8.0" || node.Flow != "xtls-rprx-vision" ||
		node.Fingerprint != "chrome" || node.Transport != "raw" {
		t.Fatalf("unexpected node: %#v", node)
	}
	node.ID = 15
	node.TrafficRate = 1
	if err := validateNode(node); err != nil {
		t.Fatal(err)
	}
}

func TestParseVLESSServerRejectsInvalidInput(t *testing.T) {
	valid := "node.example.com;443;0;tcp;reality;sni=www.example.com|pbk=" + testPublicKey + "|sid=|target=www.example.com:443"
	for _, raw := range []string{
		"node.example.com;443",
		strings.Replace(valid, ";tcp;", ";ws;", 1),
		valid + "|unknown=value",
		strings.Replace(valid, "|target=", "|sni=duplicate|target=", 1),
		strings.Repeat("a", 256),
	} {
		var node model.Node
		if err := parseVLESSServer(raw, &node); err == nil {
			t.Fatalf("invalid server was accepted: %q", raw)
		}
	}
}

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
