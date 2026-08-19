package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/SadNoo/vlesshappy/internal/accounting"
	"github.com/SadNoo/vlesshappy/internal/caddyservice"
	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/database"
	"github.com/SadNoo/vlesshappy/internal/model"
	sessionregistry "github.com/SadNoo/vlesshappy/internal/session"
	"github.com/SadNoo/vlesshappy/internal/xrayadapter"
)

type runtimeState struct {
	cfg          config.Config
	db           *database.DB
	engine       *xrayadapter.Engine
	caddy        *caddyservice.Process
	privateKey   []byte
	started      time.Time
	lastAuth     time.Time
	lastRejected map[string]uint64
	logger       *slog.Logger
}

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	state, lock, err := bootstrap(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer lock.Close()
	defer state.db.Close()
	logger.Info("service started", "component", "lifecycle", "event", "serving", "node_id", cfg.NodeID)

	authTicker := time.NewTicker(cfg.AuthRefresh())
	reportTicker := time.NewTicker(cfg.ReportEvery())
	defer authTicker.Stop()
	defer reportTicker.Stop()

	var runErr error
	for runErr == nil {
		select {
		case <-ctx.Done():
			runErr = ctx.Err()
		case <-state.caddyDone():
			runErr = state.caddy.Err()
			if runErr == nil {
				runErr = errors.New("Caddy stopped unexpectedly")
			} else {
				runErr = fmt.Errorf("Caddy stopped unexpectedly: %w", runErr)
			}
		case <-reportTicker.C:
			runErr = state.report(ctx)
		case <-authTicker.C:
			runErr = state.refresh(ctx)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Shutdown())
	defer cancel()
	shutdownErr := state.shutdown(shutdownCtx)
	if errors.Is(runErr, context.Canceled) {
		runErr = nil
	}
	return errors.Join(runErr, shutdownErr)
}

func Validate(ctx context.Context, cfg config.Config) error {
	_, err := ValidateSnapshot(ctx, cfg)
	return err
}

func ValidateSnapshot(ctx context.Context, cfg config.Config) (model.Snapshot, error) {
	var empty model.Snapshot
	privateKey, err := config.ReadSecret(cfg.RealityPrivateKeyFile)
	if err != nil {
		return empty, fmt.Errorf("REALITY secret: %w", err)
	}
	password, err := config.ReadSecret(cfg.Database.PasswordFile)
	if err != nil {
		return empty, fmt.Errorf("database secret: %w", err)
	}
	defer clear(password)
	db, err := database.Open(cfg.Database, password)
	if err != nil {
		return empty, err
	}
	defer db.Close()
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := db.Ping(checkCtx); err != nil {
		return empty, fmt.Errorf("database ping: %w", err)
	}
	snapshot, _, err := db.LoadSnapshot(checkCtx, cfg.NodeID)
	if err != nil {
		return empty, err
	}
	if err := validateCaddyContract(cfg, snapshot.Node); err != nil {
		return empty, err
	}
	if _, err := xrayadapter.BuildConfig(snapshot, cfg.Listen, privateKey); err != nil {
		return empty, err
	}
	return snapshot, nil
}

func bootstrap(ctx context.Context, cfg config.Config, logger *slog.Logger) (*runtimeState, *stateLock, error) {
	lock, err := lockStateDir(cfg.StateDir)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*runtimeState, *stateLock, error) {
		lock.Close()
		return nil, nil, err
	}
	privateKey, err := config.ReadSecret(cfg.RealityPrivateKeyFile)
	if err != nil {
		return fail(fmt.Errorf("REALITY secret: %w", err))
	}
	password, err := config.ReadSecret(cfg.Database.PasswordFile)
	if err != nil {
		return fail(fmt.Errorf("database secret: %w", err))
	}
	db, err := database.Open(cfg.Database, password)
	clear(password)
	if err != nil {
		return fail(err)
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := db.Ping(checkCtx); err != nil {
		db.Close()
		return fail(fmt.Errorf("database ping: %w", err))
	}
	state := &runtimeState{
		cfg: cfg, db: db, privateKey: privateKey, started: time.Now(),
		lastAuth: time.Now(), lastRejected: make(map[string]uint64), logger: logger,
	}
	snapshot, warnings, err := db.LoadSnapshot(checkCtx, cfg.NodeID)
	if err != nil {
		db.Close()
		return fail(err)
	}
	for _, warning := range warnings {
		logger.Warn("user isolated", "component", "auth", "event", "invalid_user", "detail", warning, "node_id", cfg.NodeID)
	}
	if err := validateCaddyContract(cfg, snapshot.Node); err != nil {
		db.Close()
		return fail(err)
	}
	if snapshot.Node.ManagedCaddy {
		caddy, err := caddyservice.Start(ctx, caddyservice.Options{
			Dir: cfg.CaddyDir, ServerName: snapshot.Node.ServerName,
		})
		if err != nil {
			db.Close()
			return fail(err)
		}
		state.caddy = caddy
		logger.Info("Caddy certificate ready", "component", "caddy", "event", "ready", "server_name", snapshot.Node.ServerName, "node_id", cfg.NodeID)
	}
	engine, err := xrayadapter.Start(snapshot, cfg, privateKey)
	if err != nil {
		if state.caddy != nil {
			stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			_ = state.caddy.Stop(stopCtx)
			stop()
		}
		db.Close()
		return fail(err)
	}
	state.engine = engine
	return state, lock, nil
}

func (s *runtimeState) report(ctx context.Context) error {
	entries, err := s.engine.Drain()
	if err != nil {
		return err
	}
	dbCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.applyTraffic(dbCtx, entries, s.engine.Snapshot()); err != nil {
		s.engine.Restore(entries)
		s.logger.Warn("traffic report deferred", "component", "accounting", "event", "database_unavailable", "error", err, "node_id", s.cfg.NodeID)
	}
	online := xrayadapter.FlattenOnline(s.engine.Online())
	metrics := s.engine.SessionMetrics()
	s.logSessionMetrics(metrics)
	if err := s.db.ReportTelemetry(dbCtx, s.cfg.NodeID, online, time.Since(s.started), systemLoad(metrics)); err != nil {
		s.logger.Warn("telemetry deferred", "component", "telemetry", "event", "database_unavailable", "error", err, "node_id", s.cfg.NodeID)
	}
	return nil
}

func (s *runtimeState) logSessionMetrics(metrics sessionregistry.Metrics) {
	for reason, total := range metrics.Rejected {
		previous := s.lastRejected[reason]
		if total > previous {
			s.logger.Warn("sessions rejected", "component", "session", "event", "limit_or_policy", "reason", reason, "count", total-previous, "node_id", s.cfg.NodeID)
		}
		s.lastRejected[reason] = total
	}
	s.logger.Info("session totals", "component", "session", "event", "active", "tcp", metrics.TCPActive, "xudp", metrics.XUDPActive, "handshakes", metrics.Handshakes, "node_id", s.cfg.NodeID)
}

func (s *runtimeState) refresh(ctx context.Context) error {
	dbCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	snapshot, warnings, err := s.db.LoadSnapshot(dbCtx, s.cfg.NodeID)
	if err != nil {
		if errors.Is(err, database.ErrNodeUnavailable) {
			return err
		}
		age := time.Since(s.lastAuth)
		s.logger.Warn("authorization refresh failed", "component", "auth", "event", "degraded", "age_seconds", int(age.Seconds()), "error", err, "node_id", s.cfg.NodeID)
		if age >= s.cfg.AuthStale() {
			return errors.New("authorization snapshot exceeded maximum age")
		}
		return nil
	}
	s.lastAuth = time.Now()
	for _, warning := range warnings {
		s.logger.Warn("user isolated", "component", "auth", "event", "invalid_user", "detail", warning, "node_id", s.cfg.NodeID)
	}
	if snapshot.Hash == s.engine.Snapshot().Hash {
		return nil
	}
	return s.reload(dbCtx, snapshot)
}

func (s *runtimeState) reload(ctx context.Context, next model.Snapshot) error {
	previous := s.engine.Snapshot()
	if s.caddy != nil && previous.Node.ServerName != next.Node.ServerName {
		return errors.New("managed Caddy SNI changed; restart the container after DNS is ready")
	}
	if s.engine.CanApply(next) {
		entries, err := s.engine.Drain()
		if err != nil {
			return err
		}
		if err := s.applyTraffic(ctx, entries, previous); err != nil {
			s.engine.Restore(entries)
			return fmt.Errorf("report traffic before authorization update: %w", err)
		}
		revoked, err := s.engine.Apply(next)
		if err != nil {
			return fmt.Errorf("apply authorization update: %w", err)
		}
		s.logger.Info("authorization applied", "component", "auth", "event", "targeted_reload", "users", len(next.Users), "revoked_users", len(revoked), "node_id", s.cfg.NodeID)
		return nil
	}
	entries, closeErr := s.engine.CloseAndDrain()
	if closeErr != nil {
		return closeErr
	}
	if err := s.applyTraffic(ctx, entries, previous); err != nil {
		rollback, rollbackErr := xrayadapter.Start(previous, s.cfg, s.privateKey)
		if rollbackErr == nil {
			rollback.Restore(entries)
			s.engine = rollback
		}
		if rollbackErr != nil {
			return fmt.Errorf("report traffic before reload failed (%v), rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("report traffic before reload: %w", err)
	}
	engine, err := xrayadapter.Start(next, s.cfg, s.privateKey)
	if err == nil {
		s.engine = engine
		s.logger.Info("authorization applied", "component", "auth", "event", "reload", "users", len(next.Users), "node_id", s.cfg.NodeID)
		return nil
	}
	rollback, rollbackErr := xrayadapter.Start(previous, s.cfg, s.privateKey)
	if rollbackErr != nil {
		return fmt.Errorf("new Xray config failed (%v), rollback failed: %w", err, rollbackErr)
	}
	s.engine = rollback
	return fmt.Errorf("new Xray config rejected and rolled back: %w", err)
}

func (s *runtimeState) applyTraffic(ctx context.Context, entries []accounting.Entry, snapshot model.Snapshot) error {
	return s.db.ApplyTraffic(ctx, snapshot.Node.ID, snapshot.Node.TrafficRate, entries)
}

func (s *runtimeState) shutdown(ctx context.Context) error {
	s.logger.Info("service draining", "component", "lifecycle", "event", "draining", "node_id", s.cfg.NodeID)
	snapshot := s.engine.Snapshot()
	entries, closeErr := s.engine.CloseAndDrain()
	reportErr := s.applyTraffic(ctx, entries, snapshot)
	var caddyErr error
	if s.caddy != nil {
		caddyErr = s.caddy.Stop(ctx)
	}
	return errors.Join(closeErr, reportErr, caddyErr)
}

func (s *runtimeState) caddyDone() <-chan struct{} {
	if s == nil || s.caddy == nil {
		return nil
	}
	return s.caddy.Done()
}

func validateCaddyContract(cfg config.Config, node model.Node) error {
	managed := cfg.CaddyDir != ""
	if managed != node.ManagedCaddy {
		if managed {
			return errors.New("caddy_dir requires an ss_node.server without target=")
		}
		return errors.New("ss_node.server without target= requires caddy_dir")
	}
	return nil
}

func systemLoad(metrics sessionregistry.Metrics) string {
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			return fmt.Sprintf("%s t=%d x=%d", strings.Join(fields[:3], "/"), metrics.TCPActive, metrics.XUDPActive)
		}
	}
	return fmt.Sprintf("g=%d t=%d x=%d", runtime.NumGoroutine(), metrics.TCPActive, metrics.XUDPActive)
}

func clear(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

type stateLock struct {
	file *os.File
}

func lockStateDir(path string) (*stateLock, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, errors.New("state_dir must be a directory and not a symlink")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("create state_dir: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return nil, fmt.Errorf("secure state_dir: %w", err)
	}
	file, err := os.OpenFile(path+"/vlesshappy.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("state_dir is already locked by another process")
	}
	return &stateLock{file: file}, nil
}

func (l *stateLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return errors.Join(unlockErr, l.file.Close())
}
