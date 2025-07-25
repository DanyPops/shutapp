package app

import (
	"context"
	"log"
	"sync"

	"go.mau.fi/whatsmeow/types"

	"github.com/DanyPops/shutapp/internal/domain/entity"
)

type Resolver struct {
	userJID  types.JID
	groupJID *types.JID

	ready     bool
	mu        sync.RWMutex
	matchChan chan entity.Message
}

func NewResolver(userJID types.JID) *Resolver {
	return &Resolver{
		userJID:   userJID,
		matchChan: make(chan entity.Message, 100),
	}
}

func (r *Resolver) ResolveGroup(ctx context.Context, groupJID types.JID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ready {
		return
	}
	r.groupJID = &groupJID
	r.ready = true
	log.Println("🟢 Target ready; deletions active.")
}

func (r *Resolver) MatchChan() <-chan entity.Message {
	return r.matchChan
}

func (r *Resolver) Hit(msg entity.Message) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.ready || r.groupJID == nil {
		return false
	}

	if msg.GroupID != r.groupJID.String() {
		return false
	}

	actualJID, err := types.ParseJID(msg.Sender)
	if err != nil {
		return false
	}

	match := r.userJID.User == actualJID.User &&
		r.userJID.Server == actualJID.Server

	return match
}

func (r *Resolver) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}
