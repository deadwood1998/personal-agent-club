package web

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

func closedPort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func TestFailsInsteadOfConnectingDirectlyWhenTheSOCKSProxyIsDown(t *testing.T) {
	direct := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { direct++ }))
	defer target.Close()
	c, err := NewClient("socks5://" + closedPort(t))
	if err != nil {
		t.Fatal(err)
	}
	r := Request(ctx, c, http.MethodGet, target.URL, nil, nil, 2*time.Second)
	if !r.Transport || direct != 0 {
		t.Fatalf("%+v, %d direct", r, direct)
	}
}

func TestTunnelsAnHTTPProxysRequestsToTheTargetHostItIsGiven(t *testing.T) {
	var mu sync.Mutex
	var tunnels []string
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				req, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					return
				}
				mu.Lock()
				tunnels = append(tunnels, req.Method+" "+req.Host)
				mu.Unlock()
				_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
			}()
		}
	}()
	c, err := NewClient("http://" + l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := Request(ctx, c, http.MethodGet, "https://api.example.invalid/x", nil, nil, 2*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if !r.Transport || len(tunnels) != 1 || tunnels[0] != "CONNECT api.example.invalid:443" {
		t.Fatalf("%+v %v", r, tunnels)
	}
}

func TestUsesTheEnvironmentsProxyWithoutAnExplicitOne(t *testing.T) {
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.String())
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	c, err := NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	r := Request(ctx, c, http.MethodGet, "http://api.example.invalid/v2/agent.list", nil, nil, 0)
	if !r.OK || len(seen) != 1 || seen[0] != "http://api.example.invalid/v2/agent.list" {
		t.Fatalf("%+v %v", r, seen)
	}
}

func TestParseProxy(t *testing.T) {
	for _, ok := range []string{"socks5://127.0.0.1:1080", "socks5h://proxy.example:1080", "http://127.0.0.1:3128", "https://p.example:443"} {
		if _, err := ParseProxy(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"127.0.0.1:1080", "ftp://h:1", "socks5://h", "http://:80"} {
		if _, err := ParseProxy(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestRefusesRedirectsAndOversizedOrInvalidBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		case "/elsewhere":
			t.Error("a redirect was followed")
		case "/big":
			_, _ = w.Write([]byte(`"` + strings.Repeat("x", MaxResponse) + `"`))
		case "/html":
			_, _ = w.Write([]byte("<html>"))
		case "/limited":
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()
	c, _ := NewClient("")
	if r := Request(ctx, c, http.MethodGet, srv.URL+"/redirect", nil, nil, 0); !r.Transport {
		t.Fatalf("%+v", r)
	}
	if r := Request(ctx, c, http.MethodGet, srv.URL+"/big", nil, nil, 0); r.OK || r.Transport {
		t.Fatalf("%+v", r)
	}
	if r := Request(ctx, c, http.MethodGet, srv.URL+"/html", nil, nil, 0); r.OK || r.Transport || r.Status != 200 {
		t.Fatalf("%+v", r)
	}
	r := Request(ctx, c, http.MethodGet, srv.URL+"/limited", nil, nil, 0)
	if e := ReadError(r); e.Code != "RATE_LIMITED" || e.RetryAfter != 30*time.Second || !e.Retryable {
		t.Fatalf("%+v", e)
	}
}

func TestMillis(t *testing.T) {
	for v, want := range map[any]int64{"1761802000000": 1761802000000, "": 0, "12a": 0, "12345678901234567": 0,
		float64(5): 5, float64(-5): 0, 1.5: 0, nil: 0} {
		if got := Millis(v); got != want {
			t.Errorf("%v: %d", v, got)
		}
	}
}
