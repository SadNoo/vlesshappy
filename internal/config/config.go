package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultAuthRefresh = 60
	defaultReport      = 60
	defaultAuthStale   = 3600
	defaultShutdown    = 120
	defaultTCPUser     = 800
	defaultTCPGlobal   = 3000
	defaultHandshakes  = 1024
	defaultXUDPUser    = 128
	defaultXUDPGlobal  = 2048
	defaultDNSSeconds  = 10
)

type Config struct {
	NodeID                int64          `json:"node_id"`
	Listen                string         `json:"listen"`
	StateDir              string         `json:"state_dir"`
	CaddyDir              string         `json:"caddy_dir"`
	RealityPrivateKeyFile string         `json:"reality_private_key_file"`
	Database              DatabaseConfig `json:"database"`
	Intervals             Intervals      `json:"intervals"`
	Limits                Limits         `json:"limits"`
	LogLevel              string         `json:"log_level"`
}

type DatabaseConfig struct {
	Address      string `json:"address"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	PasswordFile string `json:"password_file"`
	TLSMode      string `json:"tls_mode"`
	CAFile       string `json:"ca_file"`
	ServerName   string `json:"server_name"`
	MaxOpen      int    `json:"max_open"`
	MaxIdle      int    `json:"max_idle"`
}

type Intervals struct {
	AuthRefreshSeconds int `json:"auth_refresh_seconds"`
	ReportSeconds      int `json:"report_seconds"`
	AuthStaleSeconds   int `json:"auth_stale_seconds"`
	ShutdownSeconds    int `json:"shutdown_seconds"`
}

type Limits struct {
	TCPPerUser               int `json:"tcp_per_user"`
	TCPGlobal                int `json:"tcp_global"`
	ConcurrentHandshakes     int `json:"concurrent_handshakes"`
	XUDPPerUser              int `json:"xudp_per_user"`
	XUDPGlobal               int `json:"xudp_global"`
	DNSResolveTimeoutSeconds int `json:"dns_resolve_timeout_seconds"`
}

func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return cfg, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	if err := ensureEOF(dec); err != nil {
		return cfg, err
	}
	cfg.defaults()
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var value func() error
	value = func() error {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("config object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate config key %q", key)
				}
				seen[key] = struct{}{}
				if err := value(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := value(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		default:
			return errors.New("unexpected config delimiter")
		}
	}
	if err := value(); err != nil {
		return fmt.Errorf("inspect config: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("config contains multiple JSON values")
		}
		return err
	}
	return nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("config contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing config: %w", err)
	}
	return nil
}

func (c *Config) defaults() {
	if c.Intervals.AuthRefreshSeconds == 0 {
		c.Intervals.AuthRefreshSeconds = defaultAuthRefresh
	}
	if c.Intervals.ReportSeconds == 0 {
		c.Intervals.ReportSeconds = defaultReport
	}
	if c.Intervals.AuthStaleSeconds == 0 {
		c.Intervals.AuthStaleSeconds = defaultAuthStale
	}
	if c.Intervals.ShutdownSeconds == 0 {
		c.Intervals.ShutdownSeconds = defaultShutdown
	}
	if c.Limits.TCPPerUser == 0 {
		c.Limits.TCPPerUser = defaultTCPUser
	}
	if c.Limits.TCPGlobal == 0 {
		c.Limits.TCPGlobal = defaultTCPGlobal
	}
	if c.Limits.ConcurrentHandshakes == 0 {
		c.Limits.ConcurrentHandshakes = defaultHandshakes
	}
	if c.Limits.XUDPPerUser == 0 {
		c.Limits.XUDPPerUser = defaultXUDPUser
	}
	if c.Limits.XUDPGlobal == 0 {
		c.Limits.XUDPGlobal = defaultXUDPGlobal
	}
	if c.Limits.DNSResolveTimeoutSeconds == 0 {
		c.Limits.DNSResolveTimeoutSeconds = defaultDNSSeconds
	}
	if c.Database.MaxOpen == 0 {
		c.Database.MaxOpen = 4
	}
	if c.Database.MaxIdle == 0 {
		c.Database.MaxIdle = 2
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
}

func (c Config) Validate() error {
	if c.NodeID <= 0 {
		return errors.New("node_id must be positive")
	}
	if err := validateListen(c.Listen); err != nil {
		return err
	}
	if err := absolutePath("state_dir", c.StateDir); err != nil {
		return err
	}
	if c.CaddyDir != "" {
		if err := absolutePath("caddy_dir", c.CaddyDir); err != nil {
			return err
		}
	}
	if err := absolutePath("reality_private_key_file", c.RealityPrivateKeyFile); err != nil {
		return err
	}
	if err := absolutePath("database.password_file", c.Database.PasswordFile); err != nil {
		return err
	}
	if c.Database.Address == "" || c.Database.Name == "" || c.Database.Username == "" {
		return errors.New("database address, name and username are required")
	}
	if strings.ContainsAny(c.Database.Name, "`\\/\x00") {
		return errors.New("database.name contains forbidden characters")
	}
	if strings.ContainsRune(c.Database.Username, '\x00') {
		return errors.New("database.username contains NUL")
	}
	switch c.Database.TLSMode {
	case "disabled", "required":
		if c.Database.CAFile != "" || c.Database.ServerName != "" {
			return errors.New("database ca_file/server_name are only valid with tls_mode verify_ca")
		}
	case "verify_ca":
		if err := absolutePath("database.ca_file", c.Database.CAFile); err != nil {
			return err
		}
		if c.Database.ServerName == "" {
			return errors.New("database.server_name is required with tls_mode verify_ca")
		}
	default:
		return errors.New("database.tls_mode must be disabled, required or verify_ca")
	}
	if c.Database.MaxOpen < 1 || c.Database.MaxOpen > 64 || c.Database.MaxIdle < 0 || c.Database.MaxIdle > c.Database.MaxOpen {
		return errors.New("database pool sizes are invalid")
	}
	if c.Intervals.AuthRefreshSeconds < 5 || c.Intervals.ReportSeconds < 5 {
		return errors.New("refresh and report intervals must be at least 5 seconds")
	}
	if c.Intervals.AuthStaleSeconds < c.Intervals.AuthRefreshSeconds || c.Intervals.ShutdownSeconds < 1 {
		return errors.New("auth_stale_seconds or shutdown_seconds is invalid")
	}
	if c.Limits.TCPPerUser < 1 || c.Limits.TCPGlobal < c.Limits.TCPPerUser ||
		c.Limits.ConcurrentHandshakes < 1 || c.Limits.XUDPPerUser < 1 ||
		c.Limits.XUDPGlobal < c.Limits.XUDPPerUser || c.Limits.DNSResolveTimeoutSeconds < 1 ||
		c.Limits.TCPGlobal > 1_000_000 || c.Limits.ConcurrentHandshakes > 1_000_000 ||
		c.Limits.XUDPGlobal > 1_000_000 || c.Limits.DNSResolveTimeoutSeconds > 60 {
		return errors.New("session limits are invalid")
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return errors.New("log_level must be debug, info, warn or error")
	}
	return nil
}

func validateListen(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("listen must be host:port: %w", err)
	}
	if host != "" && net.ParseIP(host) == nil {
		return errors.New("listen host must be an IP address")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("listen port must be between 1 and 65535")
	}
	return nil
}

func absolutePath(name, value string) error {
	if value == "" || !filepath.IsAbs(value) {
		return fmt.Errorf("%s must be an absolute path", name)
	}
	return nil
}

func ReadSecret(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect secret file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("secret path must be a regular file and not a symlink")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("secret file must not be accessible by group or others")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Geteuid() {
		return nil, errors.New("secret file must be owned by the running UID")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("secret file is empty")
	}
	return data, nil
}

func (c Config) AuthRefresh() time.Duration {
	return time.Duration(c.Intervals.AuthRefreshSeconds) * time.Second
}
func (c Config) ReportEvery() time.Duration {
	return time.Duration(c.Intervals.ReportSeconds) * time.Second
}
func (c Config) AuthStale() time.Duration {
	return time.Duration(c.Intervals.AuthStaleSeconds) * time.Second
}
func (c Config) Shutdown() time.Duration {
	return time.Duration(c.Intervals.ShutdownSeconds) * time.Second
}
func (c Config) DNSResolveTimeout() time.Duration {
	return time.Duration(c.Limits.DNSResolveTimeoutSeconds) * time.Second
}
