package database

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/SadNoo/vlesshappy/internal/accounting"
	"github.com/SadNoo/vlesshappy/internal/caddyservice"
	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/model"
	"github.com/SadNoo/vlesshappy/internal/policy"
)

type DB struct {
	sql *sql.DB
}

var ErrNodeUnavailable = errors.New("VLESS REALITY node is missing, disabled, mis-typed or bandwidth-exhausted")

func Open(cfg config.DatabaseConfig, password []byte) (*DB, error) {
	tlsName, err := configureTLS(cfg)
	if err != nil {
		return nil, err
	}
	driverConfig := gomysql.NewConfig()
	driverConfig.Net = "tcp"
	driverConfig.Addr = cfg.Address
	driverConfig.User = cfg.Username
	driverConfig.Passwd = string(password)
	driverConfig.DBName = cfg.Name
	driverConfig.ParseTime = true
	driverConfig.Timeout = 15 * time.Second
	driverConfig.ReadTimeout = 15 * time.Second
	driverConfig.WriteTimeout = 15 * time.Second
	driverConfig.Collation = "utf8mb4_unicode_ci"
	driverConfig.TLSConfig = tlsName
	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpen)
	db.SetMaxIdleConns(cfg.MaxIdle)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &DB{sql: db}, nil
}

func configureTLS(cfg config.DatabaseConfig) (string, error) {
	switch cfg.TLSMode {
	case "disabled":
		return "false", nil
	case "required":
		return "true", nil
	case "verify_ca":
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return "", fmt.Errorf("read database CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return "", errors.New("database CA contains no certificates")
		}
		nameHash := sha256.Sum256([]byte(cfg.CAFile + "\x00" + cfg.ServerName))
		name := "vlesshappy-" + hex.EncodeToString(nameHash[:8])
		err = gomysql.RegisterTLSConfig(name, &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    pool,
			ServerName: cfg.ServerName,
		})
		if err != nil && !strings.Contains(err.Error(), "already registered") {
			return "", fmt.Errorf("register database TLS: %w", err)
		}
		return name, nil
	default:
		return "", errors.New("unsupported database TLS mode")
	}
}

func (d *DB) Ping(ctx context.Context) error { return d.sql.PingContext(ctx) }
func (d *DB) Close() error                   { return d.sql.Close() }

func (d *DB) LoadSnapshot(ctx context.Context, nodeID int64) (model.Snapshot, []string, error) {
	var snapshot model.Snapshot
	var server string
	row := d.sql.QueryRowContext(ctx, `
SELECT n.id, n.name, n.traffic_rate, n.node_class, n.node_group,
       n.node_speedlimit, n.node_connector, n.node_bandwidth, n.node_bandwidth_limit,
       n.server
FROM ss_node AS n
WHERE n.id = ? AND n.sort = 15 AND n.type = 1
  AND (n.node_bandwidth_limit = 0 OR n.node_bandwidth < n.node_bandwidth_limit)`, nodeID)
	if err := row.Scan(
		&snapshot.Node.ID, &snapshot.Node.Name, &snapshot.Node.TrafficRate,
		&snapshot.Node.Class, &snapshot.Node.Group, &snapshot.Node.SpeedLimitMbps,
		&snapshot.Node.ConnectorLimit, &snapshot.Node.Bandwidth, &snapshot.Node.BandwidthLimit,
		&server,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return snapshot, nil, ErrNodeUnavailable
		}
		return snapshot, nil, fmt.Errorf("load node: %w", err)
	}
	if err := parseVLESSServer(server, &snapshot.Node); err != nil {
		return snapshot, nil, fmt.Errorf("invalid ss_node.server: %w", err)
	}
	if err := validateNode(snapshot.Node); err != nil {
		return snapshot, nil, err
	}

	rows, err := d.sql.QueryContext(ctx, `
SELECT id, passwd, node_speedlimit, node_connector,
       COALESCE(forbidden_ip, ''), COALESCE(forbidden_port, ''), COALESCE(disconnect_ip, '')
FROM user
WHERE enable = 1 AND expire_in > NOW()
  AND CAST(transfer_enable AS DECIMAL(65, 0))
      > CAST(u AS DECIMAL(65, 0)) + CAST(d AS DECIMAL(65, 0))
  AND (is_admin = 1 OR (class >= ? AND (? = 0 OR node_group = ?)))
ORDER BY id`, snapshot.Node.Class, snapshot.Node.Group, snapshot.Node.Group)
	if err != nil {
		return snapshot, nil, fmt.Errorf("load users: %w", err)
	}
	defer rows.Close()
	warnings := make([]string, 0)
	for rows.Next() {
		var id int64
		var password, forbiddenIP, forbiddenPort, disconnectIP string
		var speed float64
		var connectors int
		if err := rows.Scan(&id, &password, &speed, &connectors, &forbiddenIP, &forbiddenPort, &disconnectIP); err != nil {
			return snapshot, warnings, fmt.Errorf("scan user: %w", err)
		}
		user, err := compileUser(id, password, speed, connectors, forbiddenIP, forbiddenPort, disconnectIP)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("user_id=%d isolated: %v", id, err))
			continue
		}
		snapshot.Users = append(snapshot.Users, user)
	}
	if err := rows.Err(); err != nil {
		return snapshot, warnings, fmt.Errorf("read users: %w", err)
	}
	snapshot.LoadedAt = time.Now()
	if err := snapshot.Finalize(); err != nil {
		return snapshot, warnings, err
	}
	return snapshot, warnings, nil
}

func compileUser(id int64, password string, speed float64, connectors int, forbiddenIP, forbiddenPort, disconnectIP string) (model.User, error) {
	if id <= 0 || password == "" || math.IsNaN(speed) || math.IsInf(speed, 0) || speed < 0 || speed > 1_000_000 || connectors < 0 || connectors > 1_000_000 {
		return model.User{}, errors.New("invalid identity or limits")
	}
	blocked, err := policy.ParseIPRules(forbiddenIP)
	if err != nil {
		return model.User{}, err
	}
	ports, err := policy.ParsePorts(forbiddenPort)
	if err != nil {
		return model.User{}, err
	}
	disconnected, err := policy.ParseSourceIPs(disconnectIP)
	if err != nil {
		return model.User{}, err
	}
	return model.User{
		ID: id, UUID: model.UUIDv3(id, password), SpeedLimitMbps: speed,
		ConnectorLimit: connectors, ForbiddenIPs: blocked,
		ForbiddenPorts: ports, DisconnectIPs: disconnected,
	}, nil
}

func validateNode(node model.Node) error {
	if node.ID <= 0 || node.PublicPort < 1 || node.PublicPort > 65535 {
		return errors.New("invalid node ID or public port")
	}
	if math.IsNaN(node.TrafficRate) || math.IsInf(node.TrafficRate, 0) || node.TrafficRate <= 0 || node.TrafficRate > 10000 {
		return errors.New("invalid node traffic rate")
	}
	if node.Fingerprint != "chrome" || node.Flow != "xtls-rprx-vision" || node.Transport != "raw" {
		return errors.New("node must use chrome, xtls-rprx-vision and raw")
	}
	if err := validateHost(node.PublicHost); err != nil {
		return fmt.Errorf("invalid public_host: %w", err)
	}
	if err := validateDNSName(node.ServerName); err != nil {
		return fmt.Errorf("invalid server_name: %w", err)
	}
	host, portText, err := net.SplitHostPort(node.Target)
	if err != nil || validateHost(host) != nil {
		return errors.New("target must be a valid host:port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("target port is invalid")
	}
	key, err := decodeBase64URL32(node.RealityPublicKey)
	if err != nil || len(key) != 32 {
		return errors.New("reality public key must be unpadded base64url for 32 bytes")
	}
	if len(node.ShortID) > 16 || len(node.ShortID)%2 != 0 {
		return errors.New("short_id must contain at most 16 even hex characters")
	}
	if _, err := hex.DecodeString(node.ShortID); err != nil {
		return errors.New("short_id must be hexadecimal")
	}
	if len(node.MinClientVersion) > 32 {
		return errors.New("minver is too long")
	}
	for _, char := range node.MinClientVersion {
		if (char < '0' || char > '9') && char != '.' {
			return errors.New("minver must contain only digits and dots")
		}
	}
	return nil
}

func parseVLESSServer(server string, node *model.Node) error {
	if len(server) == 0 || len(server) > 255 {
		return errors.New("configuration must contain 1 to 255 bytes")
	}
	parts := strings.Split(server, ";")
	if len(parts) != 6 || strings.TrimSpace(parts[2]) != "0" ||
		strings.ToLower(strings.TrimSpace(parts[3])) != "tcp" ||
		strings.ToLower(strings.TrimSpace(parts[4])) != "reality" {
		return errors.New("expected host;port;0;tcp;reality;options")
	}
	port, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || port < 1 || port > 65535 {
		return errors.New("public port must be between 1 and 65535")
	}
	options := make(map[string]string)
	for _, raw := range strings.Split(parts[5], "|") {
		pair := strings.SplitN(raw, "=", 2)
		if len(pair) != 2 {
			return errors.New("invalid option")
		}
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		if key != "sni" && key != "pbk" && key != "sid" && key != "target" && key != "minver" {
			return fmt.Errorf("unknown option %q", key)
		}
		if _, exists := options[key]; exists {
			return fmt.Errorf("duplicate option %q", key)
		}
		options[key] = strings.TrimSpace(pair[1])
	}
	for _, required := range []string{"sni", "pbk", "sid"} {
		if _, exists := options[required]; !exists {
			return fmt.Errorf("missing option %q", required)
		}
	}
	node.PublicHost = strings.TrimSpace(parts[0])
	node.PublicPort = port
	node.ServerName = options["sni"]
	node.RealityPublicKey = options["pbk"]
	node.ShortID = strings.ToLower(options["sid"])
	node.Target, node.ManagedCaddy = options["target"], false
	if _, exists := options["target"]; !exists {
		node.Target, node.ManagedCaddy = caddyservice.RealityTarget, true
	} else if node.Target == "" {
		return errors.New("target must not be empty when present")
	}
	node.MinClientVersion = options["minver"]
	node.Fingerprint = "chrome"
	node.Flow = "xtls-rprx-vision"
	node.Transport = "raw"
	return nil
}

func decodeBase64URL32(value string) ([]byte, error) {
	if strings.Contains(value, "=") {
		return nil, errors.New("padding is not allowed")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("invalid base64url X25519 key")
	}
	return decoded, nil
}

func validateHost(value string) error {
	value = strings.Trim(value, "[]")
	if address, err := netip.ParseAddr(value); err == nil && address.IsValid() {
		return nil
	}
	return validateDNSName(value)
}

func validateDNSName(value string) error {
	if value == "" || len(value) > 253 || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return errors.New("invalid hostname length")
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("invalid hostname label")
		}
		for _, r := range label {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
				return errors.New("invalid hostname character")
			}
		}
	}
	return nil
}

func (d *DB) ApplyTraffic(ctx context.Context, nodeID int64, trafficRate float64, entries []accounting.Entry) error {
	if nodeID <= 0 || math.IsNaN(trafficRate) || math.IsInf(trafficRate, 0) || trafficRate <= 0 || trafficRate > 10000 {
		return errors.New("invalid traffic report metadata")
	}
	if len(entries) == 0 {
		return nil
	}
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var rawTotal int64
	for _, entry := range entries {
		if entry.UserID <= 0 || entry.Uplink < 0 || entry.Downlink < 0 ||
			entry.Uplink > math.MaxInt64-entry.Downlink || rawTotal > math.MaxInt64-entry.Uplink-entry.Downlink {
			return errors.New("invalid or overflowing traffic entry")
		}
		billedUp, err := billed(entry.Uplink, trafficRate)
		if err != nil {
			return err
		}
		billedDown, err := billed(entry.Downlink, trafficRate)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE user SET u = u + ?, d = d + ?, t = UNIX_TIMESTAMP() WHERE id = ?`, billedUp, billedDown, entry.UserID)
		if err != nil {
			return fmt.Errorf("update user traffic: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated > 0 {
			trafficText := humanBytes(billedUp + billedDown)
			_, err = tx.ExecContext(ctx, `
INSERT INTO user_traffic_log (user_id, u, d, node_id, rate, traffic, log_time)
VALUES (?, ?, ?, ?, ?, ?, UNIX_TIMESTAMP())`, entry.UserID, entry.Uplink, entry.Downlink, nodeID, trafficRate, trafficText)
			if err != nil {
				return fmt.Errorf("insert user traffic log: %w", err)
			}
		}
		rawTotal += entry.Uplink + entry.Downlink
	}
	result, err := tx.ExecContext(ctx, `UPDATE ss_node SET node_bandwidth = node_bandwidth + ?, node_heartbeat = UNIX_TIMESTAMP() WHERE id = ?`, rawTotal, nodeID)
	if err != nil {
		return fmt.Errorf("update node bandwidth: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		if err := requireNode(ctx, tx, nodeID, "applying traffic"); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit traffic report: %w", err)
	}
	return nil
}

func billed(raw int64, rate float64) (int64, error) {
	value := float64(raw) * rate
	if math.IsNaN(value) || math.IsInf(value, 0) || value > math.MaxInt64 {
		return 0, errors.New("billed traffic overflows int64")
	}
	return int64(math.Round(value)), nil
}

func humanBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return strconv.FormatInt(value, 10) + " B"
	}
	divisor, exponent := int64(unit), 0
	for n := value / unit; n >= unit && exponent < 5; n /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.2f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}

func (d *DB) ReportTelemetry(ctx context.Context, nodeID int64, online map[int64][]string, uptime time.Duration, load string) error {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	onlineUsers := 0
	for userID, ips := range online {
		if len(ips) > 0 {
			onlineUsers++
		}
		for _, ip := range ips {
			if _, err := tx.ExecContext(ctx, `INSERT INTO alive_ip (nodeid, userid, ip, datetime) VALUES (?, ?, ?, UNIX_TIMESTAMP())`, nodeID, userID, ip); err != nil {
				return fmt.Errorf("insert alive IP: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ss_node_online_log (node_id, online_user, log_time) VALUES (?, ?, UNIX_TIMESTAMP())`, nodeID, onlineUsers); err != nil {
		return fmt.Errorf("insert online log: %w", err)
	}
	if len(load) > 32 {
		load = load[:32]
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ss_node_info (node_id, uptime, `load`, log_time) VALUES (?, ?, ?, UNIX_TIMESTAMP())", nodeID, uptime.Seconds(), load); err != nil {
		return fmt.Errorf("insert node info: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE ss_node SET node_heartbeat = UNIX_TIMESTAMP() WHERE id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		if err := requireNode(ctx, tx, nodeID, "reporting telemetry"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func requireNode(ctx context.Context, tx *sql.Tx, nodeID int64, operation string) error {
	var existingID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM ss_node WHERE id = ?`, nodeID).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("node disappeared while %s", operation)
	}
	if err != nil {
		return fmt.Errorf("verify node while %s: %w", operation, err)
	}
	return nil
}
