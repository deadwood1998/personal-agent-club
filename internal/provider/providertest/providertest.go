// Package providertest is a fake upstream for provider tests: an httptest server that answers from the recorded
// fixtures and keeps every call. Tests only; nothing here reaches a real provider.
package providertest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// Fixture is one of internal/provider/testdata's files, by name.
func Fixture(t testing.TB, name string) map[string]json.RawMessage {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Call is one request the upstream received.
type Call struct {
	Method string
	Path   string
	Query  map[string][]string
	Header http.Header
	Body   []byte
}

// JSON decodes the call's body.
func (c Call) JSON(t testing.TB) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(c.Body, &v); err != nil {
		t.Fatalf("body of %s: %v", c.Path, err)
	}
	return v
}

// Answer is what the upstream says to one call. Body is written as JSON unless it is a []byte; Drop closes the
// connection without an answer (a lost answer).
type Answer struct {
	Status int
	Header http.Header
	Body   any
	Drop   bool
}

// Upstream is a fake provider.
type Upstream struct {
	*httptest.Server
	mu    sync.Mutex
	calls []Call
}

// Serve starts an upstream that answers each call with answer.
func Serve(t testing.TB, answer func(Call) Answer) *Upstream {
	t.Helper()
	u := &Upstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c := Call{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body}
		u.mu.Lock()
		u.calls = append(u.calls, c)
		u.mu.Unlock()
		a := answer(c)
		if a.Drop {
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
					return
				}
			}
			panic(http.ErrAbortHandler)
		}
		for k, v := range a.Header {
			w.Header()[k] = v
		}
		var out []byte
		switch b := a.Body.(type) {
		case []byte:
			out = b
		case nil:
			out = []byte("{}")
		default:
			out, _ = json.Marshal(b)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(max(a.Status, 200))
		_, _ = io.Copy(w, bytes.NewReader(out))
	}))
	t.Cleanup(u.Close)
	return u
}

// Calls are the calls so far.
func (u *Upstream) Calls() []Call {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]Call(nil), u.calls...)
}
