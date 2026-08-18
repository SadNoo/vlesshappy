package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/xtls/xray-core/common/sessioncontrol"

	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/model"
	"github.com/SadNoo/vlesshappy/internal/policy"
)

const inboundTag = "vless-reality-in"

type Kind uint8

const (
	TCP Kind = iota + 1
	XUDP
)

type Metrics struct {
	TCPActive  int64
	XUDPActive int64
	Handshakes int64
	Rejected   map[string]uint64
}

type Registry struct {
	mu             sync.Mutex
	limits         config.Limits
	dnsTimeout     time.Duration
	resolver       *net.Resolver
	nodeID         int64
	nodeSpeed      float64
	nodeConnectors int
	users          map[int64]model.User
	sessions       map[uint64]*lease
	sources        map[int64]map[string]int
	userTCP        map[int64]int
	userXUDP       map[int64]int
	tcp            int
	xudp           int
	handshakes     int
	nextID         uint64
	rejected       map[string]uint64
	nodeLimiter    *rate.Limiter
	userLimiters   map[int64]*rate.Limiter
	closed         bool
}

type lease struct {
	registry  *Registry
	id        uint64
	userID    int64
	sourceIP  string
	kind      Kind
	ctx       context.Context
	cancel    context.CancelFunc
	once      sync.Once
	interrupt func()
	revoked   bool
	userRate  *rate.Limiter
	nodeRate  *rate.Limiter
}

func New(snapshot model.Snapshot, cfg config.Config) *Registry {
	r := &Registry{
		limits: cfg.Limits, dnsTimeout: cfg.DNSResolveTimeout(), resolver: net.DefaultResolver,
		users: make(map[int64]model.User), sessions: make(map[uint64]*lease),
		sources: make(map[int64]map[string]int), userTCP: make(map[int64]int),
		userXUDP: make(map[int64]int), rejected: make(map[string]uint64),
		userLimiters: make(map[int64]*rate.Limiter),
	}
	r.installSnapshot(snapshot, nil)
	return r
}

func (r *Registry) AcquireHandshake(_ context.Context, request sessioncontrol.Request) (func(), error) {
	if request.InboundTag != inboundTag {
		return func() {}, nil
	}
	r.mu.Lock()
	if r.closed || r.handshakes >= r.limits.ConcurrentHandshakes {
		r.rejectLocked("handshake_limit")
		r.mu.Unlock()
		return nil, errors.New("concurrent handshake limit reached")
	}
	r.handshakes++
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			if r.handshakes > 0 {
				r.handshakes--
			}
			r.mu.Unlock()
		})
	}, nil
}

func (r *Registry) Open(ctx context.Context, request sessioncontrol.Request) (sessioncontrol.Decision, error) {
	if request.InboundTag != inboundTag {
		return sessioncontrol.Decision{}, nil
	}
	userID, err := r.userID(request.Email)
	if err != nil {
		return sessioncontrol.Decision{}, r.reject("identity")
	}
	sourceIP, err := policy.CanonicalIP(request.SourceIP)
	if err != nil {
		return sessioncontrol.Decision{}, r.reject("source")
	}
	kind, err := requestKind(request.Network)
	if err != nil || request.DestinationPort == 0 {
		return sessioncontrol.Decision{}, r.reject("network")
	}

	r.mu.Lock()
	user, exists := r.users[userID]
	r.mu.Unlock()
	if !exists {
		return sessioncontrol.Decision{}, r.reject("authorization")
	}
	addresses, resolved, err := r.authorizeTarget(ctx, user, request)
	if err != nil {
		return sessioncontrol.Decision{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	user, exists = r.users[userID]
	if r.closed || !exists {
		return sessioncontrol.Decision{}, r.rejectLockedError("authorization")
	}
	if contains(user.DisconnectIPs, sourceIP) {
		return sessioncontrol.Decision{}, r.rejectLockedError("disconnect_ip")
	}
	if policy.PortBlocked(user.ForbiddenPorts, int(request.DestinationPort)) {
		return sessioncontrol.Decision{}, r.rejectLockedError("forbidden_port")
	}
	for _, address := range addresses {
		if policy.IPBlocked(user.ForbiddenIPs, address) {
			return sessioncontrol.Decision{}, r.rejectLockedError("forbidden_ip")
		}
	}
	if err := r.checkCapacityLocked(user, userID, sourceIP, kind); err != nil {
		return sessioncontrol.Decision{}, err
	}

	r.nextID++
	leaseCtx, cancel := context.WithCancel(ctx)
	l := &lease{
		registry: r, id: r.nextID, userID: userID, sourceIP: sourceIP, kind: kind,
		ctx: leaseCtx, cancel: cancel, userRate: r.userLimiters[userID], nodeRate: r.nodeLimiter,
	}
	r.sessions[l.id] = l
	if r.sources[userID] == nil {
		r.sources[userID] = make(map[string]int)
	}
	r.sources[userID][sourceIP]++
	if kind == TCP {
		r.tcp++
		r.userTCP[userID]++
	} else {
		r.xudp++
		r.userXUDP[userID]++
	}
	return sessioncontrol.Decision{ResolvedAddress: resolved, Lease: l}, nil
}

func (r *Registry) authorizeTarget(ctx context.Context, user model.User, request sessioncontrol.Request) ([]string, string, error) {
	if policy.PortBlocked(user.ForbiddenPorts, int(request.DestinationPort)) {
		return nil, "", r.reject("forbidden_port")
	}
	host := strings.Trim(request.DestinationHost, "[]")
	if address, err := netip.ParseAddr(host); err == nil {
		canonical := address.Unmap().String()
		if policy.IPBlocked(user.ForbiddenIPs, canonical) {
			return nil, "", r.reject("forbidden_ip")
		}
		return []string{canonical}, "", nil
	}
	if !request.DestinationDomain || host == "" {
		return nil, "", r.reject("destination")
	}
	resolveCtx, cancel := context.WithTimeout(ctx, r.dnsTimeout)
	defer cancel()
	resolved, err := r.resolver.LookupNetIP(resolveCtx, "ip", host)
	if err != nil || len(resolved) == 0 {
		return nil, "", r.reject("dns")
	}
	seen := make(map[string]struct{}, len(resolved))
	addresses := make([]string, 0, len(resolved))
	for _, address := range resolved {
		canonical := address.Unmap().String()
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		addresses = append(addresses, canonical)
	}
	sort.Strings(addresses)
	for _, address := range addresses {
		if policy.IPBlocked(user.ForbiddenIPs, address) {
			return nil, "", r.reject("forbidden_ip")
		}
	}
	if len(addresses) == 0 {
		return nil, "", r.reject("dns")
	}
	// Pin the outbound to a checked address so a second resolver lookup cannot
	// rebind the request to an address that bypassed the user policy.
	return addresses, addresses[0], nil
}

func (r *Registry) checkCapacityLocked(user model.User, userID int64, source string, kind Kind) error {
	connectorLimit := effectiveLimit(user.ConnectorLimit, r.nodeConnectors)
	if connectorLimit > 0 && r.sources[userID][source] == 0 && len(r.sources[userID]) >= connectorLimit {
		return r.rejectLockedError("connector_limit")
	}
	if kind == TCP {
		if r.tcp >= r.limits.TCPGlobal {
			return r.rejectLockedError("tcp_global")
		}
		if r.userTCP[userID] >= r.limits.TCPPerUser {
			return r.rejectLockedError("tcp_user")
		}
		return nil
	}
	if r.xudp >= r.limits.XUDPGlobal {
		return r.rejectLockedError("xudp_global")
	}
	if r.userXUDP[userID] >= r.limits.XUDPPerUser {
		return r.rejectLockedError("xudp_user")
	}
	return nil
}

func (r *Registry) Update(snapshot model.Snapshot) []int64 {
	r.mu.Lock()
	oldUsers := r.users
	newUsers := userMap(snapshot.Users)
	revokeAll := make(map[int64]struct{})
	revokeSources := make(map[int64]map[string]struct{})
	for id, old := range oldUsers {
		updated, exists := newUsers[id]
		if !exists || userPolicyWithoutDisconnectChanged(old, updated) {
			revokeAll[id] = struct{}{}
			continue
		}
		for _, source := range updated.DisconnectIPs {
			if !contains(old.DisconnectIPs, source) {
				if revokeSources[id] == nil {
					revokeSources[id] = make(map[string]struct{})
				}
				revokeSources[id][source] = struct{}{}
			}
		}
	}
	if r.nodeSpeed != snapshot.Node.SpeedLimitMbps {
		for id := range oldUsers {
			revokeAll[id] = struct{}{}
		}
	}
	r.installSnapshot(snapshot, oldUsers)
	callbacks := r.revokeLocked(revokeAll, revokeSources)
	connectorCallbacks, connectorUsers := r.enforceConnectorsLocked()
	callbacks = append(callbacks, connectorCallbacks...)
	r.mu.Unlock()
	for _, callback := range callbacks {
		callback()
	}
	affected := make(map[int64]struct{}, len(revokeAll)+len(revokeSources)+len(connectorUsers))
	for id := range revokeAll {
		affected[id] = struct{}{}
	}
	for id := range revokeSources {
		affected[id] = struct{}{}
	}
	for id := range connectorUsers {
		affected[id] = struct{}{}
	}
	ids := make([]int64, 0, len(affected))
	for id := range affected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (r *Registry) RevokeUsers(ids []int64) {
	r.mu.Lock()
	targets := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		targets[id] = struct{}{}
	}
	callbacks := r.revokeLocked(targets, nil)
	r.mu.Unlock()
	for _, callback := range callbacks {
		callback()
	}
}

func (r *Registry) Close() {
	r.mu.Lock()
	r.closed = true
	targets := make(map[int64]struct{}, len(r.users))
	for id := range r.users {
		targets[id] = struct{}{}
	}
	callbacks := r.revokeLocked(targets, nil)
	r.mu.Unlock()
	for _, callback := range callbacks {
		callback()
	}
}

func (r *Registry) Online() map[int64]map[string]time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	result := make(map[int64]map[string]time.Time, len(r.sources))
	for userID, sources := range r.sources {
		if len(sources) == 0 {
			continue
		}
		result[userID] = make(map[string]time.Time, len(sources))
		for source := range sources {
			result[userID][source] = now
		}
	}
	return result
}

func (r *Registry) Metrics() Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	rejected := make(map[string]uint64, len(r.rejected))
	for reason, count := range r.rejected {
		rejected[reason] = count
	}
	return Metrics{TCPActive: int64(r.tcp), XUDPActive: int64(r.xudp), Handshakes: int64(r.handshakes), Rejected: rejected}
}

func (r *Registry) HasUserSessions(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.userTCP[userID] > 0 || r.userXUDP[userID] > 0
}

func (r *Registry) installSnapshot(snapshot model.Snapshot, old map[int64]model.User) {
	oldNodeSpeed := r.nodeSpeed
	r.nodeID = snapshot.Node.ID
	r.nodeSpeed = snapshot.Node.SpeedLimitMbps
	r.nodeConnectors = snapshot.Node.ConnectorLimit
	r.users = userMap(snapshot.Users)
	if r.nodeLimiter == nil || old == nil || oldNodeSpeed != snapshot.Node.SpeedLimitMbps {
		r.nodeLimiter = newLimiter(snapshot.Node.SpeedLimitMbps)
	}
	updatedLimiters := make(map[int64]*rate.Limiter, len(r.users))
	for id, user := range r.users {
		if previous, exists := old[id]; exists && previous.SpeedLimitMbps == user.SpeedLimitMbps {
			updatedLimiters[id] = r.userLimiters[id]
		} else {
			updatedLimiters[id] = newLimiter(user.SpeedLimitMbps)
		}
	}
	r.userLimiters = updatedLimiters
}

func (r *Registry) revokeLocked(users map[int64]struct{}, sources map[int64]map[string]struct{}) []func() {
	callbacks := make([]func(), 0)
	for _, session := range r.sessions {
		_, wholeUser := users[session.userID]
		_, source := sources[session.userID][session.sourceIP]
		if (!wholeUser && !source) || session.revoked {
			continue
		}
		session.revoked = true
		session.cancel()
		if session.interrupt != nil {
			callbacks = append(callbacks, session.interrupt)
		}
	}
	return callbacks
}

func (r *Registry) enforceConnectorsLocked() ([]func(), map[int64]struct{}) {
	type sourceAge struct {
		source string
		first  uint64
	}
	callbacks := make([]func(), 0)
	affected := make(map[int64]struct{})
	for userID, sources := range r.sources {
		user, exists := r.users[userID]
		if !exists {
			continue
		}
		limit := effectiveLimit(user.ConnectorLimit, r.nodeConnectors)
		if limit <= 0 || len(sources) <= limit {
			continue
		}
		ages := make(map[string]uint64, len(sources))
		for _, session := range r.sessions {
			if session.userID != userID || session.revoked {
				continue
			}
			first, seen := ages[session.sourceIP]
			if !seen || session.id < first {
				ages[session.sourceIP] = session.id
			}
		}
		ordered := make([]sourceAge, 0, len(ages))
		for source, first := range ages {
			ordered = append(ordered, sourceAge{source: source, first: first})
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].first == ordered[j].first {
				return ordered[i].source < ordered[j].source
			}
			return ordered[i].first < ordered[j].first
		})
		blocked := make(map[string]struct{})
		for _, item := range ordered[limit:] {
			blocked[item.source] = struct{}{}
		}
		if len(blocked) > 0 {
			affected[userID] = struct{}{}
			callbacks = append(callbacks, r.revokeLocked(nil, map[int64]map[string]struct{}{userID: blocked})...)
		}
	}
	return callbacks, affected
}

func (r *Registry) userID(email string) (int64, error) {
	prefix := fmt.Sprintf("sspanel-node-%d-user-", r.nodeID)
	if !strings.HasPrefix(email, prefix) {
		return 0, errors.New("unexpected user identity")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(email, prefix), 10, 64)
	if err != nil || id <= 0 || email != prefix+strconv.FormatInt(id, 10) {
		return 0, errors.New("malformed user identity")
	}
	return id, nil
}

func (r *Registry) reject(reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rejectLockedError(reason)
}

func (r *Registry) rejectLocked(reason string) { r.rejected[reason]++ }

func (r *Registry) rejectLockedError(reason string) error {
	r.rejectLocked(reason)
	return fmt.Errorf("session rejected: %s", reason)
}

func (l *lease) BindInterrupt(callback func()) {
	l.registry.mu.Lock()
	l.interrupt = callback
	revoked := l.revoked
	l.registry.mu.Unlock()
	if revoked && callback != nil {
		callback()
	}
}

func (l *lease) Rebind(_ context.Context, value string) error {
	sourceIP, err := policy.CanonicalIP(value)
	if err != nil {
		return l.registry.reject("source")
	}
	r := l.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || l.revoked {
		return r.rejectLockedError("authorization")
	}
	if sourceIP == l.sourceIP {
		return nil
	}
	user, exists := r.users[l.userID]
	if !exists {
		return r.rejectLockedError("authorization")
	}
	if contains(user.DisconnectIPs, sourceIP) {
		return r.rejectLockedError("disconnect_ip")
	}
	limit := effectiveLimit(user.ConnectorLimit, r.nodeConnectors)
	if limit > 0 && r.sources[l.userID][sourceIP] == 0 {
		sourcesAfterDetach := len(r.sources[l.userID])
		if r.sources[l.userID][l.sourceIP] == 1 {
			sourcesAfterDetach--
		}
		if sourcesAfterDetach >= limit {
			return r.rejectLockedError("connector_limit")
		}
	}
	r.removeSourceLocked(l.userID, l.sourceIP)
	if r.sources[l.userID] == nil {
		r.sources[l.userID] = make(map[string]int)
	}
	r.sources[l.userID][sourceIP]++
	l.sourceIP = sourceIP
	return nil
}

func (l *lease) Wait(_ context.Context, _ sessioncontrol.Direction, bytes int) error {
	if bytes <= 0 {
		return nil
	}
	if err := wait(l.ctx, l.userRate, bytes); err != nil {
		return err
	}
	if err := wait(l.ctx, l.nodeRate, bytes); err != nil {
		return err
	}
	return nil
}

func (l *lease) Close() {
	l.once.Do(func() {
		l.cancel()
		r := l.registry
		r.mu.Lock()
		delete(r.sessions, l.id)
		if l.kind == TCP {
			if r.tcp > 0 {
				r.tcp--
			}
			if r.userTCP[l.userID] > 1 {
				r.userTCP[l.userID]--
			} else {
				delete(r.userTCP, l.userID)
			}
		} else {
			if r.xudp > 0 {
				r.xudp--
			}
			if r.userXUDP[l.userID] > 1 {
				r.userXUDP[l.userID]--
			} else {
				delete(r.userXUDP, l.userID)
			}
		}
		r.removeSourceLocked(l.userID, l.sourceIP)
		r.mu.Unlock()
	})
}

func (r *Registry) removeSourceLocked(userID int64, sourceIP string) {
	if r.sources[userID][sourceIP] > 1 {
		r.sources[userID][sourceIP]--
	} else {
		delete(r.sources[userID], sourceIP)
	}
	if len(r.sources[userID]) == 0 {
		delete(r.sources, userID)
	}
}

func wait(ctx context.Context, limiter *rate.Limiter, bytes int) error {
	if limiter == nil {
		return nil
	}
	for bytes > 0 {
		chunk := bytes
		if chunk > limiter.Burst() {
			chunk = limiter.Burst()
		}
		if err := limiter.WaitN(ctx, chunk); err != nil {
			return err
		}
		bytes -= chunk
	}
	return nil
}

func newLimiter(mbps float64) *rate.Limiter {
	if mbps <= 0 {
		return nil
	}
	bytesPerSecond := mbps * 1_000_000 / 8
	burst := int(bytesPerSecond)
	if burst < 64*1024 {
		burst = 64 * 1024
	}
	return rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
}

func userMap(users []model.User) map[int64]model.User {
	result := make(map[int64]model.User, len(users))
	for _, user := range users {
		result[user.ID] = user
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func userPolicyWithoutDisconnectChanged(left, right model.User) bool {
	left.DisconnectIPs = nil
	right.DisconnectIPs = nil
	return !reflect.DeepEqual(left, right)
}

func requestKind(network string) (Kind, error) {
	switch network {
	case "tcp":
		return TCP, nil
	case "udp":
		return XUDP, nil
	default:
		return 0, errors.New("unsupported network")
	}
}

func effectiveLimit(user, node int) int {
	if user <= 0 {
		return node
	}
	if node <= 0 || user < node {
		return user
	}
	return node
}
