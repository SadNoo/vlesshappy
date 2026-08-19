package setup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/lifecycle"
	"github.com/SadNoo/vlesshappy/internal/model"
	"github.com/SadNoo/vlesshappy/internal/realitykey"
)

const (
	defaultDataDir       = "/data"
	defaultVolumeName    = "vle-node-data"
	defaultContainerName = "vle-node"
)

type PasswordReader func() ([]byte, error)
type Validator func(context.Context, config.Config) (model.Snapshot, error)

type Options struct {
	DataDir       string
	VolumeName    string
	ContainerName string
	In            io.Reader
	Out           io.Writer
	ReadPassword  PasswordReader
	Validate      Validator
	Random        io.Reader
}

type answers struct {
	publicHost       string
	publicPort       int
	serverName       string
	databaseAddress  string
	databaseName     string
	databaseUsername string
	databasePassword []byte
	tlsMode          string
	caPEM            []byte
	databaseSNI      string
	nodeID           int64
	privateKey       string
	publicKey        string
	shortID          string
}

type prompt struct {
	in  io.Reader
	out io.Writer
}

func Run(ctx context.Context, opts Options) error {
	opts.defaults()
	if err := validateDockerName("volume name", opts.VolumeName); err != nil {
		return err
	}
	if err := validateDockerName("container name", opts.ContainerName); err != nil {
		return err
	}
	if !filepath.IsAbs(opts.DataDir) {
		return errors.New("setup data directory must be absolute")
	}
	if err := prepareDataDir(opts.DataDir); err != nil {
		return err
	}
	runtimeDir := filepath.Join(opts.DataDir, "runtime")
	if _, err := os.Lstat(runtimeDir); err == nil {
		return fmt.Errorf("%s is already initialized; setup refuses to overwrite it", runtimeDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect runtime directory: %w", err)
	}

	fmt.Fprintln(opts.Out, "vlesshappy 2.2 secure setup with managed Caddy")
	fmt.Fprintln(opts.Out, "No private key or database password will be printed or stored in Docker environment variables.")
	p := prompt{in: opts.In, out: opts.Out}
	answer, err := collect(p, opts.ReadPassword, opts.Random)
	if err != nil {
		return err
	}
	defer clear(answer.databasePassword)

	fmt.Fprintln(opts.Out, "\nCopy these PUBLIC values into the SSPanel VLESS REALITY node:")
	fmt.Fprintf(opts.Out, "public host: %s\n", answer.publicHost)
	fmt.Fprintf(opts.Out, "public port: %d\n", answer.publicPort)
	fmt.Fprintf(opts.Out, "REALITY SNI: %s\n", answer.serverName)
	fmt.Fprintf(opts.Out, "REALITY public key: %s\n", answer.publicKey)
	fmt.Fprintf(opts.Out, "REALITY short ID: %s\n", answer.shortID)
	fmt.Fprintf(opts.Out, "ss_node.server: %s\n", serverString(answer))
	fmt.Fprintln(opts.Out, "minimum client version: leave empty")
	fmt.Fprintln(opts.Out, "Save the node as enabled sort=15, then enter its ss_node.id below.")
	answer.nodeID, err = p.positiveInt64("SSPanel node ID")
	if err != nil {
		return err
	}

	stageDir := filepath.Join(opts.DataDir, ".setup")
	if err := resetStage(stageDir); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stageDir)
		}
	}()
	stageCfg, finalCfg, err := stage(stageDir, runtimeDir, answer)
	if err != nil {
		return err
	}
	if err := validateUntilReady(ctx, p, opts.Validate, stageCfg, answer); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(stageDir, "config.json"), finalCfg); err != nil {
		return err
	}
	if err := os.Rename(stageDir, runtimeDir); err != nil {
		return fmt.Errorf("activate setup: %w", err)
	}
	committed = true
	if err := syncDir(opts.DataDir); err != nil {
		return err
	}

	fmt.Fprintln(opts.Out, "\nSetup complete. Start the node with:")
	printRunCommand(opts.Out, opts.VolumeName, opts.ContainerName, answer.publicPort, answer.databaseAddress)
	return nil
}

func validateUntilReady(ctx context.Context, p prompt, validate Validator, cfg config.Config, answer answers) error {
	for {
		fmt.Fprintln(p.out, "\nValidating database, panel node and REALITY key pair...")
		snapshot, err := validate(ctx, cfg)
		if err == nil {
			err = matchPanel(snapshot.Node, answer)
		}
		if err == nil {
			return nil
		}
		fmt.Fprintf(p.out, "Validation error: %v\n", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		action, inputErr := p.line("Fix the database or panel node, then choose retry/cancel", "retry")
		if inputErr != nil || action == "cancel" {
			return fmt.Errorf("setup validation failed: %w", err)
		}
		if action != "retry" {
			fmt.Fprintln(p.out, "Enter retry or cancel.")
		}
	}
}

func (o *Options) defaults() {
	if o.DataDir == "" {
		o.DataDir = defaultDataDir
	}
	if o.VolumeName == "" {
		o.VolumeName = defaultVolumeName
	}
	if o.ContainerName == "" {
		o.ContainerName = defaultContainerName
	}
	if o.In == nil {
		o.In = os.Stdin
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.ReadPassword == nil {
		o.ReadPassword = func() ([]byte, error) {
			return nil, errors.New("a terminal password reader is required")
		}
	}
	if o.Validate == nil {
		o.Validate = lifecycle.ValidateSnapshot
	}
	if o.Random == nil {
		o.Random = rand.Reader
	}
}

func matchPanel(node model.Node, answer answers) error {
	fields := []struct {
		name     string
		expected string
		actual   string
	}{
		{"public host", answer.publicHost, node.PublicHost},
		{"public port", strconv.Itoa(answer.publicPort), strconv.Itoa(node.PublicPort)},
		{"REALITY SNI", answer.serverName, node.ServerName},
		{"REALITY public key", answer.publicKey, node.RealityPublicKey},
		{"REALITY short ID", answer.shortID, node.ShortID},
	}
	for _, field := range fields {
		if field.expected != field.actual {
			return fmt.Errorf("panel %s does not match setup output; correct the node and run setup again", field.name)
		}
	}
	return nil
}

func collect(p prompt, readPassword PasswordReader, random io.Reader) (answers, error) {
	var answer answers
	var err error
	if answer.publicHost, err = p.host("Public node address"); err != nil {
		return answer, err
	}
	if answer.publicPort, err = p.port("Public TCP port", 2053); err != nil {
		return answer, err
	}
	if answer.publicPort == 80 {
		return answer, errors.New("public TCP port 80 is reserved for Caddy certificate issuance")
	}
	if answer.serverName, err = p.dnsName("REALITY SNI"); err != nil {
		return answer, err
	}
	if answer.databaseAddress, err = p.target("MySQL address", ""); err != nil {
		return answer, err
	}
	if answer.databaseName, err = p.required("MySQL database", "sspanel"); err != nil {
		return answer, err
	}
	if strings.ContainsAny(answer.databaseName, "`\\/\x00") {
		return answer, errors.New("MySQL database contains forbidden characters")
	}
	if answer.databaseUsername, err = p.required("MySQL username", ""); err != nil {
		return answer, err
	}
	fmt.Fprint(p.out, "MySQL password: ")
	answer.databasePassword, err = readPassword()
	if err != nil {
		return answer, fmt.Errorf("read MySQL password: %w", err)
	}
	answer.databasePassword = bytes.TrimSpace(answer.databasePassword)
	if len(answer.databasePassword) == 0 {
		return answer, errors.New("MySQL password must not be empty")
	}
	fmt.Fprint(p.out, "Confirm MySQL password: ")
	confirmation, err := readPassword()
	if err != nil {
		return answer, fmt.Errorf("confirm MySQL password: %w", err)
	}
	confirmation = bytes.TrimSpace(confirmation)
	defer clear(confirmation)
	if !bytes.Equal(answer.databasePassword, confirmation) {
		return answer, errors.New("MySQL passwords do not match")
	}
	if answer.tlsMode, err = p.choice("MySQL TLS mode", "disabled", "disabled", "required", "verify_ca"); err != nil {
		return answer, err
	}
	if answer.tlsMode == "verify_ca" {
		if answer.databaseSNI, err = p.dnsName("MySQL certificate server name"); err != nil {
			return answer, err
		}
		fmt.Fprintln(p.out, "Paste the MySQL CA PEM, then enter a line containing only a single dot:")
		if answer.caPEM, err = p.pem(); err != nil {
			return answer, err
		}
	}
	if answer.privateKey, answer.publicKey, err = realitykey.Generate(random); err != nil {
		return answer, err
	}
	shortID := make([]byte, 8)
	if _, err := io.ReadFull(random, shortID); err != nil {
		return answer, fmt.Errorf("generate REALITY short ID: %w", err)
	}
	answer.shortID = hex.EncodeToString(shortID)
	return answer, nil
}

func stage(stageDir, runtimeDir string, answer answers) (config.Config, config.Config, error) {
	if err := os.Mkdir(stageDir, 0o700); err != nil {
		return config.Config{}, config.Config{}, fmt.Errorf("create setup staging directory: %w", err)
	}
	stageSecrets := filepath.Join(stageDir, "secrets")
	if err := os.Mkdir(stageSecrets, 0o700); err != nil {
		return config.Config{}, config.Config{}, err
	}
	if err := os.Mkdir(filepath.Join(stageDir, "state"), 0o700); err != nil {
		return config.Config{}, config.Config{}, err
	}
	if err := os.Mkdir(filepath.Join(stageDir, "caddy"), 0o700); err != nil {
		return config.Config{}, config.Config{}, err
	}
	if err := writeSecret(filepath.Join(stageSecrets, "vlesshappy_reality_private_key"), []byte(answer.privateKey+"\n")); err != nil {
		return config.Config{}, config.Config{}, err
	}
	if err := writeSecret(filepath.Join(stageSecrets, "vlesshappy_database_password"), append(answer.databasePassword, '\n')); err != nil {
		return config.Config{}, config.Config{}, err
	}
	if len(answer.caPEM) > 0 {
		if err := writeSecret(filepath.Join(stageSecrets, "mysql_ca.pem"), answer.caPEM); err != nil {
			return config.Config{}, config.Config{}, err
		}
	}
	stageCfg := buildConfig(stageDir, answer)
	finalCfg := buildConfig(runtimeDir, answer)
	if err := stageCfg.Validate(); err != nil {
		return config.Config{}, config.Config{}, err
	}
	if err := finalCfg.Validate(); err != nil {
		return config.Config{}, config.Config{}, err
	}
	return stageCfg, finalCfg, nil
}

func buildConfig(base string, answer answers) config.Config {
	databaseCA := ""
	if answer.tlsMode == "verify_ca" {
		databaseCA = filepath.Join(base, "secrets", "mysql_ca.pem")
	}
	return config.Config{
		NodeID: answer.nodeID, Listen: "0.0.0.0:8443",
		StateDir:              filepath.Join(base, "state"),
		CaddyDir:              filepath.Join(base, "caddy"),
		RealityPrivateKeyFile: filepath.Join(base, "secrets", "vlesshappy_reality_private_key"),
		Database: config.DatabaseConfig{
			Address: answer.databaseAddress, Name: answer.databaseName, Username: answer.databaseUsername,
			PasswordFile: filepath.Join(base, "secrets", "vlesshappy_database_password"),
			TLSMode:      answer.tlsMode, CAFile: databaseCA, ServerName: answer.databaseSNI,
			MaxOpen: 4, MaxIdle: 2,
		},
		Intervals: config.Intervals{
			AuthRefreshSeconds: 60, ReportSeconds: 60, AuthStaleSeconds: 3600, ShutdownSeconds: 120,
		},
		Limits: config.Limits{
			TCPPerUser: 800, TCPGlobal: 3000, ConcurrentHandshakes: 1024,
			XUDPPerUser: 128, XUDPGlobal: 2048, DNSResolveTimeoutSeconds: 10,
		},
		LogLevel: "info",
	}
}

func prepareDataDir(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("setup data path must be a directory and not a symlink")
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create setup data directory: %w", err)
		}
	} else {
		return fmt.Errorf("inspect setup data directory: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure setup data directory: %w", err)
	}
	return nil
}

func resetStage(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("setup staging path is not a safe directory")
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove incomplete setup staging directory: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect setup staging directory: %w", err)
	}
	return nil
}

func writeSecret(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create secret file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write secret file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync secret file: %w", err)
	}
	return file.Close()
}

func writeJSON(path string, cfg config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create generated config: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func printRunCommand(out io.Writer, volumeName, containerName string, publicPort int, databaseAddress string) {
	fmt.Fprintln(out, "docker run -d \\")
	fmt.Fprintf(out, "  --name %s \\\n", containerName)
	fmt.Fprintln(out, "  --restart unless-stopped \\")
	fmt.Fprintln(out, "  --stop-timeout 120 \\")
	fmt.Fprintln(out, "  --read-only \\")
	fmt.Fprintln(out, "  --cap-drop ALL \\")
	fmt.Fprintln(out, "  --security-opt no-new-privileges \\")
	if host, _, err := net.SplitHostPort(databaseAddress); err == nil && host == "host.docker.internal" {
		fmt.Fprintln(out, "  --add-host host.docker.internal:host-gateway \\")
	}
	fmt.Fprintln(out, "  -p 80:8080/tcp \\")
	fmt.Fprintf(out, "  -p %d:8443/tcp \\\n", publicPort)
	fmt.Fprintf(out, "  -v %s:/data \\\n", volumeName)
	fmt.Fprintln(out, "  sadno/vle:2.2")
}

func serverString(answer answers) string {
	return fmt.Sprintf("%s;%d;0;tcp;reality;sni=%s|pbk=%s|sid=%s",
		answer.publicHost, answer.publicPort, answer.serverName, answer.publicKey, answer.shortID)
}

func validateDockerName(label, value string) error {
	if value == "" || len(value) > 128 {
		return fmt.Errorf("%s is invalid", label)
	}
	for i, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || (i > 0 && (char == '_' || char == '.' || char == '-')) {
			continue
		}
		return fmt.Errorf("%s contains invalid characters", label)
	}
	return nil
}

func (p prompt) line(label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(p.out, "%s: ", label)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", label, defaultValue)
	}
	line, err := readLine(p.in, 4096)
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = defaultValue
	}
	return line, nil
}

func (p prompt) required(label, defaultValue string) (string, error) {
	value, err := p.line(label, defaultValue)
	if err != nil {
		return "", err
	}
	if value == "" || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}

func (p prompt) host(label string) (string, error) {
	value, err := p.required(label, "")
	if err != nil {
		return "", err
	}
	if err := validateHost(value); err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return value, nil
}

func (p prompt) dnsName(label string) (string, error) {
	value, err := p.required(label, "")
	if err != nil {
		return "", err
	}
	if err := validateDNSName(value); err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return value, nil
}

func (p prompt) port(label string, defaultValue int) (int, error) {
	value, err := p.line(label, strconv.Itoa(defaultValue))
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be between 1 and 65535", label)
	}
	return port, nil
}

func (p prompt) target(label, defaultValue string) (string, error) {
	value, err := p.required(label, defaultValue)
	if err != nil {
		return "", err
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || validateHost(host) != nil {
		return "", fmt.Errorf("%s must be host:port", label)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("%s has an invalid port", label)
	}
	return value, nil
}

func (p prompt) positiveInt64(label string) (int64, error) {
	value, err := p.required(label, "")
	if err != nil {
		return 0, err
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", label)
	}
	return number, nil
}

func (p prompt) choice(label, defaultValue string, choices ...string) (string, error) {
	value, err := p.line(label+" ("+strings.Join(choices, "/")+")", defaultValue)
	if err != nil {
		return "", err
	}
	for _, choice := range choices {
		if value == choice {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s must be one of %s", label, strings.Join(choices, ", "))
}

func (p prompt) pem() ([]byte, error) {
	var data bytes.Buffer
	for {
		line, err := readLine(p.in, 64*1024)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line) == "." {
			break
		}
		data.WriteString(line)
		data.WriteByte('\n')
		if data.Len() > 1024*1024 {
			return nil, errors.New("MySQL CA PEM is too large")
		}
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data.Bytes()) {
		return nil, errors.New("MySQL CA PEM contains no certificates")
	}
	return data.Bytes(), nil
}

func readLine(reader io.Reader, limit int) (string, error) {
	var data bytes.Buffer
	var one [1]byte
	for data.Len() <= limit {
		n, err := reader.Read(one[:])
		if n == 1 {
			if one[0] == '\n' {
				return strings.TrimSuffix(data.String(), "\r"), nil
			}
			data.WriteByte(one[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) && data.Len() > 0 {
				return data.String(), nil
			}
			return "", err
		}
	}
	return "", errors.New("input line is too long")
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
		for _, char := range label {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') || char == '-') {
				return errors.New("invalid hostname character")
			}
		}
	}
	return nil
}

func clear(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
