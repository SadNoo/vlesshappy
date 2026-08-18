package integration_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/SadNoo/vlesshappy/internal/accounting"
	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/database"
)

func TestMySQLAuthorizationAccountingAndTelemetry(t *testing.T) {
	address := os.Getenv("VLESSHAPPY_TEST_MYSQL_ADDRESS")
	passwordFile := os.Getenv("VLESSHAPPY_TEST_MYSQL_PASSWORD_FILE")
	if address == "" || passwordFile == "" {
		t.Skip("set VLESSHAPPY_TEST_MYSQL_ADDRESS and VLESSHAPPY_TEST_MYSQL_PASSWORD_FILE")
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal(err)
	}
	password = []byte(stringTrimSpace(password))
	const databaseName = "vlesshappy_test"

	driverConfig := gomysql.NewConfig()
	driverConfig.Net = "tcp"
	driverConfig.Addr = address
	driverConfig.User = "root"
	driverConfig.Passwd = string(password)
	driverConfig.DBName = databaseName
	driverConfig.ParseTime = true
	bootstrap, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := createSchema(ctx, bootstrap); err != nil {
		t.Fatal(err)
	}

	publicKey := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	statements := []string{
		`INSERT INTO ss_node (id,name,traffic_rate,node_class,node_group,node_speedlimit,node_connector,node_bandwidth,node_bandwidth_limit,type,sort,method,info,status,node_heartbeat) VALUES (15,'VLESS test',1,0,0,0,0,0,0,1,15,'vless','','ok',0)`,
		`INSERT INTO vless_reality_node_config VALUES (15,'node.example.com',443,'www.example.com','www.example.com:443',?,'0123456789abcdef','chrome','xtls-rprx-vision','raw','',1,NOW())`,
		`INSERT INTO user (id,passwd,u,d,transfer_enable,enable,expire_in,class,node_group,is_admin,node_speedlimit,node_connector,forbidden_ip,forbidden_port,disconnect_ip,t) VALUES (42,'passwd',0,0,1000000,1,DATE_ADD(NOW(), INTERVAL 1 DAY),0,0,0,0,0,'','','',0)`,
	}
	for i, statement := range statements {
		var args []any
		if i == 1 {
			args = []any{publicKey}
		}
		if _, err := bootstrap.ExecContext(ctx, statement, args...); err != nil {
			t.Fatal(err)
		}
	}

	serviceDB, err := database.Open(config.DatabaseConfig{
		Address: address, Name: databaseName, Username: "root", TLSMode: "disabled", MaxOpen: 4, MaxIdle: 2,
	}, password)
	if err != nil {
		t.Fatal(err)
	}
	defer serviceDB.Close()
	snapshot, warnings, err := serviceDB.LoadSnapshot(ctx, 15)
	if err != nil || len(warnings) != 0 || len(snapshot.Users) != 1 {
		t.Fatalf("snapshot users=%d warnings=%v err=%v", len(snapshot.Users), warnings, err)
	}
	batch, err := accounting.NewBatch(15, 1, 1.5, []accounting.Entry{{UserID: 42, Uplink: 10, Downlink: 20}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceDB.ApplyBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err := serviceDB.ApplyBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	var upload, download, nodeBytes int64
	var logs, batches int
	if err := bootstrap.QueryRowContext(ctx, `SELECT u,d FROM user WHERE id=42`).Scan(&upload, &download); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.QueryRowContext(ctx, `SELECT node_bandwidth FROM ss_node WHERE id=15`).Scan(&nodeBytes); err != nil {
		t.Fatal(err)
	}
	_ = bootstrap.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_traffic_log`).Scan(&logs)
	_ = bootstrap.QueryRowContext(ctx, `SELECT COUNT(*) FROM vless_traffic_batches`).Scan(&batches)
	if upload != 15 || download != 30 || nodeBytes != 30 || logs != 1 || batches != 1 {
		t.Fatalf("idempotency failed: u=%d d=%d node=%d logs=%d batches=%d", upload, download, nodeBytes, logs, batches)
	}
	if err := serviceDB.ReportTelemetry(ctx, 15, map[int64][]string{42: {"192.0.2.1"}}, time.Minute, "0.10 0.20 0.30"); err != nil {
		t.Fatal(err)
	}
	var alive, online, info int
	_ = bootstrap.QueryRowContext(ctx, `SELECT COUNT(*) FROM alive_ip`).Scan(&alive)
	_ = bootstrap.QueryRowContext(ctx, `SELECT COUNT(*) FROM ss_node_online_log`).Scan(&online)
	_ = bootstrap.QueryRowContext(ctx, `SELECT COUNT(*) FROM ss_node_info`).Scan(&info)
	if alive != 1 || online != 1 || info != 1 {
		t.Fatalf("telemetry rows: alive=%d online=%d info=%d", alive, online, info)
	}
}

func createSchema(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`DROP TABLE IF EXISTS alive_ip, ss_node_info, ss_node_online_log, user_traffic_log, vless_traffic_batches, vless_reality_node_config, user, ss_node`,
		`CREATE TABLE ss_node (id INT PRIMARY KEY,name VARCHAR(128),traffic_rate DOUBLE,node_class INT,node_group INT,node_speedlimit DOUBLE,node_connector INT,node_bandwidth BIGINT,node_bandwidth_limit BIGINT,type INT,sort INT,method VARCHAR(64),info VARCHAR(128),status VARCHAR(128),node_heartbeat BIGINT DEFAULT 0)`,
		`CREATE TABLE vless_reality_node_config (node_id INT PRIMARY KEY,public_host VARCHAR(253),public_port INT,server_name VARCHAR(253),target VARCHAR(300),reality_public_key VARCHAR(128),short_id VARCHAR(16),fingerprint VARCHAR(32),flow VARCHAR(64),transport VARCHAR(16),min_client_version VARCHAR(32),config_version BIGINT UNSIGNED,updated_at TIMESTAMP)`,
		`CREATE TABLE user (id INT PRIMARY KEY,passwd VARCHAR(64),u BIGINT,d BIGINT,transfer_enable BIGINT,enable INT,expire_in DATETIME,class INT,node_group INT,is_admin INT,node_speedlimit DOUBLE,node_connector INT,forbidden_ip TEXT,forbidden_port TEXT,disconnect_ip TEXT,t BIGINT DEFAULT 0)`,
		`CREATE TABLE vless_traffic_batches (batch_id CHAR(36) PRIMARY KEY,node_id INT,payload_sha256 BINARY(32),created_at BIGINT,applied_at BIGINT)`,
		`CREATE TABLE user_traffic_log (id BIGINT AUTO_INCREMENT PRIMARY KEY,user_id INT,u BIGINT,d BIGINT,node_id INT,rate DOUBLE,traffic VARCHAR(32),log_time BIGINT)`,
		`CREATE TABLE alive_ip (id BIGINT AUTO_INCREMENT PRIMARY KEY,nodeid INT,userid INT,ip TEXT,datetime BIGINT)`,
		`CREATE TABLE ss_node_online_log (id BIGINT AUTO_INCREMENT PRIMARY KEY,node_id INT,online_user INT,log_time BIGINT)`,
		"CREATE TABLE ss_node_info (id BIGINT AUTO_INCREMENT PRIMARY KEY,node_id INT,uptime DOUBLE,`load` VARCHAR(32),log_time BIGINT)",
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func stringTrimSpace(value []byte) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\n' || value[start] == '\r' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\t') {
		end--
	}
	return string(value[start:end])
}

func TestPasswordFixturePathIsAbsolute(t *testing.T) {
	if path := os.Getenv("VLESSHAPPY_TEST_MYSQL_PASSWORD_FILE"); path != "" && !filepath.IsAbs(path) {
		t.Fatal("test password file must be absolute")
	}
}
