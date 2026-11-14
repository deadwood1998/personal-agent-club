package providers

import (
	"testing"

	"github.com/nwverse/personal-agent-club/internal/protocol"
)

func TestRegistryHasTheAPIProviders(t *testing.T) {
	r, err := Registry(Config{Proxy: "socks5h://127.0.0.1:1080"})
	if err != nil {
		t.Fatal(err)
	}
	available := map[string]bool{}
	for _, p := range r.List() {
		available[p.Provider] = p.Available
		if p.Available && p.Connection != protocol.ConnectionServer {
			t.Fatalf("%+v", p)
		}
	}
	want := map[string]bool{"cue": true, "grok_bot": true, "groq": true, "muse": false, "dots": false, "claude_duet": false}
	for name, ok := range want {
		if available[name] != ok {
			t.Errorf("%s available %v", name, available[name])
		}
	}
	if _, err := Registry(Config{Proxy: "nope"}); err == nil {
		t.Fatal("a bad proxy was accepted")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv(EnvProxy, " socks5://127.0.0.1:1080 ")
	t.Setenv(EnvGroqModel, "llama-3.3-70b-versatile")
	if c := FromEnv(); c.Proxy != "socks5://127.0.0.1:1080" || c.GroqModel != "llama-3.3-70b-versatile" {
		t.Fatalf("%+v", c)
	}
}
