package whatsapp

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/DanyPops/shutapp/internal/domain/entity"
)

// Client wraps whatsmeow.Client with our app-facing methods.
type Client struct {
	wc              *whatsmeow.Client
	targetJID       types.JID
	eventSubs       []func(entity.Message)
	addedMsgHandler bool
}

// NewClient opens/creates an SQLite store and initializes the whatsmeow client.
func NewClient(ctx context.Context, dbPath string, logLevel string) (*Client, error) {
	dbLog := waLog.Stdout("DB", "INFO", true)
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), dbLog)
	if err != nil {
		return nil, err
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, err
	}

	clientLog := waLog.Stdout("WA", logLevel, true)
	wc := whatsmeow.NewClient(deviceStore, clientLog)

	c := &Client{
		wc: wc,
	}

	wc.AddEventHandler(c.handleEvent)
	return c, nil
}

func (c *Client) SetTargetJID(jid types.JID) {
	c.targetJID = jid
}

// Connect ensures we are linked (QR on fresh), then establishes websocket.
func (c *Client) Connect(ctx context.Context) error {
	if c.wc.Store.ID == nil {
		// New login: show QR
		qrChan, _ := c.wc.GetQRChannel(ctx)
		if err := c.wc.Connect(); err != nil {
			return err
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render QR
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// Using Stdout by default; writer param omitted to fallback to Stdout
			} else {
				log.Println("Login event:", evt.Event)
			}
			if evt.Event == "success" || evt.Event == "timeout" {
				break
			}
		}
		return nil
	}

	return c.wc.Connect()
}

// Disconnect cleanly closes the websocket.
func (c *Client) Disconnect() {
	c.wc.Disconnect()
}

// Subscribe to incoming messages
func (c *Client) AddMessageHandler(fn func(entity.Message)) {
	if c.addedMsgHandler {
		return
	}
	c.eventSubs = append(c.eventSubs, fn)
	c.addedMsgHandler = true
}

func (c *Client) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Message:
		info := v.Info
		msg := v.Message

		resolvedSender := info.Sender.String()
		pn, err := c.ResolvePN(context.Background(), info.Sender)
		if err == nil {
			log.Printf("🔍 Resolved sender %s → %s\n", info.Sender.String(), pn.String())
			resolvedSender = pn.String()
		} else {
			log.Printf("⚠️  Could not resolve LID to PN: %v\n", err)
		}

		var text string
		if msg.GetConversation() != "" {
			text = msg.GetConversation()
		} else if ext := msg.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}

		em := entity.Message{
			ID:      info.ID,
			Sender:  resolvedSender,
			GroupID: info.Chat.String(),
			Content: text,
		}

		for _, sub := range c.eventSubs {
			sub(em)
		}

	case *events.HistorySync:
		log.Println("📦 Processing history sync...")

		hs := v.Data // *waHistorySync.HistorySyncData

		for _, chat := range hs.Conversations {
			for _, historyMsg := range chat.Messages { // historyMsg is *waHistorySync.HistorySyncMsg
				msg := historyMsg.GetMessage()
				if msg == nil || msg.GetKey() == nil {
					continue
				}

				key := msg.GetKey()
				chatJID := key.GetRemoteJID()

				var senderJID string
				if msg.Participant != nil {
					senderJID = *msg.Participant
				} else if key.Participant != nil {
					senderJID = *key.Participant
				} else {
					senderJID = ""
				}

				msgID := key.GetID()
				message := msg.GetMessage()

				var content string
				if message.GetConversation() != "" {
					content = message.GetConversation()
				} else if ext := message.GetExtendedTextMessage(); ext != nil {
					content = ext.GetText()
				} else {
					continue // Skip unsupported types
				}

				em := entity.Message{
					ID:      msgID,
					Sender:  senderJID,
					GroupID: chatJID,
					Content: content,
				}

				for _, sub := range c.eventSubs {
					sub(em)
				}
			}
		}

	}
}

// ResolveGroupByName searches the synced chat store for a subject containing (case-insensitive) the given name.
// Returns the first match. Improve w/ exact matching or config.
func (c *Client) ResolveGroupByName(ctx context.Context, name string) (*types.JID, error) {
	groups, err := c.wc.GetJoinedGroups()
	if err != nil {
		return nil, err
	}

	lname := strings.ToLower(name)
	for _, group := range groups {
		if strings.ToLower(group.Name) == lname || strings.Contains(strings.ToLower(group.Name), lname) {
			jid := group.JID
			return &jid, nil
		}
	}
	return nil, fmt.Errorf("group %q not found", name)
}

// Revoke tries to delete a message for everyone. WhatsApp enforces a time window; may fail.
// BuildRevoke is preferred over deprecated RevokeMessage.
func (c *Client) Revoke(ctx context.Context, chat types.JID, msgID string) error {
	// Build and send revoke message
	msg := c.wc.BuildRevoke(chat, c.targetJID, types.MessageID(msgID))
	_, err := c.wc.SendMessage(ctx, chat, msg)
	return err
}

// Access underlying whatsmeow client (for advanced use).
func (c *Client) Raw() *whatsmeow.Client { return c.wc }

// ResolveUserByPhone scans synced contacts for a JID matching the given phone number.
// It returns the first match (usually fine for personal usage).
func (c *Client) ResolveUserByPhone(ctx context.Context, phone string) (*types.JID, error) {
	phone = strings.TrimPrefix(phone, "+")
	jid := types.NewJID(phone, types.DefaultUserServer)

	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("resolve target phone (after retries): no WhatsApp JID found for phone: %s", phone)
		case <-ticker.C:
			// First try resolving via LID store
			lid, err := c.wc.Store.LIDs.GetLIDForPN(ctx, jid)
			if err == nil && !lid.IsEmpty() {
				// ✅ Now resolve the JID back from the LID
				jid, err := c.wc.Store.LIDs.GetPNForLID(ctx, lid)
				if err == nil && !jid.IsEmpty() {
					log.Printf("✅ Resolved phone %s via LID→PN mapping: %s", phone, jid.String())
					return &jid, nil
				}
			}

			// Fallback: try contact list
			contacts, err := c.wc.Store.Contacts.GetAllContacts(ctx)
			if err == nil {
				for cjid := range contacts {
					if cjid.User == phone {
						log.Printf("✅ Resolved phone %s via contact store: %s", phone, cjid.String())
						return &cjid, nil
					}
				}
			}

			log.Printf("🔄 Waiting for contact or LID sync for phone: %s", phone)
		}
	}
}

func (c *Client) ResolvePN(ctx context.Context, lid types.JID) (types.JID, error) {
	pn, err := c.wc.Store.LIDs.GetPNForLID(ctx, lid)
	if err == nil && !pn.IsEmpty() {
		return pn, nil
	}
	// fallback: maybe bare JID means PN already
	if lid.Server == types.DefaultUserServer {
		return lid, nil
	}
	return types.EmptyJID, fmt.Errorf("no PN found for LID %s", lid)
}
