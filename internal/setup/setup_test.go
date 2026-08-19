package setup

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/model"
)

func TestRunCreatesValidatedRuntimeWithoutLeakingSecrets(t *testing.T) {
	dataDir := t.TempDir()
	seed := bytes.Repeat([]byte{0x42}, 80)
	input := strings.NewReader(strings.Join([]string{
		"node.example.com", "2053", "decoy.example.net", "",
		"db.example.com:3306", "", "vlesshappy", "disabled", "123", "",
	}, "\n"))
	var output bytes.Buffer
	var privateKey, publicKey, shortID string
	passwordCalls := 0
	validatorCalls := 0
	err := Run(context.Background(), Options{
		DataDir: dataDir, VolumeName: "test-volume", ContainerName: "test-node",
		In: input, Out: &output, Random: bytes.NewReader(seed),
		ReadPassword: func() ([]byte, error) {
			passwordCalls++
			return []byte("database-test-secret"), nil
		},
		Validate: func(_ context.Context, cfg config.Config) (model.Snapshot, error) {
			validatorCalls++
			if cfg.NodeID != 123 || !strings.Contains(cfg.StateDir, string(filepath.Separator)+".setup"+string(filepath.Separator)) {
				t.Fatalf("unexpected staged config: %#v", cfg)
			}
			password, err := config.ReadSecret(cfg.Database.PasswordFile)
			if err != nil || string(password) != "database-test-secret" {
				t.Fatalf("unexpected staged password: %q, %v", password, err)
			}
			private, err := config.ReadSecret(cfg.RealityPrivateKeyFile)
			if err != nil {
				t.Fatal(err)
			}
			privateKey = string(private)
			decoded, err := base64.RawURLEncoding.DecodeString(privateKey)
			if err != nil {
				t.Fatal(err)
			}
			key, err := ecdh.X25519().NewPrivateKey(decoded)
			if err != nil {
				t.Fatal(err)
			}
			publicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
			shortID = outputValue(t, output.String(), "REALITY short ID: ")
			return model.Snapshot{Node: model.Node{
				PublicHost: "node.example.com", PublicPort: 2053,
				ServerName: "decoy.example.net", Target: "decoy.example.net:443",
				RealityPublicKey: publicKey, ShortID: shortID,
			}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if passwordCalls != 2 || validatorCalls != 1 {
		t.Fatalf("password calls=%d validator calls=%d", passwordCalls, validatorCalls)
	}
	runtimeDir := filepath.Join(dataDir, "runtime")
	cfg, err := config.Load(filepath.Join(runtimeDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NodeID != 123 || cfg.StateDir != filepath.Join(runtimeDir, "state") ||
		cfg.Database.PasswordFile != filepath.Join(runtimeDir, "secrets", "vlesshappy_database_password") {
		t.Fatalf("unexpected final config: %#v", cfg)
	}
	for _, path := range []string{
		filepath.Join(runtimeDir, "config.json"),
		filepath.Join(runtimeDir, "secrets", "vlesshappy_database_password"),
		filepath.Join(runtimeDir, "secrets", "vlesshappy_reality_private_key"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode is %o", path, info.Mode().Perm())
		}
	}
	text := output.String()
	if strings.Contains(text, "database-test-secret") || strings.Contains(text, privateKey) {
		t.Fatalf("secret leaked in output: %s", text)
	}
	for _, public := range []string{publicKey, shortID, "-p 2053:8443/tcp", "-v test-volume:/data"} {
		if !strings.Contains(text, public) {
			t.Fatalf("missing public output %q in %s", public, text)
		}
	}

	err = Run(context.Background(), Options{DataDir: dataDir, In: strings.NewReader("")})
	if err == nil || !strings.Contains(err.Error(), "already initialized") {
		t.Fatalf("second setup error = %v", err)
	}
}

func outputValue(t *testing.T, output, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("missing output prefix %q in %s", prefix, output)
	return ""
}

func TestRunDoesNotCommitFailedValidation(t *testing.T) {
	dataDir := t.TempDir()
	input := strings.NewReader(strings.Join([]string{
		"node.example.com", "443", "decoy.example.net", "",
		"db.example.com:3306", "sspanel", "vlesshappy", "required", "15", "cancel", "",
	}, "\n"))
	err := Run(context.Background(), Options{
		DataDir: dataDir, In: input, Out: &bytes.Buffer{}, Random: bytes.NewReader(bytes.Repeat([]byte{0x24}, 40)),
		ReadPassword: func() ([]byte, error) { return []byte("test-secret"), nil },
		Validate: func(context.Context, config.Config) (model.Snapshot, error) {
			return model.Snapshot{}, errors.New("database unavailable")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range []string{"runtime", ".setup"} {
		if _, err := os.Lstat(filepath.Join(dataDir, name)); !os.IsNotExist(err) {
			t.Fatalf("failed setup left %s behind: %v", name, err)
		}
	}
}

func TestMatchPanelRejectsCopyError(t *testing.T) {
	answer := answers{
		publicHost: "node.example.com", publicPort: 443, serverName: "decoy.example.net",
		target: "decoy.example.net:443", publicKey: "public", shortID: "0123456789abcdef",
	}
	node := model.Node{
		PublicHost: answer.publicHost, PublicPort: answer.publicPort, ServerName: answer.serverName,
		Target: answer.target, RealityPublicKey: "different", ShortID: answer.shortID,
	}
	if err := matchPanel(node, answer); err == nil || !strings.Contains(err.Error(), "public key") {
		t.Fatalf("unexpected mismatch result: %v", err)
	}
}

func TestValidateUntilReadyRetriesPanelMismatch(t *testing.T) {
	answer := answers{
		publicHost: "node.example.com", publicPort: 443, serverName: "decoy.example.net",
		target: "decoy.example.net:443", publicKey: "public", shortID: "0123456789abcdef",
	}
	valid := model.Node{
		PublicHost: answer.publicHost, PublicPort: answer.publicPort, ServerName: answer.serverName,
		Target: answer.target, RealityPublicKey: answer.publicKey, ShortID: answer.shortID,
	}
	calls := 0
	var output bytes.Buffer
	err := validateUntilReady(context.Background(), prompt{in: strings.NewReader("\n"), out: &output},
		func(context.Context, config.Config) (model.Snapshot, error) {
			calls++
			node := valid
			if calls == 1 {
				node.ShortID = "incorrect"
			}
			return model.Snapshot{Node: node}, nil
		}, config.Config{}, answer)
	if err != nil || calls != 2 {
		t.Fatalf("retry result: calls=%d err=%v", calls, err)
	}
	if !strings.Contains(output.String(), "panel REALITY short ID does not match") {
		t.Fatal(output.String())
	}
}

func TestPrintRunCommandAddsHostGatewayOnlyWhenNeeded(t *testing.T) {
	var output bytes.Buffer
	printRunCommand(&output, "vle-data", "vle-node", 8443, "host.docker.internal:3306")
	if !strings.Contains(output.String(), "--add-host host.docker.internal:host-gateway") {
		t.Fatal(output.String())
	}
	output.Reset()
	printRunCommand(&output, "vle-data", "vle-node", 8443, "db.example.com:3306")
	if strings.Contains(output.String(), "--add-host") {
		t.Fatal(output.String())
	}
}

func TestPEMPromptAcceptsCertificateAuthority(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute),
		NotAfter: time.Now().Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	input := append(append([]byte{}, certificate...), []byte(".\n")...)
	actual, err := (prompt{in: bytes.NewReader(input), out: &bytes.Buffer{}}).pem()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, certificate) {
		t.Fatalf("unexpected PEM result")
	}
}
