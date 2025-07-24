package iface

import (
	"context"

	"go.mau.fi/whatsmeow/types"

	"github.com/DanyPops/shutapp/internal/app"
	"github.com/DanyPops/shutapp/internal/domain/entity"
	"github.com/DanyPops/shutapp/internal/infra/whatsapp"
)

type Listener struct {
	client   *whatsapp.Client
	resolver *app.Resolver
	ctx      context.Context
}

func NewListener(ctx context.Context, client *whatsapp.Client, resolver *app.Resolver) *Listener {
	return &Listener{ctx: ctx, client: client, resolver: resolver}
}

func (l *Listener) Start() error {
	// Subscribe to infra message feed
	l.client.AddMessageHandler(func(m entity.Message) {
		if l.resolver.Hit(m) {
			chat, _ := types.ParseJID(m.GroupID)
			_ = l.client.Revoke(l.ctx, chat, m.ID)
		}
	})
	return nil
}
