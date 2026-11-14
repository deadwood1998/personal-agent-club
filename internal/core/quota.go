package core

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/nwverse/personal-agent-club/internal/protocol"
	"github.com/nwverse/personal-agent-club/internal/provider"
	"github.com/nwverse/personal-agent-club/internal/store"
)

// Quota comes only from what costs nothing: the readings a provider's answer carries (Groq's rate limit headers)
// and any 429. It is kept in memory: after a restart nothing is known until the next answer.
const (
	quotaLowAt        = 0.2
	quotaStaleMs      = 15 * 60_000
	quotaUnknownReset = 5 * 60_000
)

type quotaState struct {
	readings  []provider.QuotaReading
	checkedAt int64
	// limitedUntil is when a 429 stops counting as exhausted; resetsAt is what the provider said (0 if nothing).
	limitedUntil int64
	resetsAt     int64
}

// observe records a 429 among err.
func (s *Space) observe(accountID string, err error) {
	var pe *provider.Error
	if accountID == "" || !errors.As(err, &pe) || (pe.HTTPStatus != 429 && pe.Code != provider.CodeRateLimited) {
		return
	}
	now := s.now()
	q := &quotaState{checkedAt: now, limitedUntil: now + quotaUnknownReset}
	if pe.RetryAfter > 0 {
		q.resetsAt = now + pe.RetryAfter.Milliseconds()
		q.limitedUntil = q.resetsAt
	}
	s.setQuota(accountID, q)
}

// noteQuota records an answer's readings.
func (s *Space) noteQuota(accountID string, readings []provider.QuotaReading) {
	if len(readings) == 0 {
		return
	}
	s.setQuota(accountID, &quotaState{readings: readings, checkedAt: s.now()})
}

func (s *Space) setQuota(accountID string, q *quotaState) {
	now := s.now()
	s.mu.Lock()
	before := quotaState{}
	if old := s.quota[accountID]; old != nil {
		before = *old
	}
	s.quota[accountID] = q
	s.mu.Unlock()
	if quotaLevel(&before, now) != quotaLevel(q, now) {
		s.notify(nil)
	}
}

// quotaLevel is the state a client draws: ok, low, exhausted or "" (unknown).
func quotaLevel(q *quotaState, now int64) string {
	v := buildQuota(q, false, now)
	if v == nil {
		return ""
	}
	return v.State
}

func (s *Space) quotaView(a store.Account, now int64) *protocol.Quota {
	s.mu.Lock()
	q := s.quota[a.AccountID]
	s.mu.Unlock()
	return buildQuota(q, a.ShowQuota, now)
}

func buildQuota(q *quotaState, show bool, now int64) *protocol.Quota {
	if q == nil || q.checkedAt == 0 {
		return nil
	}
	stale := now-q.checkedAt > quotaStaleMs
	if q.limitedUntil != 0 {
		if now >= q.limitedUntil {
			return nil
		}
		label := "exhausted"
		if q.resetsAt != 0 {
			label = "exhausted until " + time.UnixMilli(q.resetsAt).UTC().Format(time.RFC3339)
		}
		return &protocol.Quota{State: protocol.QuotaExhausted, Label: label, ResetsAt: q.resetsAt, CheckedAt: q.checkedAt,
			Source: protocol.QuotaSourceResponse, Stale: stale, Show: show}
	}
	var dims []protocol.Quota
	for _, r := range q.readings {
		if r.Limit <= 0 {
			continue
		}
		left := math.Max(0, math.Min(1, r.Remaining/r.Limit))
		if r.ResetsAt != 0 && now >= r.ResetsAt {
			continue
		}
		left = math.Round(left*1000) / 1000
		state := protocol.QuotaOK
		switch {
		case left == 0:
			state = protocol.QuotaExhausted
		case left <= quotaLowAt:
			state = protocol.QuotaLow
		}
		label := fmt.Sprintf("%.0f%% of %s left", left*100, r.Kind)
		if r.Window != "" {
			label += " (per " + r.Window + ")"
		}
		dims = append(dims, protocol.Quota{State: state, Remaining: &left, Kind: r.Kind, Label: label, ResetsAt: r.ResetsAt,
			CheckedAt: q.checkedAt, Source: protocol.QuotaSourceProvider, Stale: stale, Show: show})
	}
	if len(dims) == 0 {
		return nil
	}
	tightest := 0
	for i, d := range dims {
		if *d.Remaining < *dims[tightest].Remaining {
			tightest = i
		}
	}
	out := dims[tightest]
	for i, d := range dims {
		if i != tightest {
			out.Details = append(out.Details, d)
		}
	}
	return &out
}
