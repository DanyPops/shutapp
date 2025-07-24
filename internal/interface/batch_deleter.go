package iface

import (
	"context"
	"log"
	"time"

	"github.com/DanyPops/shutapp/internal/domain/entity"
	"go.mau.fi/whatsmeow/types"
)

type RevokeClient interface {
	Revoke(ctx context.Context, chat types.JID, msgID string) error
}

func StartBatchDeleter(ctx context.Context, rc RevokeClient, matches <-chan entity.Message) {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		var batch []entity.Message

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-matches:
				batch = append(batch, msg)
			case <-ticker.C:
				if len(batch) == 0 {
					continue
				}
				toDelete := batch
				batch = nil

				for _, m := range toDelete {
					jid, err := types.ParseJID(m.GroupID)
					if err != nil {
						log.Printf("❌ Invalid JID: %v", err)
						continue
					}
					err = rc.Revoke(ctx, jid, m.ID)
					if err != nil {
						log.Printf("❌ Revoke failed: %v", err)
					} else {
						log.Printf("✅ Revoked: %s", m.ID)
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
	}()
}
