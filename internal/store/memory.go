package store

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/nwverse/personal-agent-club/internal/protocol"
)

// echoWindowMs is how far back a stored user message may be to count as the agent's echo of a send.
const echoWindowMs = 60_000

// Memory is a Store in memory.
type Memory struct {
	mu       sync.Mutex
	accounts map[string]Account
	sessions map[string]Session
	messages map[string]map[string]Message // by session key, then external id
	outbox   map[string]Outbox
}

// NewMemory is an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{accounts: map[string]Account{}, sessions: map[string]Session{},
		messages: map[string]map[string]Message{}, outbox: map[string]Outbox{}}
}

var _ Store = (*Memory)(nil)

func (m *Memory) ListAccounts(context.Context) ([]Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := slices.Collect(maps.Values(m.accounts))
	slices.SortFunc(out, func(a, b Account) int {
		return cmp.Or(cmp.Compare(a.CreatedAt, b.CreatedAt), strings.Compare(a.AccountID, b.AccountID))
	})
	return out, nil
}

func (m *Memory) Account(_ context.Context, id string) (Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[id]
	return a, ok, nil
}

func (m *Memory) InsertAccount(_ context.Context, a Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts[a.AccountID] = a
	return nil
}

func (m *Memory) UpdateAccount(_ context.Context, id string, p AccountPatch, now int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[id]
	if !ok {
		return ErrNotFound
	}
	if p.Status != nil {
		a.Status = *p.Status
	}
	if p.DisplayName != nil {
		a.DisplayName = *p.DisplayName
	}
	if p.LastSyncedAt != nil {
		a.LastSyncedAt = *p.LastSyncedAt
	}
	if p.LastError != nil {
		a.LastError = *p.LastError
	}
	if p.AssistantControl != nil {
		a.AssistantControl = *p.AssistantControl
	}
	if p.ShowQuota != nil {
		a.ShowQuota = *p.ShowQuota
	}
	a.UpdatedAt = now
	m.accounts[id] = a
	return nil
}

func (m *Memory) RemoveAccount(_ context.Context, id string, deleteData bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[id]; !ok {
		return ErrNotFound
	}
	delete(m.accounts, id)
	if !deleteData {
		return nil
	}
	for key, s := range m.sessions {
		if s.AccountID == id {
			delete(m.sessions, key)
			delete(m.messages, key)
		}
	}
	for cid, o := range m.outbox {
		if o.AccountID == id {
			delete(m.outbox, cid)
		}
	}
	return nil
}

func (m *Memory) UpsertSessions(_ context.Context, sessions []Session, now int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := 0
	for _, s := range sessions {
		old, ok := m.sessions[s.SessionKey]
		if !ok {
			s.ReadTimeMs = max(now, s.RankTimeMs)
			s.UpdatedAt = now
			s.Ref = maps.Clone(s.Ref)
			m.sessions[s.SessionKey] = s
			changed++
			continue
		}
		next := old
		next.Title, next.Kind, next.ParentSessionKey, next.CanSend = s.Title, s.Kind, s.ParentSessionKey, s.CanSend
		next.AvatarURL, next.Ref = s.AvatarURL, maps.Clone(s.Ref)
		next.RankTimeMs = max(old.RankTimeMs, s.RankTimeMs)
		if next.Title != old.Title || next.Kind != old.Kind || next.ParentSessionKey != old.ParentSessionKey ||
			next.CanSend != old.CanSend || next.AvatarURL != old.AvatarURL || !maps.Equal(next.Ref, old.Ref) ||
			next.RankTimeMs != old.RankTimeMs {
			next.UpdatedAt = now
			m.sessions[s.SessionKey] = next
			changed++
		}
	}
	return changed, nil
}

func (m *Memory) UpsertMessages(_ context.Context, key string, messages []Message, now int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	byID := m.messages[key]
	if byID == nil {
		byID = map[string]Message{}
		m.messages[key] = byID
	}
	changed := 0
	for _, msg := range messages {
		msg.SessionKey = key
		old, ok := byID[msg.ExternalID]
		if msg.CreateTimeMs == 0 {
			msg.CreateTimeMs = now
			if ok {
				msg.CreateTimeMs = old.CreateTimeMs
			}
		}
		if ok && sameMessage(old, msg) {
			continue
		}
		byID[msg.ExternalID] = msg
		changed++
	}
	m.refresh(key, now)
	return changed, nil
}

func sameMessage(a, b Message) bool {
	if a.Role != b.Role || a.CreateTimeMs != b.CreateTimeMs || a.Position != b.Position || len(a.Blocks) != len(b.Blocks) {
		return false
	}
	for i := range a.Blocks {
		x, _ := a.Blocks[i].MarshalJSON()
		y, _ := b.Blocks[i].MarshalJSON()
		if string(x) != string(y) {
			return false
		}
	}
	return true
}

// refresh recomputes a session's rank, preview and unread count. m.mu is held.
func (m *Memory) refresh(key string, now int64) {
	s, ok := m.sessions[key]
	if !ok {
		return
	}
	var latest *Message
	var lastUser int64
	for _, msg := range m.messages[key] {
		if msg.Role == protocol.RoleUser {
			lastUser = max(lastUser, msg.CreateTimeMs)
		}
		if (msg.Role == protocol.RoleUser || msg.Role == protocol.RoleAssistant) && msg.Text() != "" {
			if latest == nil || compareMessages(msg, *latest) > 0 {
				x := msg
				latest = &x
			}
		}
	}
	next := s
	if latest != nil {
		next.RankTimeMs = max(s.RankTimeMs, latest.CreateTimeMs)
		next.PreviewText = latest.Text()
		next.PreviewIsMine = latest.Role == protocol.RoleUser
	}
	since := max(s.ReadTimeMs, lastUser)
	next.UnreadCount = 0
	for _, msg := range m.messages[key] {
		if msg.Role == protocol.RoleAssistant && msg.CreateTimeMs > since {
			next.UnreadCount++
		}
	}
	if next.RankTimeMs != s.RankTimeMs || next.PreviewText != s.PreviewText || next.PreviewIsMine != s.PreviewIsMine ||
		next.UnreadCount != s.UnreadCount {
		next.UpdatedAt = now
		m.sessions[key] = next
	}
}

func compareMessages(a, b Message) int {
	return cmp.Or(cmp.Compare(a.CreateTimeMs, b.CreateTimeMs), cmp.Compare(a.Position, b.Position),
		strings.Compare(a.ExternalID, b.ExternalID))
}

func (m *Memory) Session(_ context.Context, key string) (Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[key]
	if !ok {
		return Session{}, false, nil
	}
	if _, listed := m.accounts[s.AccountID]; !listed {
		return Session{}, false, nil
	}
	return s, true, nil
}

func (m *Memory) Sessions(_ context.Context, limit int) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Session
	for _, s := range m.sessions {
		if _, listed := m.accounts[s.AccountID]; listed {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Session) int {
		return cmp.Or(cmp.Compare(b.RankTimeMs, a.RankTimeMs), strings.Compare(a.SessionKey, b.SessionKey))
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) AccountSessions(_ context.Context, id string) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Session
	for _, s := range m.sessions {
		if s.AccountID == id {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Session) int { return strings.Compare(a.SessionKey, b.SessionKey) })
	return out, nil
}

func (m *Memory) Messages(_ context.Context, key string, limit int, before *Cursor) ([]Message, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Message
	for _, msg := range m.messages[key] {
		if before != nil && compareMessages(msg, Message{CreateTimeMs: before.CreateTimeMs, Position: before.Position,
			ExternalID: before.ExternalID}) >= 0 {
			continue
		}
		all = append(all, msg)
	}
	slices.SortFunc(all, compareMessages)
	hasMore := limit > 0 && len(all) > limit
	if hasMore {
		all = all[len(all)-limit:]
	}
	return all, hasMore, nil
}

func (m *Memory) MarkRead(_ context.Context, key string, now int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[key]
	if !ok {
		return ErrNotFound
	}
	s.ReadTimeMs = max(s.ReadTimeMs, now, s.RankTimeMs)
	s.UnreadCount = 0
	s.UpdatedAt = now
	m.sessions[key] = s
	return nil
}

func (m *Memory) EnqueueOutbox(_ context.Context, o Outbox) (Outbox, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.outbox[o.ClientID]; ok {
		return old, false, nil
	}
	o.Status = protocol.LocalPending
	o.UpdatedAt = o.CreatedAt
	m.outbox[o.ClientID] = o
	return o, true, nil
}

func (m *Memory) Outbox(_ context.Context, clientID string) (Outbox, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.outbox[clientID]
	return o, ok, nil
}

func (m *Memory) ClaimOutbox(_ context.Context, clientID string, now int64) (Outbox, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.outbox[clientID]
	if !ok || o.Status != protocol.LocalPending {
		return Outbox{}, false, nil
	}
	o.Status, o.AttemptCount, o.UpdatedAt = protocol.LocalClaimed, o.AttemptCount+1, now
	m.outbox[clientID] = o
	return o, true, nil
}

func (m *Memory) SettleOutbox(_ context.Context, clientID, status, lastError string, now int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.outbox[clientID]
	if !ok {
		return ErrNotFound
	}
	o.Status, o.LastError, o.UpdatedAt = status, lastError, now
	m.outbox[clientID] = o
	return nil
}

func (m *Memory) CancelOutbox(_ context.Context, clientID, key string, now int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.outbox[clientID]
	if !ok || o.SessionKey != key || o.Status != protocol.LocalPending {
		return false, nil
	}
	o.Status, o.LastError, o.UpdatedAt = protocol.LocalFailed, "CANCELLED", now
	m.outbox[clientID] = o
	return true, nil
}

func (m *Memory) AbandonClaimedOutbox(_ context.Context, now int64) ([]Outbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Outbox
	for id, o := range m.outbox {
		if o.Status == protocol.LocalClaimed {
			o.Status, o.LastError, o.UpdatedAt = protocol.LocalUnconfirmed, "SEND_UNCONFIRMED", now
			m.outbox[id] = o
			out = append(out, o)
		}
	}
	sortOutbox(out)
	return out, nil
}

func (m *Memory) ConfirmOutbox(_ context.Context, clientID, status, lastError string, now int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.outbox[clientID]
	if !ok || o.Status != protocol.LocalUnconfirmed {
		return false, nil
	}
	o.Status, o.LastError, o.UpdatedAt = status, lastError, now
	m.outbox[clientID] = o
	return true, nil
}

func (m *Memory) PendingOutbox(context.Context) ([]Outbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Outbox
	for _, o := range m.outbox {
		if o.Status == protocol.LocalPending {
			out = append(out, o)
		}
	}
	sortOutbox(out)
	return out, nil
}

func (m *Memory) LocalMessages(_ context.Context, key string, since int64) ([]Outbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Outbox
	for _, o := range m.outbox {
		if o.SessionKey != key || (o.Status == protocol.LocalSent && o.UpdatedAt < since) {
			continue
		}
		echoed := false
		for _, msg := range m.messages[key] {
			if msg.Role == protocol.RoleUser && msg.CreateTimeMs >= o.CreatedAt-echoWindowMs &&
				strings.TrimSpace(msg.Text()) == strings.TrimSpace(o.Text) {
				echoed = true
				break
			}
		}
		if !echoed {
			out = append(out, o)
		}
	}
	sortOutbox(out)
	return out, nil
}

func sortOutbox(out []Outbox) {
	slices.SortFunc(out, func(a, b Outbox) int {
		return cmp.Or(cmp.Compare(a.CreatedAt, b.CreatedAt), strings.Compare(a.ClientID, b.ClientID))
	})
}

func (m *Memory) Search(_ context.Context, q string, limit int) ([]protocol.SearchHit, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return []protocol.SearchHit{}, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hits := []protocol.SearchHit{}
	for key, byID := range m.messages {
		s, ok := m.sessions[key]
		if !ok {
			continue
		}
		if _, listed := m.accounts[s.AccountID]; !listed {
			continue
		}
		for _, msg := range byID {
			text := msg.Text()
			if !strings.Contains(strings.ToLower(text), q) {
				continue
			}
			hits = append(hits, protocol.SearchHit{SessionKey: key, ExternalID: msg.ExternalID, Text: text,
				Role: msg.Role, CreateTimeMs: msg.CreateTimeMs, Title: s.Title, Provider: s.Provider})
		}
	}
	slices.SortFunc(hits, func(a, b protocol.SearchHit) int {
		return cmp.Or(cmp.Compare(b.CreateTimeMs, a.CreateTimeMs), strings.Compare(a.SessionKey, b.SessionKey),
			strings.Compare(a.ExternalID, b.ExternalID))
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}
