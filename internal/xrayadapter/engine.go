package xrayadapter

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/sessioncontrol"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/stats"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	"github.com/xtls/xray-core/proxy"
	vless "github.com/xtls/xray-core/proxy/vless"

	"github.com/SadNoo/vlesshappy/internal/accounting"
	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/model"
	sessionregistry "github.com/SadNoo/vlesshappy/internal/session"
)

type Engine struct {
	mu         sync.Mutex
	instance   *core.Instance
	stats      stats.Manager
	users      proxy.UserManager
	sessions   *sessionregistry.Registry
	unregister func()
	snapshot   model.Snapshot
	retired    map[int64]struct{}
}

func Start(snapshot model.Snapshot, cfg config.Config, privateKey []byte) (*Engine, error) {
	listen := cfg.Listen
	configJSON, err := BuildConfig(snapshot, listen, privateKey)
	if err != nil {
		return nil, err
	}
	config, err := serial.LoadJSONConfig(bytes.NewReader(configJSON))
	if err != nil {
		return nil, fmt.Errorf("compile Xray config: %w", err)
	}
	sessions := sessionregistry.New(snapshot, cfg)
	unregister, err := sessioncontrol.Register(sessions)
	if err != nil {
		return nil, fmt.Errorf("register Xray session controller: %w", err)
	}
	fail := func(err error) (*Engine, error) {
		sessions.Close()
		unregister()
		return nil, err
	}
	instance, err := core.New(config)
	if err != nil {
		return fail(fmt.Errorf("create Xray core: %w", err))
	}
	if err := instance.Start(); err != nil {
		_ = instance.Close()
		return fail(fmt.Errorf("start Xray core: %w", err))
	}
	feature := instance.GetFeature(stats.ManagerType())
	manager, ok := feature.(stats.Manager)
	if !ok || manager == nil {
		_ = instance.Close()
		return fail(errors.New("Xray stats manager is unavailable"))
	}
	userManager, err := inboundUserManager(instance)
	if err != nil {
		_ = instance.Close()
		return fail(err)
	}
	return &Engine{instance: instance, stats: manager, users: userManager, sessions: sessions, unregister: unregister, snapshot: snapshot, retired: make(map[int64]struct{})}, nil
}

func inboundUserManager(instance *core.Instance) (proxy.UserManager, error) {
	feature := instance.GetFeature(inbound.ManagerType())
	manager, ok := feature.(inbound.Manager)
	if !ok || manager == nil {
		return nil, errors.New("Xray inbound manager is unavailable")
	}
	handler, err := manager.GetHandler(context.Background(), "vless-reality-in")
	if err != nil {
		return nil, fmt.Errorf("get VLESS inbound: %w", err)
	}
	provider, ok := handler.(proxy.GetInbound)
	if !ok {
		return nil, errors.New("VLESS inbound does not expose its user manager")
	}
	users, ok := provider.GetInbound().(proxy.UserManager)
	if !ok {
		return nil, errors.New("VLESS inbound user manager is unavailable")
	}
	return users, nil
}

func BuildConfig(snapshot model.Snapshot, listen string, privateKey []byte) ([]byte, error) {
	privateText, err := verifyRealityKey(privateKey, snapshot.Node.RealityPublicKey)
	if err != nil {
		return nil, err
	}
	host, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	clients := make([]map[string]any, 0, len(snapshot.Users))
	for _, user := range snapshot.Users {
		email := model.UserEmail(snapshot.Node.ID, user.ID)
		clients = append(clients, map[string]any{
			"id": user.UUID, "email": email, "level": 0, "flow": snapshot.Node.Flow,
		})
	}
	reality := map[string]any{
		"show": false, "target": snapshot.Node.Target, "xver": 0,
		"serverNames": []string{snapshot.Node.ServerName},
		"privateKey":  privateText, "shortIds": []string{snapshot.Node.ShortID},
	}
	if snapshot.Node.MinClientVersion != "" {
		reality["minClientVer"] = snapshot.Node.MinClientVersion
	}
	config := map[string]any{
		"log":   map[string]any{"loglevel": "warning"},
		"stats": map[string]any{},
		"policy": map[string]any{"levels": map[string]any{"0": map[string]any{
			"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true,
		}}},
		"inbounds": []any{map[string]any{
			"tag": "vless-reality-in", "listen": host, "port": port, "protocol": "vless",
			"settings": map[string]any{"clients": clients, "decryption": "none"},
			"streamSettings": map[string]any{
				"network": "raw", "security": "reality", "realitySettings": reality,
			},
		}},
		"outbounds": []any{
			map[string]any{"tag": "direct", "protocol": "freedom"},
			map[string]any{"tag": "blocked", "protocol": "blackhole"},
		},
		"routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}},
	}
	return json.Marshal(config)
}

func (e *Engine) CanApply(next model.Snapshot) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return dataPlaneNodeEqual(e.snapshot.Node, next.Node)
}

func (e *Engine) Apply(next model.Snapshot) ([]int64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.instance == nil {
		return nil, errors.New("Xray engine is closed")
	}
	if !dataPlaneNodeEqual(e.snapshot.Node, next.Node) {
		return nil, errors.New("node data-plane configuration requires a full restart")
	}

	oldByID := usersByID(e.snapshot.Users)
	newByID := usersByID(next.Users)
	remove := make([]model.User, 0)
	add := make([]model.User, 0)
	for id, old := range oldByID {
		updated, exists := newByID[id]
		if !exists || old.UUID != updated.UUID {
			remove = append(remove, old)
		}
	}
	for id, updated := range newByID {
		old, exists := oldByID[id]
		if !exists || old.UUID != updated.UUID {
			add = append(add, updated)
		}
	}
	sort.Slice(remove, func(i, j int) bool { return remove[i].ID < remove[j].ID })
	sort.Slice(add, func(i, j int) bool { return add[i].ID < add[j].ID })

	removed := make([]model.User, 0, len(remove))
	added := make([]model.User, 0, len(add))
	for _, user := range remove {
		if err := e.users.RemoveUser(context.Background(), model.UserEmail(e.snapshot.Node.ID, user.ID)); err != nil {
			return nil, e.rollbackUsers(added, removed, err)
		}
		removed = append(removed, user)
	}
	for _, user := range add {
		memory, err := memoryUser(next.Node, user)
		if err != nil {
			return nil, e.rollbackUsers(added, removed, err)
		}
		if err := e.users.AddUser(context.Background(), memory); err != nil {
			return nil, e.rollbackUsers(added, removed, err)
		}
		added = append(added, user)
	}
	revoked := e.sessions.Update(next)
	for id := range oldByID {
		if _, exists := newByID[id]; !exists {
			e.retired[id] = struct{}{}
		}
	}
	for id := range newByID {
		delete(e.retired, id)
	}
	e.snapshot = next
	return revoked, nil
}

func (e *Engine) rollbackUsers(added, removed []model.User, cause error) error {
	var rollbackErr error
	for _, user := range added {
		rollbackErr = errors.Join(rollbackErr, e.users.RemoveUser(context.Background(), model.UserEmail(e.snapshot.Node.ID, user.ID)))
	}
	for _, user := range removed {
		memory, err := memoryUser(e.snapshot.Node, user)
		if err == nil {
			err = e.users.AddUser(context.Background(), memory)
		}
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if rollbackErr != nil {
		return fmt.Errorf("apply users failed (%v), rollback failed: %w", cause, rollbackErr)
	}
	return cause
}

func memoryUser(node model.Node, user model.User) (*protocol.MemoryUser, error) {
	account, err := (&vless.Account{Id: user.UUID, Flow: node.Flow, Encryption: "none"}).AsAccount()
	if err != nil {
		return nil, fmt.Errorf("compile user %d: %w", user.ID, err)
	}
	return &protocol.MemoryUser{Account: account, Email: model.UserEmail(node.ID, user.ID), Level: 0}, nil
}

func dataPlaneNodeEqual(left, right model.Node) bool {
	return left.ID == right.ID && left.TrafficRate == right.TrafficRate && left.ServerName == right.ServerName && left.Target == right.Target &&
		left.ManagedCaddy == right.ManagedCaddy &&
		left.RealityPublicKey == right.RealityPublicKey && left.ShortID == right.ShortID &&
		left.Fingerprint == right.Fingerprint && left.Flow == right.Flow && left.Transport == right.Transport &&
		left.MinClientVersion == right.MinClientVersion
}

func usersByID(users []model.User) map[int64]model.User {
	result := make(map[int64]model.User, len(users))
	for _, user := range users {
		result[user.ID] = user
	}
	return result
}

func verifyRealityKey(privateKey []byte, publicText string) (string, error) {
	privateText := strings.TrimSpace(string(privateKey))
	if strings.Contains(privateText, "=") {
		return "", errors.New("REALITY private key must be unpadded base64url")
	}
	privateBytes, err := base64.RawURLEncoding.DecodeString(privateText)
	if err != nil || len(privateBytes) != 32 {
		return "", errors.New("REALITY private key must decode to 32 bytes")
	}
	key, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return "", errors.New("invalid REALITY private key")
	}
	derived := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	if derived != publicText {
		return "", errors.New("REALITY private key does not match panel public key")
	}
	return privateText, nil
}

func (e *Engine) Drain() ([]accounting.Entry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.drainLocked()
}

func (e *Engine) drainLocked() ([]accounting.Entry, error) {
	userIDs := make(map[int64]struct{}, len(e.snapshot.Users)+len(e.retired))
	for _, user := range e.snapshot.Users {
		userIDs[user.ID] = struct{}{}
	}
	for userID := range e.retired {
		userIDs[userID] = struct{}{}
	}
	ordered := make([]int64, 0, len(userIDs))
	for userID := range userIDs {
		ordered = append(ordered, userID)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	entries := make([]accounting.Entry, 0, len(ordered))
	for _, userID := range ordered {
		email := model.UserEmail(e.snapshot.Node.ID, userID)
		uplink, err := takeCounter(e.stats, "user>>>"+email+">>>traffic>>>uplink")
		if err != nil {
			return nil, err
		}
		downlink, err := takeCounter(e.stats, "user>>>"+email+">>>traffic>>>downlink")
		if err != nil {
			return nil, err
		}
		if uplink != 0 || downlink != 0 {
			entries = append(entries, accounting.Entry{UserID: userID, Uplink: uplink, Downlink: downlink})
		} else if _, retired := e.retired[userID]; retired && !e.sessions.HasUserSessions(userID) {
			delete(e.retired, userID)
		}
	}
	return entries, nil
}

func takeCounter(manager stats.Manager, name string) (int64, error) {
	counter := manager.GetCounter(name)
	if counter == nil {
		return 0, nil
	}
	value := counter.Set(0)
	if value < 0 {
		counter.Add(value)
		return 0, fmt.Errorf("negative Xray counter %q", name)
	}
	return value, nil
}

func (e *Engine) Restore(entries []accounting.Entry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, entry := range entries {
		email := model.UserEmail(e.snapshot.Node.ID, entry.UserID)
		if counter := e.stats.GetCounter("user>>>" + email + ">>>traffic>>>uplink"); counter != nil {
			counter.Add(entry.Uplink)
		}
		if counter := e.stats.GetCounter("user>>>" + email + ">>>traffic>>>downlink"); counter != nil {
			counter.Add(entry.Downlink)
		}
	}
}

func (e *Engine) Online() map[int64]map[string]time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessions.Online()
}

func (e *Engine) SessionMetrics() sessionregistry.Metrics {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessions.Metrics()
}

func (e *Engine) CloseAndDrain() ([]accounting.Entry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.instance == nil {
		return nil, nil
	}
	e.sessions.Close()
	closeErr := e.instance.Close()
	e.instance = nil
	if e.unregister != nil {
		e.unregister()
		e.unregister = nil
	}
	entries, drainErr := e.drainLocked()
	return entries, errors.Join(closeErr, drainErr)
}

func (e *Engine) Snapshot() model.Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot
}

func FlattenOnline(value map[int64]map[string]time.Time) map[int64][]string {
	result := make(map[int64][]string, len(value))
	for userID, byIP := range value {
		for ip := range byIP {
			result[userID] = append(result[userID], ip)
		}
		sort.Strings(result[userID])
	}
	return result
}
