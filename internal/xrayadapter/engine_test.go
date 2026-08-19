package xrayadapter

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/xtls/xray-core/infra/conf/serial"

	"github.com/SadNoo/vlesshappy/internal/model"
)

func TestBuildConfigCompilesWithPinnedXray(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateText := base64.RawURLEncoding.EncodeToString(privateKey.Bytes())
	publicText := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())
	snapshot := model.Snapshot{
		Node: model.Node{
			ID: 15, Name: "test", TrafficRate: 1, PublicHost: "node.example.com",
			PublicPort: 2053, ServerName: "www.example.com", Target: "127.0.0.1:9443", ManagedCaddy: true,
			RealityPublicKey: publicText, ShortID: "0123456789abcdef", Fingerprint: "chrome",
			Flow: "xtls-rprx-vision", Transport: "raw",
		},
		Users: []model.User{{
			ID: 42, UUID: model.UUIDv3(42, "test-only"),
			ForbiddenIPs: []string{"192.0.2.0/24"}, ForbiddenPorts: []string{"25", "1000-2000"},
			DisconnectIPs: []string{"198.51.100.1"},
		}},
	}
	if err := snapshot.Finalize(); err != nil {
		t.Fatal(err)
	}
	data, err := BuildConfig(snapshot, "127.0.0.1:18443", []byte(privateText))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := serial.LoadJSONConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("pinned Xray rejected generated config: %v\n%s", err, data)
	}
	text := string(data)
	for _, required := range []string{"vless", "reality", "xtls-rprx-vision", "AsIs", "statsUserOnline"} {
		if !strings.Contains(text, required) {
			t.Fatalf("generated config lacks %q", required)
		}
	}
	if strings.Contains(text, "test-only") {
		t.Fatal("panel password leaked into Xray config")
	}

	otherKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherText := base64.RawURLEncoding.EncodeToString(otherKey.Bytes())
	if _, err := BuildConfig(snapshot, "127.0.0.1:18443", []byte(otherText)); err == nil {
		t.Fatal("mismatched REALITY key was accepted")
	}
}
