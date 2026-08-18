// Package sessioncontrol exposes the two small data-plane hooks required by
// vlesshappy. Protocol and cryptographic code stays independent of panel logic.
package sessioncontrol

import (
	"context"
	"sync"
)

type Direction uint8

const (
	Uplink Direction = iota + 1
	Downlink
)

type Request struct {
	InboundTag        string
	Email             string
	SourceIP          string
	Network           string
	DestinationHost   string
	DestinationPort   uint16
	DestinationDomain bool
}

type Decision struct {
	// ResolvedAddress pins a domain request to an address that the controller
	// already resolved and authorized. Empty keeps the original destination.
	ResolvedAddress string
	Lease           Lease
}

type Lease interface {
	BindInterrupt(func())
	Rebind(context.Context, string) error
	Wait(context.Context, Direction, int) error
	Close()
}

type Controller interface {
	AcquireHandshake(context.Context, Request) (func(), error)
	Open(context.Context, Request) (Decision, error)
}

var state struct {
	sync.RWMutex
	controller Controller
}

// Register installs the single process-wide controller. Xray-core supports
// multiple instances, but vlesshappy intentionally runs one node per process.
func Register(controller Controller) (func(), error) {
	state.Lock()
	defer state.Unlock()
	if state.controller != nil {
		return nil, ErrControllerRegistered
	}
	state.controller = controller
	var once sync.Once
	return func() {
		once.Do(func() {
			state.Lock()
			if state.controller == controller {
				state.controller = nil
			}
			state.Unlock()
		})
	}, nil
}

func AcquireHandshake(ctx context.Context, request Request) (func(), error) {
	state.RLock()
	controller := state.controller
	state.RUnlock()
	if controller == nil {
		return func() {}, nil
	}
	return controller.AcquireHandshake(ctx, request)
}

func Open(ctx context.Context, request Request) (Decision, error) {
	state.RLock()
	controller := state.controller
	state.RUnlock()
	if controller == nil {
		return Decision{}, nil
	}
	return controller.Open(ctx, request)
}
