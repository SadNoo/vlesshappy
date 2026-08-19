package caddyservice

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	RealityTarget = "127.0.0.1:9443"
	HTTPPort      = 8080
	BinaryPath    = "/caddy"
)

const defaultPage = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Welcome</title></head>
<body><main><h1>Welcome</h1><p>This site is available.</p></main></body>
</html>
`

type Options struct {
	Binary     string
	Dir        string
	ServerName string
	Stdout     io.Writer
	Stderr     io.Writer
	Ready      func(context.Context, string) error
}

type Process struct {
	cmd      *exec.Cmd
	done     chan struct{}
	errMu    sync.Mutex
	waitErr  error
	stopOnce sync.Once
}

func Start(ctx context.Context, opts Options) (*Process, error) {
	if opts.Binary == "" {
		opts.Binary = BinaryPath
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Ready == nil {
		opts.Ready = waitForCertificate
	}
	configPath, err := Prepare(opts.Dir, opts.ServerName)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(opts.Binary, "run", "--config", configPath, "--adapter", "caddyfile")
	cmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+filepath.Join(opts.Dir, "data"),
		"XDG_CONFIG_HOME="+filepath.Join(opts.Dir, "config"),
	)
	cmd.Stdout, cmd.Stderr = opts.Stdout, opts.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Caddy: %w", err)
	}
	process := &Process{cmd: cmd, done: make(chan struct{})}
	go func() {
		process.errMu.Lock()
		process.waitErr = cmd.Wait()
		process.errMu.Unlock()
		close(process.done)
	}()
	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ready := make(chan error, 1)
	go func() { ready <- opts.Ready(readyCtx, opts.ServerName) }()
	select {
	case <-process.done:
		err := process.Err()
		if err == nil {
			err = errors.New("Caddy exited before its certificate became ready")
		}
		return nil, err
	case err := <-ready:
		if err != nil {
			stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			_ = process.Stop(stopCtx)
			return nil, err
		}
		return process, nil
	case <-readyCtx.Done():
		stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = process.Stop(stopCtx)
		return nil, fmt.Errorf("wait for Caddy certificate: %w", readyCtx.Err())
	}
}

func Prepare(dir, serverName string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", errors.New("caddy_dir must be absolute")
	}
	if err := validateDNSName(serverName); err != nil {
		return "", fmt.Errorf("Caddy server name: %w", err)
	}
	for _, path := range []string{dir, filepath.Join(dir, "data"), filepath.Join(dir, "config"), filepath.Join(dir, "site")} {
		if err := secureDir(path); err != nil {
			return "", err
		}
	}
	indexPath := filepath.Join(dir, "site", "index.html")
	if err := writeOnce(indexPath, []byte(defaultPage)); err != nil {
		return "", err
	}
	configPath := filepath.Join(dir, "Caddyfile")
	content := []byte(render(serverName, filepath.Join(dir, "site")))
	if err := writeAtomic(configPath, content); err != nil {
		return "", err
	}
	return configPath, nil
}

func (p *Process) Done() <-chan struct{} {
	if p == nil {
		return nil
	}
	return p.done
}

func (p *Process) Err() error {
	if p == nil {
		return nil
	}
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.waitErr
}

func (p *Process) Stop(ctx context.Context) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	var signalErr error
	p.stopOnce.Do(func() { signalErr = p.cmd.Process.Signal(syscall.SIGTERM) })
	select {
	case <-p.done:
		err := p.Err()
		if signalErr != nil && !errors.Is(signalErr, os.ErrProcessDone) {
			return errors.Join(signalErr, err)
		}
		return ignoreSignalExit(err)
	case <-ctx.Done():
		killErr := p.cmd.Process.Kill()
		return errors.Join(ctx.Err(), killErr)
	}
}

func render(serverName, siteDir string) string {
	return fmt.Sprintf(`{
	admin off
	auto_https disable_redirects
	http_port %d
	https_port 9443
}

%s {
	bind 127.0.0.1
	root * %s
	file_server
	tls {
		key_type rsa4096
		issuer acme {
			disable_tlsalpn_challenge
		}
	}
}

http://%s:%d {
	bind 0.0.0.0
}
`, HTTPPort, serverName, strconv.Quote(siteDir), serverName, HTTPPort)
}

func waitForCertificate(ctx context.Context, serverName string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{
			MinVersion: tls.VersionTLS13,
			ServerName: serverName,
		}}
		connection, err := dialer.DialContext(ctx, "tcp", RealityTarget)
		if err == nil {
			return connection.Close()
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Caddy did not obtain a valid certificate for %s: %w", serverName, ctx.Err())
		case <-ticker.C:
		}
	}
}

func secureDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("create Caddy directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Caddy directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Caddy path must be a directory and not a symlink")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure Caddy directory: %w", err)
	}
	return nil
}

func writeOnce(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("Caddy site file must be regular and not a symlink")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".Caddyfile-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func validateDNSName(value string) error {
	if value == "" || len(value) > 253 || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return errors.New("invalid hostname length")
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("invalid hostname label")
		}
		for _, char := range label {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') || char == '-') {
				return errors.New("invalid hostname character")
			}
		}
	}
	return nil
}

func ignoreSignalExit(err error) error {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() && status.Signal() == syscall.SIGTERM {
			return nil
		}
	}
	return err
}
