package protocol

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func ptr[T any](v T) *T { return &v }

// The goldens pin the JSON field names (and which ones are omitted when empty) against the Persona hub's
// server/internal/agents/wire.go and the agentsrc parameters in docs/extensions/extensions.md. A failing golden is a
// wire change: check it against Persona before running with -update.
func TestGolden(t *testing.T) {
	cases := map[string]any{
		"status": StatusResult{Enabled: true, Providers: []Provider{
			{Provider: "cue", Connection: ConnectionServer, Available: false},
			{Provider: "echo", Connection: ConnectionServer, Available: true},
		}, Accounts: []Account{
			{AccountID: "a0123456789ab", Provider: "echo", DisplayName: "Echo", Status: AccountActive,
				ExternalAccountID: "echo:00112233", LastSyncedAt: 1800000000000, SessionCount: 2},
			{AccountID: "a0123456789ac", Provider: "echo", SessionCount: 0},
		}},
		"status_empty": StatusResult{Enabled: true, Setup: SetupNoAccounts, Providers: []Provider{}, Accounts: []Account{}},
		"sessions": SessionsResult{Sessions: []Session{
			{SessionKey: "agent:echo:a0123456789ab:main", AccountID: "a0123456789ab", Provider: "echo", Title: "Echo",
				Kind: KindMain, CanSend: true, UnreadCount: 1, RankTimeMs: 1800000000000, AvatarURL: "https://example.com/a.png",
				PreviewText: "echo: hi", PreviewIsMine: false, OnTopRankTimeMs: 0},
			{SessionKey: "agent:echo:a0123456789ab:notes", AccountID: "a0123456789ab", Provider: "echo", Title: "Notes",
				Kind: KindSide, ParentSessionKey: "agent:echo:a0123456789ab:main", PreviewText: "mine", PreviewIsMine: true,
				OnTopRankTimeMs: 1800000000001},
		}},
		"messages": MessagePage{
			Session: &Session{SessionKey: "agent:echo:a0123456789ab:main", AccountID: "a0123456789ab", Provider: "echo",
				Title: "Echo", Kind: KindMain, CanSend: true},
			Messages: []Message{
				{ExternalID: "m1", SessionKey: "agent:echo:a0123456789ab:main", Role: RoleAssistant,
					Blocks: []Block{TextBlock("echo: hi"), {Type: "tool-call", Name: "search", Status: "done",
						Summary: "looked it up", Input: json.RawMessage(`{"q":"x"}`)}}, CreateTimeMs: 1800000000000, Position: 1},
				{ExternalID: "local:00000000-0000-4000-8000-000000000001", SessionKey: "agent:echo:a0123456789ab:main",
					Role: RoleUser, Blocks: []Block{TextBlock("hi")}, CreateTimeMs: 1800000000002,
					Local: &Local{ClientID: "00000000-0000-4000-8000-000000000001", Status: LocalUnconfirmed, Error: "SEND_UNCONFIRMED"}},
			},
			HasMore: true, NextCursor: "WzE4MDAwMDAwMDAwMDAsMSwibTEiXQ",
		},
		"messages_empty": MessagePage{Messages: []Message{}},
		"search": SearchResult{Hits: []SearchHit{{SessionKey: "agent:echo:a0123456789ab:main", ExternalID: "m1",
			Text: "echo: hi", Role: RoleAssistant, CreateTimeMs: 1800000000000, Title: "Echo", Provider: "echo"}}},
		"updated":      UpdatedEvent{Contribution: "pac", SessionKeys: []string{"agent:echo:a0123456789ab:main"}},
		"updated_list": UpdatedEvent{Contribution: "pac"},
		"params": map[string]any{
			"messages":       MessagesParams{SessionKey: "k", Cursor: "c", Limit: 50},
			"send":           SendParams{SessionKey: "k", ClientID: "00000000-0000-4000-8000-000000000001", Text: "hi"},
			"delete_message": DeleteMessageParams{SessionKey: "k", ClientID: "00000000-0000-4000-8000-000000000001"},
			"mark_read":      MarkReadParams{SessionKey: "k"},
			"search":         SearchParams{Q: "hi"},
		},
		"error": Error{Code: CodeNotFound, Message: "no such session"},
		"quota": Quota{State: QuotaOK, Remaining: ptr(0.62), Kind: QuotaKindTokens, Label: "62% left", ResetsAt: 1794500000000,
			CheckedAt: 1794496400000, Source: QuotaSourceProvider, Stale: false, Show: true},
		"quota_exhausted": Quota{State: QuotaExhausted, Source: QuotaSourceResponse, ResetsAt: 1794500000000},
		"club_info": ClubInfo{Version: "0.1.0", ProtocolVersion: Version, Providers: []Provider{
			{Provider: "muse", Connection: ConnectionBrowser}}, Browser: false},
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", name+".json")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s changed:\n got %s\nwant %s", path, got, want)
			}
		})
	}
}

// What the hub sends and reads must round-trip: unknown fields are ignored, unknown block types kept verbatim.
func TestDecodeHubShapes(t *testing.T) {
	var p MessagesParams
	if err := json.Unmarshal([]byte(`{"contribution":"pac","session_key":"k","cursor":"","limit":50}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Contribution != "pac" || p.SessionKey != "k" || p.Limit != 50 {
		t.Fatalf("%+v", p)
	}
	raw := `{"type":"reaction","emoji":"👍","from":"user"}`
	var b Block
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(b)
	if string(out) != raw {
		t.Fatalf("block not kept verbatim: %s", out)
	}
	var m Message
	if err := json.Unmarshal([]byte(`{"externalId":"x","sessionKey":"k","role":"user","blocks":[],"createTimeMs":1,"position":0,"local":{"clientId":"c","status":"sent"},"origin":"assistant"}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Local == nil || m.Local.Status != LocalSent {
		t.Fatalf("%+v", m)
	}
}

func TestCodeOf(t *testing.T) {
	for _, e := range []*Error{NotFound("x"), Invalid("x"), Conflict("x"), Unsupported("x")} {
		code, _ := e.WireError()
		if CodeOf(e) != e.Code || code != e.Code {
			t.Fatalf("%+v", e)
		}
	}
}
