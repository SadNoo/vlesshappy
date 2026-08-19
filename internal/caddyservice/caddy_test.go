package caddyservice

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCreatesStrictManagedConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "caddy data")
	configPath, err := Prepare(dir, "decoy.example.net")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{
		"admin off", "http_port 8080", "https_port 9443", "bind 127.0.0.1",
		"key_type rsa4096", "disable_tlsalpn_challenge", "decoy.example.net", "http://decoy.example.net:8080", "bind 0.0.0.0",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated Caddyfile lacks %q:\n%s", expected, text)
		}
	}
	for _, path := range []string{dir, filepath.Join(dir, "data"), filepath.Join(dir, "config"), filepath.Join(dir, "site")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat directory %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("unexpected directory %s mode: %v", path, info.Mode().Perm())
		}
	}
	for _, path := range []string{configPath, filepath.Join(dir, "site", "index.html")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat file %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("unexpected file %s mode: %v", path, info.Mode().Perm())
		}
	}
}

func TestStartAndStopSupervisedProcess(t *testing.T) {
	binary, err := exec.LookPath("yes")
	if err != nil {
		t.Skip("yes helper is unavailable")
	}
	process, err := Start(context.Background(), Options{
		Binary: binary, Dir: filepath.Join(t.TempDir(), "caddy"), ServerName: "decoy.example.net",
		Stdout: io.Discard, Stderr: io.Discard,
		Ready: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.Done():
	default:
		t.Fatal("Caddy process was not reaped")
	}
}

func TestPreparePreservesCustomSiteAndRejectsUnsafeInput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "caddy")
	if _, err := Prepare(dir, "decoy.example.net"); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "site", "index.html")
	if err := os.WriteFile(indexPath, []byte("custom"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(dir, "next.example.net"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil || string(data) != "custom" {
		t.Fatalf("custom site was replaced: %q, %v", data, err)
	}
	if _, err := Prepare(dir, "bad name.example.net"); err == nil {
		t.Fatal("unsafe server name was accepted")
	}
	unsafeDir := filepath.Join(t.TempDir(), "unsafe")
	if err := os.Symlink(dir, unsafeDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(unsafeDir, "decoy.example.net"); err == nil {
		t.Fatal("symlink Caddy directory was accepted")
	}
}
