package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow/types"

	"github.com/DanyPops/shutapp/internal/domain/entity"
)

type Resolver struct {
	groupName string
	userJID   types.JID
	groupJID  *types.JID

	ready     bool
	mu        sync.RWMutex
	onReady   func()
	matchChan chan entity.Message
}

func NewResolver(groupName string, userJID types.JID, onReady func()) *Resolver {
	return &Resolver{
		groupName: strings.ToLower(groupName),
		userJID:   userJID,
		onReady:   onReady,
		matchChan: make(chan entity.Message, 100),
	}
}

func (r *Resolver) ResolveGroup(ctx context.Context, groupJID types.JID, actualName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ready {
		return
	}
	r.groupJID = &groupJID
	r.ready = true
	if r.onReady != nil {
		go r.onReady()
	}
}

func (r *Resolver) MatchChan() <-chan entity.Message {
	return r.matchChan
}

func (r *Resolver) Hit(msg entity.Message) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fmt.Println("---- DEBUG: Resolver.Hit ----")
	fmt.Printf("Expected sender (userJID): %s\n", r.userJID.String())
	fmt.Printf("Expected group: %s\n", r.groupJID)
	fmt.Printf("Actual sender: %s\n", msg.Sender)
	fmt.Printf("Actual group: %s\n", msg.GroupID)

	if !r.ready || r.groupJID == nil {
		fmt.Println("→ Resolver not ready or groupJID is nil")
		return false
	}

	expectedJID := r.userJID
	actualJID, err := types.ParseJID(msg.Sender)
	if err != nil {
		fmt.Println("→ Failed to parse actual sender JID:", err)
		return false
	}

	match := msg.GroupID == r.groupJID.String() &&
		expectedJID.User == actualJID.User &&
		expectedJID.Server == actualJID.Server
	fmt.Printf("→ Hit Match: %v\n", match)
	return match
}

func (r *Resolver) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}
