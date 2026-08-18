package dispatcher

import (
	"context"
	"sync"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/common/sessioncontrol"
	"github.com/xtls/xray-core/transport"
)

type controlledSession struct {
	lease sessioncontrol.Lease
	once  sync.Once
	stop  func() bool
}

func openControlledSession(ctx context.Context, destination net.Destination) (net.Destination, *controlledSession, error) {
	inbound := session.InboundFromContext(ctx)
	request := sessioncontrol.Request{
		Network:         destination.Network.SystemString(),
		DestinationHost: destination.Address.String(),
		DestinationPort: uint16(destination.Port),
	}
	request.DestinationDomain = destination.Address.Family().IsDomain()
	if inbound != nil {
		request.InboundTag = inbound.Tag
		if inbound.User != nil {
			request.Email = inbound.User.Email
		}
		if inbound.Source.Address != nil {
			request.SourceIP = inbound.Source.Address.String()
		}
	}
	decision, err := sessioncontrol.Open(ctx, request)
	if err != nil {
		return destination, nil, err
	}
	if decision.ResolvedAddress != "" {
		destination.Address = net.ParseAddress(decision.ResolvedAddress)
	}
	if decision.Lease == nil {
		return destination, nil, nil
	}
	// Splice copy would bypass the controlled readers/writers and therefore the
	// shared user/node limiter. Disable it only for controlled panel sessions.
	if inbound != nil {
		inbound.CanSpliceCopy = 3
	}
	controlled := &controlledSession{lease: decision.Lease}
	controlled.stop = context.AfterFunc(ctx, controlled.close)
	return destination, controlled, nil
}

func (s *controlledSession) close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.stop != nil {
			s.stop()
		}
		s.lease.Close()
	})
}

func (s *controlledSession) bind(inbound, outbound *transport.Link) {
	if s == nil {
		return
	}
	s.lease.BindInterrupt(func() {
		common.Interrupt(inbound.Reader)
		common.Interrupt(inbound.Writer)
		common.Interrupt(outbound.Reader)
		common.Interrupt(outbound.Writer)
	})
	outbound.Reader = &controlledReader{Reader: outbound.Reader, session: s, direction: sessioncontrol.Uplink}
	outbound.Writer = &controlledWriter{Writer: outbound.Writer, session: s, direction: sessioncontrol.Downlink}
	inbound.Done = s.close
	inbound.Rebind = s.rebind
}

func (s *controlledSession) rebind(ctx context.Context) error {
	inbound := session.InboundFromContext(ctx)
	if inbound == nil || inbound.Source.Address == nil {
		return sessioncontrol.ErrSourceUnavailable
	}
	return s.lease.Rebind(ctx, inbound.Source.Address.String())
}

func (s *controlledSession) bindDirect(link *transport.Link) {
	if s == nil {
		return
	}
	s.lease.BindInterrupt(func() {
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
	})
	link.Reader = &controlledReader{Reader: link.Reader, session: s, direction: sessioncontrol.Uplink}
	link.Writer = &controlledWriter{Writer: link.Writer, session: s, direction: sessioncontrol.Downlink}
}

type controlledReader struct {
	buf.Reader
	session   *controlledSession
	direction sessioncontrol.Direction
}

func (r *controlledReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	mb, err := r.Reader.ReadMultiBuffer()
	if len(mb) != 0 {
		if waitErr := r.session.lease.Wait(context.Background(), r.direction, int(mb.Len())); waitErr != nil {
			buf.ReleaseMulti(mb)
			return nil, waitErr
		}
	}
	return mb, err
}

func (r *controlledReader) Interrupt() { common.Interrupt(r.Reader) }

type controlledWriter struct {
	buf.Writer
	session   *controlledSession
	direction sessioncontrol.Direction
}

func (w *controlledWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if len(mb) != 0 {
		if err := w.session.lease.Wait(context.Background(), w.direction, int(mb.Len())); err != nil {
			buf.ReleaseMulti(mb)
			return err
		}
	}
	return w.Writer.WriteMultiBuffer(mb)
}

func (w *controlledWriter) Close() error { return common.Close(w.Writer) }
func (w *controlledWriter) Interrupt()   { common.Interrupt(w.Writer) }
