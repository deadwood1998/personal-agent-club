// Package store keeps one space's accounts, sessions, messages and outbox. Store is the interface the core uses;
// Memory implements it for tests and the skeleton. A SQLite implementation (one file per space, the same tables as
// the Lark gateway's agent_accounts / agent_sessions / agent_messages / agent_outbox) comes later.
package store

import (
	"context"
	"errors"
	"strings"

	"github.com/nwverse/personal-agent-club/internal/protocol"
)

// ErrNotFound is returned by updates of a row that does not exist.
var ErrNotFound = errors.New("store: not found")

// Account is a paired account. Credentials are not here: they live in the vault.
type Account struct {
	AccountID         string
	Provider          string
	DisplayName       string
	ExternalAccountID string
	Status            string // protocol.AccountActive, AccountPaused, AccountError
	LastSyncedAt      int64
	LastError         string
	// AssistantControl is the account's assistant control level (core.Control), "off" when new.
	AssistantControl string
	ShowQuota        bool
	CreatedAt        int64
	UpdatedAt        int64
}

// AccountPatch changes the fields that are set.
type AccountPatch struct {
	Status           *string
	DisplayName      *string
	LastSyncedAt     *int64
	LastError        *string
	AssistantControl *string
	ShowQuota        *bool
}

// Session is one stored session.
type Session struct {
	SessionKey       string
	AccountID        string
	Provider         string
	ExternalID       string
	Title            string
	Kind             string
	ParentSessionKey string
	CanSend          bool
	AvatarURL        string
	Ref              map[string]string
	RankTimeMs       int64
	ReadTimeMs       int64
	UnreadCount      int
	PreviewText      string
	PreviewIsMine    bool
	UpdatedAt        int64
}

// Message is one stored message.
type Message struct {
	SessionKey   string
	ExternalID   string
	Role         string
	Blocks       []protocol.Block
	CreateTimeMs int64
	Position     int64
}

// Text is the message's searchable text: its text blocks, one per line.
func (m Message) Text() string { return BlocksText(m.Blocks) }

// BlocksText joins the text of text blocks.
func BlocksText(blocks []protocol.Block) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// Cursor is a position in a session's messages: the page before it is older.
type Cursor struct {
	CreateTimeMs int64
	Position     int64
	ExternalID   string
}

// Outbox is one send. Status is one of protocol.LocalPending, LocalClaimed, LocalSent, LocalUnconfirmed,
// LocalFailed.
type Outbox struct {
	ClientID     string
	SessionKey   string
	AccountID    string
	Text         string
	Status       string
	LastError    string
	AttemptCount int
	CreatedAt    int64
	UpdatedAt    int64
}

// Store is one space's data.
type Store interface {
	ListAccounts(ctx context.Context) ([]Account, error)
	Account(ctx context.Context, accountID string) (Account, bool, error)
	InsertAccount(ctx context.Context, a Account) error
	UpdateAccount(ctx context.Context, accountID string, p AccountPatch, now int64) error
	// RemoveAccount removes the account; with deleteData its sessions, messages and outbox go too, otherwise they
	// stay (unlisted) for a later re-pairing or export.
	RemoveAccount(ctx context.Context, accountID string, deleteData bool) error

	// UpsertSessions writes the sessions an account lists; it returns how many were new or changed.
	UpsertSessions(ctx context.Context, sessions []Session, now int64) (int, error)
	// UpsertMessages writes one page of a session's messages and refreshes the session's rank, preview and unread
	// count; it returns how many were new or changed.
	UpsertMessages(ctx context.Context, sessionKey string, messages []Message, now int64) (int, error)
	// Session is a session of a listed account.
	Session(ctx context.Context, sessionKey string) (Session, bool, error)
	// Sessions are the listed accounts' sessions, most recent first.
	Sessions(ctx context.Context, limit int) ([]Session, error)
	AccountSessions(ctx context.Context, accountID string) ([]Session, error)
	// Messages is the page of limit messages before the cursor (nil: the newest), oldest first.
	Messages(ctx context.Context, sessionKey string, limit int, before *Cursor) (page []Message, hasMore bool, err error)
	MarkRead(ctx context.Context, sessionKey string, now int64) error

	// EnqueueOutbox writes a pending send; inserted is false when its client id is taken (the existing row is
	// returned).
	EnqueueOutbox(ctx context.Context, o Outbox) (row Outbox, inserted bool, err error)
	Outbox(ctx context.Context, clientID string) (Outbox, bool, error)
	// ClaimOutbox moves pending to claimed once; a second claimer gets ok false.
	ClaimOutbox(ctx context.Context, clientID string, now int64) (Outbox, bool, error)
	SettleOutbox(ctx context.Context, clientID, status, lastError string, now int64) error
	// CancelOutbox fails a pending send of the session; ok is false when there is none.
	CancelOutbox(ctx context.Context, clientID, sessionKey string, now int64) (bool, error)
	// AbandonClaimedOutbox marks every claimed send unconfirmed (after a restart it may have gone out).
	AbandonClaimedOutbox(ctx context.Context, now int64) ([]Outbox, error)
	// ConfirmOutbox moves an unconfirmed send to sent or failed; ok is false when it was not unconfirmed.
	ConfirmOutbox(ctx context.Context, clientID, status, lastError string, now int64) (bool, error)
	PendingOutbox(ctx context.Context) ([]Outbox, error)
	// LocalMessages are the session's sends the agent has not echoed yet: not sent, or sent since `since` with no
	// matching user message.
	LocalMessages(ctx context.Context, sessionKey string, since int64) ([]Outbox, error)

	// Search finds messages of listed accounts whose text contains q, newest first.
	Search(ctx context.Context, q string, limit int) ([]protocol.SearchHit, error)
}
