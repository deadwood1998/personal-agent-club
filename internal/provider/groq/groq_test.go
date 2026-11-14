package groq

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nwverse/personal-agent-club/internal/protocol"
	"github.com/nwverse/personal-agent-club/internal/provider"
	"github.com/nwverse/personal-agent-club/internal/provider/providertest"
)

const key = "fixture-key-not-real"

var (
	ctx     = context.Background()
	session = provider.SessionRef{ExternalID: SessionID, Ref: map[string]string{"model": DefaultModel}}
	message = provider.Outgoing{ClientID: "1c5e1c2a-1f0e-4d6b-8c3a-2e4f5a6b7c8d", Text: "明早有几班？"}
)

func TestVerifiesTheKeyAgainstTheModelList(t *testing.T) {
	groq := providertest.Fixture(t, "groq-openai-v1.json")
	up := providertest.Serve(t, func(providertest.Call) providertest.Answer { return providertest.Answer{Body: groq["models"]} })
	acct, err := New(Options{Origin: up.URL + "/openai/v1"}).Verify(ctx, key)
	if err != nil || acct != (provider.RemoteAccount{ExternalAccountID: "groq", DisplayName: "Groq"}) {
		t.Fatalf("%+v %v", acct, err)
	}
	c := up.Calls()[0]
	if c.Path != "/openai/v1/models" || c.Header.Get("authorization") != "Bearer "+key {
		t.Fatalf("%+v", c)
	}
	if _, err := New(Options{Origin: up.URL, Model: "missing-model"}).Verify(ctx, key); provider.CodeOf(err) != "MODEL_UNAVAILABLE" {
		t.Fatal(err)
	}
	if _, err := New(Options{Origin: up.URL, Model: "llama-3.3-70b-versatile"}).Verify(ctx, key); err != nil {
		t.Fatal(err)
	}
	rejected := providertest.Serve(t, func(providertest.Call) providertest.Answer {
		return providertest.Answer{Status: 401, Body: groq["error.invalid_api_key"]}
	})
	if _, err := New(Options{Origin: rejected.URL}).Verify(ctx, key); provider.CodeOf(err) != "AUTH_FAILED" {
		t.Fatal(err)
	}
}

func TestAnswersWithTheRecentTurnsAndReturnsBothSides(t *testing.T) {
	groq := providertest.Fixture(t, "groq-openai-v1.json")
	header := http.Header{}
	header.Set("x-ratelimit-limit-requests", "1000")
	header.Set("x-ratelimit-remaining-requests", "150")
	header.Set("x-ratelimit-reset-requests", "2m59.56s")
	header.Set("x-ratelimit-limit-tokens", "8000")
	header.Set("x-ratelimit-remaining-tokens", "7000")
	header.Set("x-ratelimit-reset-tokens", "7.5s")
	up := providertest.Serve(t, func(providertest.Call) providertest.Answer {
		return providertest.Answer{Body: groq["chat.completion"], Header: header}
	})
	now := time.UnixMilli(1_000)
	a := New(Options{Origin: up.URL + "/openai/v1", Now: func() time.Time { return now }})
	if !a.WantsHistory() {
		t.Fatal("groq keeps no transcript")
	}
	res, err := a.Send(ctx, key, session, message, provider.SendContext{History: []provider.Turn{
		{Role: "user", Text: "查航班"}, {Role: "assistant", Text: "去哪？"}}})
	if err != nil {
		t.Fatal(err)
	}
	calls := up.Calls()
	if len(calls) != 1 || calls[0].Path != "/openai/v1/chat/completions" {
		t.Fatalf("%+v", calls)
	}
	if got := string(calls[0].Body); got != `{"messages":[{"content":"查航班","role":"user"},{"content":"去哪？","role":"assistant"},{"content":"明早有几班？","role":"user"}],"model":"openai/gpt-oss-120b"}` {
		t.Fatal(got)
	}
	want := provider.SendResult{Status: provider.Sent, ExternalID: "u:" + message.ClientID, Messages: []provider.RemoteMessage{
		{ExternalID: "u:" + message.ClientID, Role: "user", Blocks: []protocol.Block{protocol.TextBlock(message.Text)}, CreateTimeMs: 1000},
		{ExternalID: "a:" + message.ClientID, Role: "assistant", Blocks: []protocol.Block{protocol.TextBlock("明早 9 点有 3 班直飞。")}, CreateTimeMs: 1001},
	}, Quota: []provider.QuotaReading{
		{Kind: "requests", Limit: 1000, Remaining: 150, ResetsAt: 1000 + 179560, Window: "day"},
		{Kind: "tokens", Limit: 8000, Remaining: 7000, ResetsAt: 1000 + 7500, Window: "minute"},
	}}
	if !reflect.DeepEqual(res, want) {
		t.Fatalf("%+v", res)
	}
}

func TestAnEmptyAnswerIsAStatus(t *testing.T) {
	up := providertest.Serve(t, func(providertest.Call) providertest.Answer {
		return providertest.Answer{Body: map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "  "}}}}}
	})
	res, err := New(Options{Origin: up.URL}).Send(ctx, key, session, message, provider.SendContext{})
	if err != nil || res.Messages[1].Role != "system" || res.Messages[1].Blocks[0].Status != "empty_answer" || res.Quota != nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestReportsALostAnswerAsUnconfirmedAndRefusalsAsErrors(t *testing.T) {
	groq := providertest.Fixture(t, "groq-openai-v1.json")
	lost := providertest.Serve(t, func(providertest.Call) providertest.Answer { return providertest.Answer{Drop: true} })
	res, err := New(Options{Origin: lost.URL}).Send(ctx, key, session, message, provider.SendContext{})
	if err != nil || !reflect.DeepEqual(res, provider.SendResult{Status: provider.Unconfirmed, Error: "SEND_UNCONFIRMED"}) || len(lost.Calls()) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	limited := providertest.Serve(t, func(providertest.Call) providertest.Answer {
		return providertest.Answer{Status: 429, Body: groq["error.rate_limit"], Header: http.Header{"Retry-After": {"7"}}}
	})
	_, err = New(Options{Origin: limited.URL}).Send(ctx, key, session, message, provider.SendContext{})
	if provider.CodeOf(err) != "RATE_LIMITED" || len(limited.Calls()) != 1 {
		t.Fatal(err)
	}
	if e, ok := err.(*provider.Error); !ok || e.RetryAfter != 7*time.Second || e.HTTPStatus != 429 {
		t.Fatalf("%#v", err)
	}
	expired := providertest.Serve(t, func(providertest.Call) providertest.Answer {
		return providertest.Answer{Status: 401, Body: groq["error.invalid_api_key"]}
	})
	if _, err := New(Options{Origin: expired.URL}).Send(ctx, key, session, message, provider.SendContext{}); provider.CodeOf(err) != "AUTH_FAILED" {
		t.Fatal(err)
	}
}

func TestKeepsTheNewestTurnsWithinTheBudgetAndChecksTheModelID(t *testing.T) {
	var turns []provider.Turn
	for i := range 30 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		turns = append(turns, provider.Turn{Role: role, Text: fmt.Sprintf("t%d", i)})
	}
	kept := History(turns)
	if len(kept) != 20 || kept[0].Text != "t10" || kept[19].Text != "t29" {
		t.Fatalf("%+v", kept)
	}
	if got := History([]provider.Turn{{Role: "user", Text: strings.Repeat("x", 30_000)}}); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	if Model(" llama-3.3-70b-versatile ") != "llama-3.3-70b-versatile" || Model("bad model; rm") != DefaultModel || Model("") != DefaultModel {
		t.Fatal("model ids")
	}
}

func TestQuotaHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-limit-requests", "nope")
	h.Set("x-ratelimit-remaining-requests", "1")
	h.Set("x-ratelimit-limit-tokens", "100")
	h.Set("x-ratelimit-remaining-tokens", "0")
	got := Quota(h, time.UnixMilli(0))
	if len(got) != 1 || got[0].Kind != "tokens" || got[0].Remaining != 0 || got[0].ResetsAt != 0 {
		t.Fatalf("%+v", got)
	}
}
