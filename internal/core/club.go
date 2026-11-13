// Package core is club itself, behind both shells: spaces, accounts, one sync round, the outbox, read state, search
// and change events. Both the Persona extension shell and the serve shell call it through Club.Call, so they answer
// the same protocol by construction.
package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nwverse/personal-agent-club/internal/protocol"
	"github.com/nwverse/personal-agent-club/internal/provider"
	"github.com/nwverse/personal-agent-club/internal/store"
	"github.com/nwverse/personal-agent-club/internal/vault"
)

// DefaultFollowUps are when a sent message's session is synced again, and an unconfirmed send asked about.
var DefaultFollowUps = []time.Duration{4 * time.Second, 15 * time.Second, 45 * time.Second}

// Options configure a Club.
type Options struct {
	// Version is the program's version, reported by club.info.
	Version string
	// Registry holds the provider adapters (required).
	Registry *provider.Registry
	// OpenSpace opens a space's store and vault; nil keeps every space in memory.
	OpenSpace func(name string) (store.Store, vault.Vault, error)
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// FollowUps are the follow-up delays; nil is DefaultFollowUps, an empty non-nil slice turns them off.
	FollowUps []time.Duration
	// Log receives operational messages (never credentials or message text); nil discards them.
	Log *slog.Logger
}

// Club is one club: its spaces and their events.
type Club struct {
	opts   Options
	events broker

	mu     sync.Mutex
	spaces map[string]*Space
	timers map[*time.Timer]struct{}
	closed bool
	wg     sync.WaitGroup
}

// New is a club.
func New(opts Options) *Club {
	if opts.Registry == nil {
		opts.Registry = provider.NewRegistry()
	}
	if opts.OpenSpace == nil {
		opts.OpenSpace = func(string) (store.Store, vault.Vault, error) { return store.NewMemory(), vault.NewMemory(), nil }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.FollowUps == nil {
		opts.FollowUps = DefaultFollowUps
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &Club{opts: opts, spaces: map[string]*Space{}, timers: map[*time.Timer]struct{}{}}
}

// Registry is the club's providers.
func (c *Club) Registry() *provider.Registry { return c.opts.Registry }

func (c *Club) now() int64 { return c.opts.Now().UnixMilli() }

// Info is club.info's answer.
func (c *Club) Info() protocol.ClubInfo {
	return protocol.ClubInfo{Version: c.opts.Version, ProtocolVersion: protocol.Version,
		Providers: c.opts.Registry.List(), Browser: false}
}

// Space opens a space (once; later calls get the same one). Opening recovers its outbox: sends claimed before a
// restart become unconfirmed and are never sent again.
func (c *Club) Space(ctx context.Context, name string) (*Space, error) {
	if !ValidSpace(name) {
		return nil, protocol.Invalid(fmt.Sprintf("invalid space name %q", name))
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("club is closed")
	}
	if s := c.spaces[name]; s != nil {
		c.mu.Unlock()
		return s, nil
	}
	st, v, err := c.opts.OpenSpace(name)
	if err != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("open space %s: %w", name, err)
	}
	s := &Space{name: name, club: c, store: st, vault: v}
	c.spaces[name] = s
	c.mu.Unlock()
	if err := s.recover(ctx); err != nil {
		c.opts.Log.Warn("outbox recovery failed", "space", name, "err", err)
	}
	return s, nil
}

// Subscribe receives a space's change events ("" for every space).
func (c *Club) Subscribe(space string) *Subscription { return c.events.subscribe(space) }

func (c *Club) publish(e Event) { c.events.publish(e) }

// after runs f after d unless the club closes first.
func (c *Club) after(d time.Duration, f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	var t *time.Timer
	c.wg.Add(1)
	t = time.AfterFunc(d, func() {
		defer c.wg.Done()
		c.mu.Lock()
		_, live := c.timers[t]
		delete(c.timers, t)
		closed := c.closed
		c.mu.Unlock()
		if live && !closed {
			f()
		}
	})
	c.timers[t] = struct{}{}
}

// Close stops follow-ups and waits for those running.
func (c *Club) Close() {
	c.mu.Lock()
	c.closed = true
	for t := range c.timers {
		if t.Stop() {
			c.wg.Done()
		}
		delete(c.timers, t)
	}
	c.mu.Unlock()
	c.wg.Wait()
}
