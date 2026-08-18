package inbound

import (
	"context"

	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/common/sessioncontrol"
)

func acquireHandshake(ctx context.Context) (func(), error) {
	request := sessioncontrol.Request{}
	if inbound := session.InboundFromContext(ctx); inbound != nil {
		request.InboundTag = inbound.Tag
		if inbound.Source.Address != nil {
			request.SourceIP = inbound.Source.Address.String()
		}
	}
	return sessioncontrol.AcquireHandshake(ctx, request)
}
