package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/DanyPops/shutapp/internal/app"
	"github.com/DanyPops/shutapp/internal/infra/whatsapp"
	iface "github.com/DanyPops/shutapp/internal/interface"
	"go.mau.fi/whatsmeow/types"
)

func main() {
	ctx := context.Background()

	// Load configuration
	cfg, err := app.LoadTargetConfig("target.yaml")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Init WhatsApp client
	client, err := whatsapp.NewClient(ctx, "session.db", "info")
	if err != nil {
		log.Fatalf("wa client: %v", err)
	}

	// Connect WhatsApp client (handles QR automatically)
	if err := client.Connect(ctx); err != nil {
		log.Fatalf("connect: %v", err)
	}

	// Resolve target JID
	var userPNJID *types.JID

	const maxWait = 15 * time.Second
	const interval = 1 * time.Second

	start := time.Now()
	for {
		userPNJID, err = client.ResolveUserByPhone(ctx, cfg.Phone)
		if err == nil {
			break
		}

		if time.Since(start) > maxWait {
			log.Fatalf("resolve target phone (after retries): %v", err)
		}
		log.Println("🔄 Waiting for contact sync...")

		time.Sleep(interval)
	}

	// Resolve target JID (formerly in NewClient)
	jid := types.NewJID(strings.TrimPrefix(cfg.Phone, "+"), types.DefaultUserServer)
	client.SetTargetJID(jid)

	// Resolve group
	gjid, err := client.ResolveGroupByName(ctx, cfg.Group)
	if err != nil {
		log.Fatalf("resolve group: %v", err)
	}

	// Resolver
	resolver := app.NewResolver(
		*userPNJID,
	)
	resolver.ResolveGroup(ctx, *gjid)

	// Listener
	l := iface.NewListener(ctx, client, resolver)
	_ = l.Start()

	// Batch Deleter
	iface.StartBatchDeleter(ctx, client, resolver.MatchChan())

	select {}
}
