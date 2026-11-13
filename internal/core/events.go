package core

import (
	"slices"
	"sync"
)

// Event is agentsrc.updated for one space: SessionKeys changed; nil means the list itself changed.
type Event struct {
	Space       string
	SessionKeys []string
}

// maxPending is how many events a slow subscriber may have waiting before they fold into one "list changed".
const maxPending = 64

// Subscription receives one space's events ("" is every space's). Wait on Ready, then Take what is there.
type Subscription struct {
	space  string
	b      *broker
	ready  chan struct{}
	mu     sync.Mutex
	queue  []Event
	closed bool
}

// Ready is signalled when events are waiting.
func (s *Subscription) Ready() <-chan struct{} { return s.ready }

// Take returns and clears the events waiting.
func (s *Subscription) Take() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.queue
	s.queue = nil
	return q
}

// Close stops the subscription.
func (s *Subscription) Close() { s.b.remove(s) }

func (s *Subscription) push(e Event) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if len(s.queue) >= maxPending {
		s.queue = []Event{{Space: e.Space}}
	} else {
		s.queue = append(s.queue, Event{Space: e.Space, SessionKeys: slices.Clone(e.SessionKeys)})
	}
	s.mu.Unlock()
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

type broker struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

func (b *broker) subscribe(space string) *Subscription {
	s := &Subscription{space: space, b: b, ready: make(chan struct{}, 1)}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs == nil {
		b.subs = map[*Subscription]struct{}{}
	}
	b.subs[s] = struct{}{}
	return s
}

func (b *broker) remove(s *Subscription) {
	b.mu.Lock()
	delete(b.subs, s)
	b.mu.Unlock()
	s.mu.Lock()
	s.closed, s.queue = true, nil
	s.mu.Unlock()
}

func (b *broker) publish(e Event) {
	if len(e.SessionKeys) == 0 {
		e.SessionKeys = nil
	}
	b.mu.Lock()
	subs := make([]*Subscription, 0, len(b.subs))
	for s := range b.subs {
		if s.space == "" || s.space == e.Space {
			subs = append(subs, s)
		}
	}
	b.mu.Unlock()
	for _, s := range subs {
		s.push(e)
	}
}
