// Package providers builds the registry of the API providers club ships: cue, grok_bot and groq, all through one
// HTTP client and its proxy. The browser providers (muse, dots, claude_duet) are not in this version.
package providers

import (
	"os"
	"strings"

	"github.com/nwverse/personal-agent-club/internal/provider"
	"github.com/nwverse/personal-agent-club/internal/provider/cue"
	"github.com/nwverse/personal-agent-club/internal/provider/grokbot"
	"github.com/nwverse/personal-agent-club/internal/provider/groq"
	"github.com/nwverse/personal-agent-club/internal/provider/web"
)

// Environment variables Config reads.
const (
	EnvProxy     = "PERSONAL_AGENT_CLUB_PROXY"
	EnvGroqModel = "PERSONAL_AGENT_CLUB_GROQ_MODEL"
)

// Config configures the providers.
type Config struct {
	// Proxy every provider request goes through (socks5://, socks5h://, http:// or https://); "" uses HTTP_PROXY,
	// HTTPS_PROXY and NO_PROXY from the environment.
	Proxy string
	// GroqModel is the model Groq accounts ask; "" is groq.DefaultModel.
	GroqModel string
}

// FromEnv is the configuration in the environment.
func FromEnv() Config {
	return Config{Proxy: strings.TrimSpace(os.Getenv(EnvProxy)), GroqModel: strings.TrimSpace(os.Getenv(EnvGroqModel))}
}

// Registry is a registry with the API providers.
func Registry(c Config) (*provider.Registry, error) {
	client, err := web.NewClient(c.Proxy)
	if err != nil {
		return nil, err
	}
	return provider.NewRegistry(
		cue.New(cue.Options{Client: client}),
		grokbot.New(grokbot.Options{Client: client}),
		groq.New(groq.Options{Client: client, Model: c.GroqModel}),
	), nil
}
