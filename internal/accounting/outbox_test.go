package accounting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOutboxRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, 15, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := NewBatch(15, 7, 1.5, []Entry{
		{UserID: 42, Uplink: 10, Downlink: 20},
		{UserID: 1, Uplink: 5},
	}, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(batch); err != nil {
		t.Fatal(err)
	}
	files, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !reflect.DeepEqual(files[0].Batch, batch) {
		t.Fatalf("round trip = %#v", files)
	}
	if err := store.Remove(files[0].Path); err != nil {
		t.Fatal(err)
	}
	if files, err := store.List(); err != nil || len(files) != 0 {
		t.Fatalf("after remove = %#v, %v", files, err)
	}
}

func TestOutboxRejectsTamperingAndIncompleteWrite(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, 15, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := NewBatch(15, 1, 1, []Entry{{UserID: 1, Uplink: 1}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value["traffic_rate"] = 2
	data, _ = json.Marshal(value)
	if err := os.WriteFile(filepath.Join(store.dir, "00000000000000000001-test.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("tampered batch was accepted")
	}

	dir2 := t.TempDir()
	store2, err := Open(dir2, 15, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store2.dir, "pending.json.tmp"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store2.List(); err == nil {
		t.Fatal("incomplete batch was ignored")
	}
}

func TestOutboxCapacity(t *testing.T) {
	store, err := Open(t.TempDir(), 15, 200)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := NewBatch(15, 1, 1, []Entry{{UserID: 1, Uplink: 1}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(batch); err == nil {
		t.Fatal("oversized batch was accepted")
	}
}

func TestWasPublished(t *testing.T) {
	if WasPublished(nil) || WasPublished(os.ErrPermission) {
		t.Fatal("ordinary errors reported as published")
	}
	if !WasPublished(&saveError{published: true, err: os.ErrPermission}) {
		t.Fatal("published save error was not recognized")
	}
}
