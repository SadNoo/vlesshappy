package policy

import (
	"reflect"
	"testing"
)

func TestParseIPRulesNormalizesAndDeduplicates(t *testing.T) {
	actual, err := ParseIPRules("192.0.2.1, ::ffff:192.0.2.1, 2001:db8::/32")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"192.0.2.1/32", "2001:db8::/32"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("got %#v, want %#v", actual, expected)
	}

	mapped, err := ParseIPRules("::ffff:198.51.100.0/120")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mapped, []string{"198.51.100.0/24"}) {
		t.Fatalf("mapped prefix = %#v", mapped)
	}
}

func TestParsePorts(t *testing.T) {
	actual, err := ParsePorts("443, 1000-2000;53")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"1000-2000", "443", "53"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("got %#v, want %#v", actual, expected)
	}
	for _, invalid := range []string{"0", "65536", "200-100", "1-2-3", "dns"} {
		if _, err := ParsePorts(invalid); err == nil {
			t.Fatalf("accepted invalid port rule %q", invalid)
		}
	}
}

func TestSourceIPCanonicalization(t *testing.T) {
	actual, err := ParseSourceIPs("::ffff:203.0.113.7,203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, []string{"203.0.113.7"}) {
		t.Fatalf("got %#v", actual)
	}
}
