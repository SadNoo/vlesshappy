package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStrictConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := fmt.Sprintf(`{
  "node_id": 15,
  "listen": "127.0.0.1:8443",
  "state_dir": %q,
  "reality_private_key_file": %q,
  "database": {
    "address": "127.0.0.1:3306",
    "name": "sspanel",
    "username": "service",
    "password_file": %q,
    "tls_mode": "disabled"
  },
  "intervals": {}
}`, dir, filepath.Join(dir, "reality"), filepath.Join(dir, "database"))
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Intervals.AuthRefreshSeconds != 60 || cfg.OutboxMaxBytes != 1<<30 || cfg.Database.MaxOpen != 4 {
		t.Fatalf("defaults not applied: %#v", cfg)
	}

	for label, replacement := range map[string]string{
		"unknown":   strings.Replace(data, `"intervals": {}`, `"intervals": {}, "extra": true`, 1),
		"duplicate": strings.Replace(data, `"node_id": 15`, `"node_id": 15, "node_id": 16`, 1),
	} {
		if err := os.WriteFile(path, []byte(replacement), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("%s config was accepted", label)
		}
	}
}

func TestReadSecretPermissionsAndSymlink(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("test-only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	actual, err := ReadSecret(secret)
	if err != nil || string(actual) != "test-only" {
		t.Fatalf("ReadSecret = %q, %v", actual, err)
	}
	if err := os.Chmod(secret, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(secret); err == nil {
		t.Fatal("world-readable secret was accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(link); err == nil {
		t.Fatal("secret symlink was accepted")
	}
}
