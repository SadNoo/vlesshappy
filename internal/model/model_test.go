package model

import "testing"

func TestUUIDv3MatchesPHPFixture(t *testing.T) {
	fixtures := []struct {
		id       int64
		password string
		expected string
	}{
		{1, "password", "d2b7d951-3d54-3f23-abb8-f752e336b3cd"},
		{42, "example-pass", "9af01be3-7b93-39b9-8fa0-40f7ad54eb71"},
	}
	for _, fixture := range fixtures {
		if actual := UUIDv3(fixture.id, fixture.password); actual != fixture.expected {
			t.Fatalf("UUIDv3(%d) = %q, want %q", fixture.id, actual, fixture.expected)
		}
	}
}

func TestSnapshotFinalizeIsStable(t *testing.T) {
	left := Snapshot{Node: Node{ID: 1}, Users: []User{
		{ID: 2, UUID: "b", ForbiddenIPs: []string{"192.0.2.0/24", "203.0.113.0/24"}},
		{ID: 1, UUID: "a"},
	}}
	right := Snapshot{Node: left.Node, Users: []User{left.Users[1], left.Users[0]}}
	if err := left.Finalize(); err != nil {
		t.Fatal(err)
	}
	if err := right.Finalize(); err != nil {
		t.Fatal(err)
	}
	if left.Hash != right.Hash {
		t.Fatalf("hash differs: %s != %s", left.Hash, right.Hash)
	}
	before := left.Hash
	left.Node.Bandwidth++
	if err := left.Finalize(); err != nil {
		t.Fatal(err)
	}
	if left.Hash != before {
		t.Fatal("dynamic node bandwidth caused an authorization reload hash")
	}
}
