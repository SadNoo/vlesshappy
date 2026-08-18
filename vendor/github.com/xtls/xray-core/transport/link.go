package transport

import (
	"context"

	"github.com/xtls/xray-core/common/buf"
)

// Link is a utility for connecting between an inbound and an outbound proxy handler.
type Link struct {
	Reader buf.Reader
	Writer buf.Writer
	// Done releases optional platform session state. Generic users may leave it nil.
	Done func()
	// Rebind updates optional platform state when a logical XUDP session migrates.
	Rebind func(context.Context) error
}
