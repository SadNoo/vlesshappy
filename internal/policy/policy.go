package policy

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

func ParseIPRules(value string) ([]string, error) {
	parts := fields(value)
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		var normalized string
		if prefix, err := netip.ParsePrefix(part); err == nil {
			address := prefix.Addr()
			bits := prefix.Bits()
			if address.Is4In6() {
				bits -= 96
			}
			prefix = netip.PrefixFrom(address.Unmap(), bits).Masked()
			normalized = prefix.String()
		} else if address, err := netip.ParseAddr(part); err == nil {
			address = address.Unmap()
			normalized = netip.PrefixFrom(address, address.BitLen()).String()
		} else {
			return nil, fmt.Errorf("invalid IP or CIDR rule %q", part)
		}
		if _, ok := seen[normalized]; !ok {
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result, nil
}

func ParseSourceIPs(value string) ([]string, error) {
	parts := fields(value)
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		address, err := netip.ParseAddr(strings.Trim(part, "[]"))
		if err != nil {
			return nil, fmt.Errorf("invalid source IP %q", part)
		}
		normalized := address.Unmap().String()
		if _, ok := seen[normalized]; !ok {
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result, nil
}

func ParsePorts(value string) ([]string, error) {
	parts := fields(value)
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		bounds := strings.Split(part, "-")
		if len(bounds) > 2 {
			return nil, fmt.Errorf("invalid port rule %q", part)
		}
		first, err := port(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("invalid port rule %q", part)
		}
		last := first
		if len(bounds) == 2 {
			last, err = port(bounds[1])
			if err != nil || last < first {
				return nil, fmt.Errorf("invalid port rule %q", part)
			}
		}
		normalized := strconv.Itoa(first)
		if first != last {
			normalized += "-" + strconv.Itoa(last)
		}
		if _, ok := seen[normalized]; !ok {
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result, nil
}

func CanonicalIP(value string) (string, error) {
	address, err := netip.ParseAddr(strings.Trim(value, "[]"))
	if err != nil {
		return "", err
	}
	return address.Unmap().String(), nil
}

func IPBlocked(rules []string, value string) bool {
	address, err := netip.ParseAddr(strings.Trim(value, "[]"))
	if err != nil {
		return true
	}
	address = address.Unmap()
	for _, rule := range rules {
		prefix, err := netip.ParsePrefix(rule)
		if err == nil && prefix.Contains(address) {
			return true
		}
	}
	return false
}

func PortBlocked(rules []string, value int) bool {
	for _, rule := range rules {
		bounds := strings.Split(rule, "-")
		first, err := strconv.Atoi(bounds[0])
		if err != nil {
			return true
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(bounds[1])
			if err != nil {
				return true
			}
		}
		if value >= first && value <= last {
			return true
		}
	}
	return false
}

func fields(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})
}

func port(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("port out of range")
	}
	return n, nil
}
