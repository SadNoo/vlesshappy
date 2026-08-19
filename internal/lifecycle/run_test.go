package lifecycle

import (
	"strings"
	"testing"

	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/model"
)

func TestValidateCaddyContract(t *testing.T) {
	managed := model.Node{ManagedCaddy: true}
	legacy := model.Node{}
	if err := validateCaddyContract(config.Config{CaddyDir: "/data/caddy"}, managed); err != nil {
		t.Fatal(err)
	}
	if err := validateCaddyContract(config.Config{}, legacy); err != nil {
		t.Fatal(err)
	}
	if err := validateCaddyContract(config.Config{}, managed); err == nil || !strings.Contains(err.Error(), "requires caddy_dir") {
		t.Fatalf("unexpected managed mismatch: %v", err)
	}
	if err := validateCaddyContract(config.Config{CaddyDir: "/data/caddy"}, legacy); err == nil || !strings.Contains(err.Error(), "without target") {
		t.Fatalf("unexpected legacy mismatch: %v", err)
	}
}
