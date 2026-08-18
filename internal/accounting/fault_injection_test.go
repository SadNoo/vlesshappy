package accounting

import (
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestOutboxFaultInjectionStages(t *testing.T) {
	batch, err := NewBatch(15, 1, 1, []Entry{{UserID: 1, Uplink: 64}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		hooks     storeHooks
		published bool
	}{
		{name: "partial-write", hooks: storeHooks{write: func(file *os.File, data []byte) (int, error) {
			n, _ := file.Write(data[:len(data)/2])
			return n, io.ErrShortWrite
		}}},
		{name: "disk-full", hooks: storeHooks{write: func(*os.File, []byte) (int, error) { return 0, syscall.ENOSPC }}},
		{name: "file-fsync", hooks: storeHooks{sync: func(*os.File) error { return errors.New("injected fsync failure") }}},
		{name: "rename", hooks: storeHooks{rename: func(string, string) error { return errors.New("injected rename failure") }}},
		{name: "directory-fsync", published: true, hooks: storeHooks{syncDir: func(string) error { return errors.New("injected directory fsync failure") }}},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			store, err := Open(t.TempDir(), 15, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			store.hooks = &fixture.hooks
			err = store.Save(batch)
			if err == nil {
				t.Fatal("injected failure was ignored")
			}
			if WasPublished(err) != fixture.published {
				t.Fatalf("published=%v, want %v", WasPublished(err), fixture.published)
			}
		})
	}
}
