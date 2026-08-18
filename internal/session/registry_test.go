package session

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/xtls/xray-core/common/sessioncontrol"

	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/model"
)

func TestLimitsAndTargetedSourceRevocation(t *testing.T) {
	snapshot := model.Snapshot{
		Node:  model.Node{ID: 15, ConnectorLimit: 2},
		Users: []model.User{{ID: 42, ConnectorLimit: 2}},
	}
	registry := New(snapshot, config.Config{Limits: config.Limits{
		TCPPerUser: 2, TCPGlobal: 2, ConcurrentHandshakes: 1,
		XUDPPerUser: 1, XUDPGlobal: 1, DNSResolveTimeoutSeconds: 1,
	}})
	request := sessioncontrol.Request{
		InboundTag: inboundTag, Email: "sspanel-node-15-user-42", SourceIP: "192.0.2.1",
		Network: "tcp", DestinationHost: "198.51.100.1", DestinationPort: 443,
	}
	first, err := registry.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Open(context.Background(), request); err == nil {
		t.Fatal("per-user TCP limit was bypassed")
	}

	var interrupted atomic.Int32
	first.Lease.BindInterrupt(func() { interrupted.Add(1) })
	snapshot.Users[0].DisconnectIPs = []string{"192.0.2.1"}
	registry.Update(snapshot)
	if interrupted.Load() != 1 {
		t.Fatal("disconnect_ip did not revoke the matching source")
	}
	first.Lease.Close()
	second.Lease.Close()
}

func TestXUDPSourceRebindHonorsConnectorLimit(t *testing.T) {
	registry := New(model.Snapshot{
		Node: model.Node{ID: 15}, Users: []model.User{{ID: 42, ConnectorLimit: 1}},
	}, config.Config{Limits: config.Limits{
		TCPPerUser: 1, TCPGlobal: 1, ConcurrentHandshakes: 1,
		XUDPPerUser: 1, XUDPGlobal: 1, DNSResolveTimeoutSeconds: 1,
	}})
	decision, err := registry.Open(context.Background(), sessioncontrol.Request{
		InboundTag: inboundTag, Email: "sspanel-node-15-user-42", SourceIP: "192.0.2.1",
		Network: "udp", DestinationHost: "198.51.100.1", DestinationPort: 53,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := decision.Lease.Rebind(context.Background(), "192.0.2.2"); err != nil {
		t.Fatal(err)
	}
	online := registry.Online()[42]
	if len(online) != 1 || online["192.0.2.2"].IsZero() {
		t.Fatal("XUDP source rebind did not replace the connector source")
	}
	tcp, err := registry.Open(context.Background(), sessioncontrol.Request{
		InboundTag: inboundTag, Email: "sspanel-node-15-user-42", SourceIP: "192.0.2.2",
		Network: "tcp", DestinationHost: "198.51.100.2", DestinationPort: 443,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := decision.Lease.Rebind(context.Background(), "192.0.2.3"); err == nil {
		t.Fatal("XUDP source rebind bypassed the connector limit")
	}
	tcp.Lease.Close()
	decision.Lease.Close()
}

func TestHandshakeLimit(t *testing.T) {
	registry := New(model.Snapshot{Node: model.Node{ID: 15}}, config.Config{Limits: config.Limits{
		TCPPerUser: 1, TCPGlobal: 1, ConcurrentHandshakes: 1,
		XUDPPerUser: 1, XUDPGlobal: 1, DNSResolveTimeoutSeconds: 1,
	}})
	release, err := registry.AcquireHandshake(context.Background(), sessioncontrol.Request{InboundTag: inboundTag})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AcquireHandshake(context.Background(), sessioncontrol.Request{InboundTag: inboundTag}); err == nil {
		t.Fatal("concurrent handshake limit was bypassed")
	}
	release()
}
