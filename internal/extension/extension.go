// Package extension is the Persona shell: club as a Persona extension program, contributing the agent source "pac".
// The hub's agentsrc.* calls go to the club's default space through the same core.Club.Call the serve shell uses,
// and the space's changes go back to the hub as agentsrc.updated.
package extension

import (
	"context"
	"encoding/json"

	"github.com/nwverse/persona/sdk/go/ext"

	pac "github.com/nwverse/personal-agent-club"
	"github.com/nwverse/personal-agent-club/internal/core"
	"github.com/nwverse/personal-agent-club/internal/protocol"
	"github.com/nwverse/personal-agent-club/internal/provider/echo"
)

// ID is the extension's id; Contribution is its agent_sources contribution, the prefix of its session ids in
// Persona (<contribution>:<sessionKey>), which must never change.
const (
	ID           = "personal-agent-club"
	Contribution = "pac"
)

// Config is the extension's configuration (persona-extension.json "configuration").
type Config struct {
	DemoEcho bool `json:"demo_echo"`
}

// Program is the extension program serving club. The club is closed when the program ends.
func Program(club *core.Club, version string) ext.Program {
	return ext.Program{
		ID:       ID,
		Version:  version,
		Manifest: json.RawMessage(pac.Manifest),
		Init: func(ctx context.Context, init *ext.Init, hub *ext.Hub) (map[string]any, error) {
			var cfg Config
			if err := init.Config(&cfg); err != nil {
				return nil, err
			}
			if cfg.DemoEcho {
				if _, ok := club.Registry().Get(echo.Name); !ok {
					club.Registry().Register(echo.New())
				}
			}
			if _, err := club.Space(ctx, core.DefaultSpace); err != nil {
				return nil, err
			}
			sub := club.Subscribe(core.DefaultSpace)
			go forward(ctx, sub, hub, club)
			return nil, nil
		},
		Namespaces: map[string]ext.Handler{
			protocol.AgentSourceNamespace: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
				if c := ext.Contribution(params); c != "" && c != Contribution {
					return nil, ext.NoContribution(c)
				}
				return club.Call(ctx, core.DefaultSpace, protocol.AgentSourceNamespace+"."+method, params)
			},
		},
	}
}

// forward sends the space's changes to the hub until the program ends, then closes the club.
func forward(ctx context.Context, sub *core.Subscription, hub *ext.Hub, club *core.Club) {
	defer club.Close()
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Ready():
			for _, e := range sub.Take() {
				_ = hub.Notify(protocol.EventUpdated, protocol.UpdatedEvent{Contribution: Contribution, SessionKeys: e.SessionKeys})
			}
		}
	}
}
