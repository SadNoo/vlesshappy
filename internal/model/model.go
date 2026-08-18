package model

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type Node struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	TrafficRate      float64 `json:"traffic_rate"`
	Class            int     `json:"class"`
	Group            int     `json:"group"`
	SpeedLimitMbps   float64 `json:"speed_limit_mbps"`
	ConnectorLimit   int     `json:"connector_limit"`
	Bandwidth        int64   `json:"-"`
	BandwidthLimit   int64   `json:"-"`
	PublicHost       string  `json:"public_host"`
	PublicPort       int     `json:"public_port"`
	ServerName       string  `json:"server_name"`
	Target           string  `json:"target"`
	RealityPublicKey string  `json:"reality_public_key"`
	ShortID          string  `json:"short_id"`
	Fingerprint      string  `json:"fingerprint"`
	Flow             string  `json:"flow"`
	Transport        string  `json:"transport"`
	MinClientVersion string  `json:"min_client_version"`
}

type User struct {
	ID             int64    `json:"id"`
	UUID           string   `json:"uuid"`
	SpeedLimitMbps float64  `json:"speed_limit_mbps"`
	ConnectorLimit int      `json:"connector_limit"`
	ForbiddenIPs   []string `json:"forbidden_ips,omitempty"`
	ForbiddenPorts []string `json:"forbidden_ports,omitempty"`
	DisconnectIPs  []string `json:"disconnect_ips,omitempty"`
}

type Snapshot struct {
	Node     Node      `json:"node"`
	Users    []User    `json:"users"`
	LoadedAt time.Time `json:"-"`
	Hash     string    `json:"-"`
}

func UserEmail(nodeID, userID int64) string {
	return fmt.Sprintf("sspanel-node-%d-user-%d", nodeID, userID)
}

func UUIDv3(userID int64, password string) string {
	// RFC 4122 DNS namespace. MD5 is required by UUIDv3 compatibility, not used as a password hash.
	namespace := [16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	h := md5.New()
	_, _ = h.Write(namespace[:])
	_, _ = h.Write([]byte(fmt.Sprintf("%d|%s", userID, password)))
	sum := h.Sum(nil)
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(sum[0:4]), hex.EncodeToString(sum[4:6]), hex.EncodeToString(sum[6:8]), hex.EncodeToString(sum[8:10]), hex.EncodeToString(sum[10:16]))
}

func (s *Snapshot) Finalize() error {
	sort.Slice(s.Users, func(i, j int) bool { return s.Users[i].ID < s.Users[j].ID })
	for i := range s.Users {
		sort.Strings(s.Users[i].ForbiddenIPs)
		sort.Strings(s.Users[i].ForbiddenPorts)
		sort.Strings(s.Users[i].DisconnectIPs)
	}
	data, err := json.Marshal(struct {
		Node  Node   `json:"node"`
		Users []User `json:"users"`
	}{s.Node, s.Users})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	s.Hash = hex.EncodeToString(sum[:])
	return nil
}

func (s Snapshot) UserByID(id int64) (User, bool) {
	i := sort.Search(len(s.Users), func(i int) bool { return s.Users[i].ID >= id })
	if i < len(s.Users) && s.Users[i].ID == id {
		return s.Users[i], true
	}
	return User{}, false
}
