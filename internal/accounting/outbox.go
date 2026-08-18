package accounting

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

type Entry struct {
	UserID   int64 `json:"user_id"`
	Uplink   int64 `json:"uplink"`
	Downlink int64 `json:"downlink"`
}

type Batch struct {
	Version              int     `json:"version"`
	ID                   string  `json:"id"`
	NodeID               int64   `json:"node_id"`
	ConfigurationVersion uint64  `json:"configuration_version"`
	CreatedAt            int64   `json:"created_at"`
	TrafficRate          float64 `json:"traffic_rate"`
	Entries              []Entry `json:"entries"`
	PayloadSHA256        string  `json:"payload_sha256"`
}

type Store struct {
	dir      string
	nodeID   int64
	maxBytes int64
	hooks    *storeHooks
}

// storeHooks is an unexported fault-injection seam. Production stores leave it
// nil, so runtime behavior always uses the operating system directly.
type storeHooks struct {
	write   func(*os.File, []byte) (int, error)
	sync    func(*os.File) error
	rename  func(string, string) error
	syncDir func(string) error
}

type saveError struct {
	published bool
	err       error
}

func (e *saveError) Error() string { return e.err.Error() }
func (e *saveError) Unwrap() error { return e.err }

// WasPublished reports that the immutable batch is visible in the outbox even
// though the directory fsync failed. Callers must not restore the same counters.
func WasPublished(err error) bool {
	var target *saveError
	return errors.As(err, &target) && target.published
}

func Open(stateDir string, nodeID, maxBytes int64) (*Store, error) {
	dir := filepath.Join(stateDir, "outbox")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create outbox: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure outbox: %w", err)
	}
	store := &Store{dir: dir, nodeID: nodeID, maxBytes: maxBytes}
	if _, err := store.List(); err != nil {
		return nil, err
	}
	return store, nil
}

func NewBatch(nodeID int64, configurationVersion uint64, rate float64, entries []Entry, now time.Time) (Batch, error) {
	batch := Batch{
		Version:              SchemaVersion,
		ID:                   newUUID(),
		NodeID:               nodeID,
		ConfigurationVersion: configurationVersion,
		CreatedAt:            now.Unix(),
		TrafficRate:          rate,
		Entries:              append([]Entry(nil), entries...),
	}
	sort.Slice(batch.Entries, func(i, j int) bool { return batch.Entries[i].UserID < batch.Entries[j].UserID })
	if err := batch.validate(false); err != nil {
		return Batch{}, err
	}
	hash, err := batch.hash()
	if err != nil {
		return Batch{}, err
	}
	batch.PayloadSHA256 = hash
	return batch, nil
}

func (b Batch) Empty() bool { return len(b.Entries) == 0 }

func (b Batch) RawTotal() (int64, error) {
	var total int64
	for _, entry := range b.Entries {
		if entry.Uplink > math.MaxInt64-entry.Downlink || total > math.MaxInt64-entry.Uplink-entry.Downlink {
			return 0, errors.New("traffic total overflows int64")
		}
		total += entry.Uplink + entry.Downlink
	}
	return total, nil
}

func (s *Store) Save(batch Batch) error {
	if batch.NodeID != s.nodeID {
		return errors.New("outbox batch has wrong node_id")
	}
	if err := batch.validate(true); err != nil {
		return err
	}
	data, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	used, err := s.Size()
	if err != nil {
		return err
	}
	if int64(len(data)) > s.maxBytes-used {
		return errors.New("outbox capacity exceeded")
	}
	base := fmt.Sprintf("%020d-%s.json", time.Now().UnixNano(), batch.ID)
	finalPath := filepath.Join(s.dir, base)
	tempPath := finalPath + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create outbox batch: %w", err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			// Keep a failed temp file as evidence. Startup refuses it instead of silently losing traffic.
			_ = os.Chmod(tempPath, 0o400)
		}
	}()
	write := file.Write
	if s.hooks != nil && s.hooks.write != nil {
		write = func(data []byte) (int, error) { return s.hooks.write(file, data) }
	}
	if written, err := write(data); err != nil || written != len(data) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return fmt.Errorf("write outbox batch: %w", err)
	}
	syncFile := file.Sync
	if s.hooks != nil && s.hooks.sync != nil {
		syncFile = func() error { return s.hooks.sync(file) }
	}
	if err := syncFile(); err != nil {
		return fmt.Errorf("sync outbox batch: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close outbox batch: %w", err)
	}
	rename := os.Rename
	if s.hooks != nil && s.hooks.rename != nil {
		rename = s.hooks.rename
	}
	if err := rename(tempPath, finalPath); err != nil {
		return fmt.Errorf("publish outbox batch: %w", err)
	}
	ok = true
	syncDirectory := syncDir
	if s.hooks != nil && s.hooks.syncDir != nil {
		syncDirectory = s.hooks.syncDir
	}
	if err := syncDirectory(s.dir); err != nil {
		return &saveError{published: true, err: err}
	}
	return nil
}

func (s *Store) List() ([]BatchFile, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read outbox: %w", err)
	}
	files := make([]BatchFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".tmp") {
			return nil, fmt.Errorf("incomplete outbox file %q requires operator review", name)
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			return nil, fmt.Errorf("unexpected outbox entry %q", name)
		}
		path := filepath.Join(s.dir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe outbox entry %q", name)
		}
		batch, err := readBatch(path)
		if err != nil {
			return nil, fmt.Errorf("read outbox %q: %w", name, err)
		}
		if batch.NodeID != s.nodeID {
			return nil, fmt.Errorf("outbox %q belongs to node %d", name, batch.NodeID)
		}
		files = append(files, BatchFile{Path: path, Batch: batch})
	}
	sort.Slice(files, func(i, j int) bool { return filepath.Base(files[i].Path) < filepath.Base(files[j].Path) })
	return files, nil
}

type BatchFile struct {
	Path  string
	Batch Batch
}

func (s *Store) Remove(path string) error {
	clean := filepath.Clean(path)
	if filepath.Dir(clean) != s.dir || !strings.HasSuffix(clean, ".json") {
		return errors.New("refusing to remove path outside outbox")
	}
	if err := os.Remove(clean); err != nil {
		return fmt.Errorf("remove outbox batch: %w", err)
	}
	return syncDir(s.dir)
}

func (s *Store) Size() (int64, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		if info.Size() > math.MaxInt64-total {
			return 0, errors.New("outbox size overflows int64")
		}
		total += info.Size()
	}
	return total, nil
}

func readBatch(path string) (Batch, error) {
	var batch Batch
	data, err := os.ReadFile(path)
	if err != nil {
		return batch, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&batch); err != nil {
		return batch, err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return batch, errors.New("outbox contains trailing JSON")
	}
	if err := batch.validate(true); err != nil {
		return batch, err
	}
	return batch, nil
}

func (b Batch) validate(checkHash bool) error {
	if b.Version != SchemaVersion || b.NodeID <= 0 || b.ConfigurationVersion == 0 || b.CreatedAt <= 0 {
		return errors.New("invalid outbox metadata")
	}
	if !validUUID(b.ID) {
		return errors.New("invalid batch UUID")
	}
	if math.IsNaN(b.TrafficRate) || math.IsInf(b.TrafficRate, 0) || b.TrafficRate <= 0 || b.TrafficRate > 10000 {
		return errors.New("invalid traffic rate")
	}
	lastID := int64(0)
	for _, entry := range b.Entries {
		if entry.UserID <= lastID || entry.Uplink < 0 || entry.Downlink < 0 || entry.Uplink > math.MaxInt64-entry.Downlink {
			return errors.New("invalid or unsorted traffic entries")
		}
		if entry.Uplink == 0 && entry.Downlink == 0 {
			return errors.New("zero traffic entry")
		}
		lastID = entry.UserID
	}
	if checkHash {
		expected, err := b.hash()
		if err != nil {
			return err
		}
		if !bytes.Equal([]byte(expected), []byte(b.PayloadSHA256)) {
			return errors.New("outbox payload hash mismatch")
		}
	}
	return nil
}

func (b Batch) hash() (string, error) {
	b.PayloadSHA256 = ""
	data, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func newUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open outbox directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync outbox directory: %w", err)
	}
	return nil
}
