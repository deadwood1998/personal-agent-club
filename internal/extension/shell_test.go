package extension

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nwverse/persona/sdk/go/ext"
	"github.com/nwverse/persona/sdk/go/exttest"
	"github.com/nwverse/persona/sdk/go/wire"

	pac "github.com/nwverse/personal-agent-club"
	"github.com/nwverse/personal-agent-club/internal/core"
	"github.com/nwverse/personal-agent-club/internal/protocol"
	"github.com/nwverse/personal-agent-club/internal/provider"
	"github.com/nwverse/personal-agent-club/internal/provider/echo"
	"github.com/nwverse/personal-agent-club/internal/store"
)

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func storePatch(level string) store.AccountPatch { return store.AccountPatch{AssistantControl: &level} }

const demoCredential = "echo-synthetic-0001"

func start(t *testing.T) (*exttest.Host, *core.Club) {
	t.Helper()
	club := core.New(core.Options{Registry: provider.NewRegistry(), FollowUps: []time.Duration{}})
	h := exttest.Start(t, pac.Manifest, exttest.Serve(t, Program(club, pac.Version())), json.RawMessage(`{"demo_echo":true}`))
	return h, club
}

func render(t *testing.T, h *exttest.Host, st pageState) *wire.View {
	t.Helper()
	var v wire.View
	if err := h.Call("ui.render", wire.UIParams{Contribution: AccountsUI, Placement: wire.PlacementPluginPage, Size: "full",
		UIVersion: 6, Locale: "zh-Hans", State: stateOf(st)}, &v); err != nil {
		t.Fatalf("ui.render %+v: %v", st, err)
	}
	checkView(t, &v)
	return &v
}

func act(t *testing.T, h *exttest.Host, st pageState, action string, values map[string]string) *wire.ActionResult {
	t.Helper()
	var res wire.ActionResult
	if err := h.Call("ui.action", wire.UIParams{Contribution: AccountsUI, Placement: wire.PlacementPluginPage, Size: "full",
		UIVersion: 6, Locale: "zh-Hans", State: stateOf(st), Action: action, Values: values}, &res); err != nil {
		t.Fatalf("ui.action %s: %v", action, err)
	}
	if res.View != nil {
		checkView(t, res.View)
	}
	return &res
}

func stateOf(st pageState) string {
	if st == (pageState{}) {
		return ""
	}
	b, _ := json.Marshal(st)
	return string(b)
}

func checkView(t *testing.T, v *wire.View) {
	t.Helper()
	raw, _ := json.Marshal(v)
	if err := exttest.CheckView(raw, false); err != nil {
		t.Fatalf("view the apps cannot draw: %v\n%s", err, raw)
	}
	if strings.Contains(string(raw), demoCredential) {
		t.Fatalf("a view carries the credential: %s", raw)
	}
}

func text(v *wire.View) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestAccountsPage(t *testing.T) {
	h, club := start(t)
	v := render(t, h, pageState{})
	if !strings.Contains(text(v), "还没有账号") || !strings.Contains(text(v), "Echo") {
		t.Fatalf("empty list: %s", text(v))
	}

	// pairing: the key goes in and comes back nowhere
	if res := act(t, h, pageState{Page: pagePair, Provider: echo.Name}, actPair, map[string]string{fieldCredential: "  "}); !strings.Contains(res.Toast, "无效") {
		t.Fatalf("empty key: %+v", res)
	}
	res := act(t, h, pageState{Page: pagePair, Provider: echo.Name}, actPair, map[string]string{fieldCredential: demoCredential, fieldName: "Demo"})
	if !res.Pop || !strings.Contains(res.Toast, "Demo") {
		t.Fatalf("pair: %+v", res)
	}
	st, err := club.Space(context.Background(), core.DefaultSpace)
	if err != nil {
		t.Fatal(err)
	}
	status, _ := st.Status(context.Background())
	if len(status.Accounts) != 1 {
		t.Fatalf("accounts: %+v", status.Accounts)
	}
	id := status.Accounts[0].AccountID
	if v := render(t, h, pageState{}); !strings.Contains(text(v), "Demo") || !strings.Contains(text(v), "正常") {
		t.Fatalf("list: %s", text(v))
	}
	page := pageState{Page: pageAccount, Account: id}
	if v := render(t, h, page); !strings.Contains(text(v), fieldControl) || !strings.Contains(text(v), "立即同步") {
		t.Fatalf("account: %s", text(v))
	}
	if res := act(t, h, page, actSync, nil); !strings.Contains(res.Toast, "已同步") {
		t.Fatalf("sync: %+v", res)
	}
	// pause and the assistant control level go through club.accounts.update
	if res := act(t, h, page, actPause, nil); !strings.Contains(res.Toast, "已更新") || !strings.Contains(text(res.View), "已暂停") {
		t.Fatalf("pause: %+v", res)
	}
	if res := act(t, h, page, actResume, nil); !strings.Contains(res.Toast, "已更新") || !strings.Contains(text(res.View), "正常") {
		t.Fatalf("resume: %+v", res)
	}
	if res := act(t, h, page, actControl, map[string]string{fieldControl: "read"}); !strings.Contains(res.Toast, "已更新") {
		t.Fatalf("control: %+v", res)
	}
	if c, _ := st.AssistantControl(context.Background(), id); c != core.ControlRead {
		t.Fatalf("control is %s", c)
	}
	// without the grok_bot provider in this club, its sign-in says it is not available
	if res := act(t, h, pageState{Page: pageGrok}, actGrokStart, nil); !strings.Contains(res.Toast, "不支持") {
		t.Fatalf("grok start: %+v", res)
	}
	if res := act(t, h, page, actRemove, nil); !res.Pop {
		t.Fatalf("remove: %+v", res)
	}
	if v := render(t, h, page); !strings.Contains(text(v), "已不在") {
		t.Fatalf("removed account: %s", text(v))
	}
}

func TestAccountsPageEnglish(t *testing.T) {
	h, _ := start(t)
	var v wire.View
	if err := h.Call("ui.render", wire.UIParams{Contribution: AccountsUI, Placement: wire.PlacementPluginPage, Size: "full",
		UIVersion: 3, Locale: "en"}, &v); err != nil {
		t.Fatal(err)
	}
	checkView(t, &v)
	if !strings.Contains(text(&v), "No accounts yet") {
		t.Fatalf("en: %s", text(&v))
	}
}

func TestConnectorChecks(t *testing.T) {
	club := core.New(core.Options{Registry: provider.NewRegistry()})
	h := exttest.CheckConnectors(t, pac.Manifest, exttest.Serve(t, Program(club, pac.Version())))
	var items []wire.ConnectorItem
	if err := json.Unmarshal(h.Init.Contributes["connectors"], &items); err != nil || len(items) != 1 || items[0].ID != Connector {
		t.Fatalf("initialize does not say it has the connector: %s (%v)", h.Init.Contributes["connectors"], err)
	}
	// the hub tells an unknown method from one the source cannot do
	err := h.Call("agentsrc.nonexistent_method", map[string]any{"contribution": Contribution}, nil)
	if e, ok := err.(*ext.Error); !ok || e.Code != wire.ErrorCodeUnknownMethod {
		t.Fatalf("unknown agentsrc method: %v", err)
	}
}

func setControl(t *testing.T, club *core.Club, id string, level core.Control) {
	t.Helper()
	s, _ := club.Space(context.Background(), core.DefaultSpace)
	v := string(level)
	if err := s.Store().UpdateAccount(context.Background(), id, storePatch(v), 1); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantTools(t *testing.T) {
	h, club := start(t)
	act(t, h, pageState{Page: pagePair, Provider: echo.Name}, actPair, map[string]string{fieldCredential: demoCredential})
	space, _ := club.Space(context.Background(), core.DefaultSpace)
	status, _ := space.Status(context.Background())
	id := status.Accounts[0].AccountID
	sessions, _ := space.Sessions(context.Background())
	var main, notes string
	for _, s := range sessions.Sessions {
		if s.CanSend {
			main = s.SessionKey
		} else {
			notes = s.SessionKey
		}
	}
	if main == "" || notes == "" {
		t.Fatalf("sessions: %+v", sessions)
	}

	callTool := func(name string, args any) (string, error) {
		raw, _ := json.Marshal(args)
		var res wire.ConnectorCallResult
		err := h.Call("connector.tools.call", wire.ConnectorCallParams{Contribution: Connector, Name: name, Args: raw}, &res)
		return res.Text, err
	}
	describe := func(name string, args any) (wire.ConnectorDescribeResult, error) {
		raw, _ := json.Marshal(args)
		var res wire.ConnectorDescribeResult
		err := h.Call("connector.actions.describe", wire.ConnectorCallParams{Contribution: Connector, Name: name, Args: raw}, &res)
		return res, err
	}
	do := func(name string, args any) (string, error) {
		raw, _ := json.Marshal(args)
		var res wire.ConnectorCallResult
		err := h.Call("connector.actions.do", wire.ConnectorDoParams{Contribution: Connector, Name: name, Args: raw, RequestID: "req-1"}, &res)
		return res.Text, err
	}

	// off (the default): the account does not exist for the assistant
	var cs wire.ConnectorStatus
	if err := h.Call("connector.status", wire.ConnectorParams{Contribution: Connector}, &cs); err != nil || cs.State != wire.ConnectorStateNotConfigured {
		t.Fatalf("status at off: %+v %v", cs, err)
	}
	if out, err := callTool(toolList, map[string]any{}); err != nil || strings.Contains(out, id) {
		t.Fatalf("list at off: %q %v", out, err)
	}
	if _, err := callTool(toolRead, map[string]any{"session_key": main}); err == nil {
		t.Fatal("read at off answered")
	}

	// read: list and read, no writes
	setControl(t, club, id, core.ControlRead)
	if err := h.Call("connector.status", wire.ConnectorParams{Contribution: Connector}, &cs); err != nil || cs.State != wire.ConnectorStateOk {
		t.Fatalf("status at read: %+v %v", cs, err)
	}
	if out, err := callTool(toolList, map[string]any{"provider": echo.Name}); err != nil || !strings.Contains(out, main) {
		t.Fatalf("list at read: %q %v", out, err)
	}
	if out, err := callTool(toolRead, map[string]any{"session_key": main, "turns": 2}); err != nil || !strings.Contains(out, "不可信") {
		t.Fatalf("read at read: %q %v", out, err)
	}
	if _, err := describe(toolSend, map[string]any{"session_key": main, "text": "hi"}); err == nil {
		t.Fatal("send described at read")
	}
	if _, err := do(toolSend, map[string]any{"session_key": main, "text": "hi"}); err == nil {
		t.Fatal("send done at read")
	}

	// confirm: writes, after the gate
	setControl(t, club, id, core.ControlConfirm)
	d, err := describe(toolSend, map[string]any{"session_key": main, "text": "hello"})
	if err != nil || d.Summary != "hello" {
		t.Fatalf("describe send: %+v %v", d, err)
	}
	if _, err := describe(toolSend, map[string]any{"session_key": notes, "text": "hello"}); err == nil {
		t.Fatal("send to a read-only session described")
	}
	out, err := do(toolSend, map[string]any{"session_key": main, "text": "hello"})
	if err != nil || !strings.Contains(out, "已发出") {
		t.Fatalf("send: %q %v", out, err)
	}
	if again, err := do(toolSend, map[string]any{"session_key": main, "text": "hello"}); err != nil || again != out {
		t.Fatalf("the same request again: %q %v", again, err)
	}
	if out, err := callTool(toolRead, map[string]any{"session_key": main}); err != nil || !strings.Contains(out, "echo: hello") {
		t.Fatalf("read after send: %q %v", out, err)
	}
	// start and interrupt: not in this core yet
	if _, err := describe(toolStart, map[string]any{"account_id": id, "text": "x"}); err == nil || !strings.Contains(err.Error(), "还不能") {
		t.Fatalf("start: %v", err)
	}
	if _, err := describe(toolInterrupt, map[string]any{"session_key": main}); err == nil || !strings.Contains(err.Error(), "不能打断") {
		t.Fatalf("interrupt: %v", err)
	}
	if d, err := describe(toolInterrupt, map[string]any{"session_key": main, "then": "stop"}); err != nil || !strings.Contains(d.Action.Name, "排队") {
		t.Fatalf("interrupt with then: %+v %v", d, err)
	}
}

func TestClientIDIsAUUID(t *testing.T) {
	a, b := clientID("r1"), clientID("r1")
	if a != b || !core.ValidClientID(a) || clientID("r2") == a {
		t.Fatalf("client ids %q %q", a, b)
	}
}

func TestReasonHidesTheSecret(t *testing.T) {
	got := reason("zh", protocol.Invalid("bad key "+demoCredential), demoCredential)
	if strings.Contains(got, demoCredential) || !strings.HasPrefix(got, "无效") {
		t.Fatalf("reason: %q", got)
	}
}

func TestEnsureKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), KeyFileName)
	if err := EnsureKey(path); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() != keySize || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", fi, err)
	}
	first, _ := os.ReadFile(path)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureKey(path); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(path)
	second, _ := os.ReadFile(path)
	if fi.Mode().Perm() != 0o600 || string(first) != string(second) {
		t.Fatal("an existing key was not kept as it is, at 0600")
	}
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureKey(path); err == nil {
		t.Fatal("a key of the wrong size was accepted")
	}
}

func TestNewClubPreparesTheDataDirectory(t *testing.T) {
	data := t.TempDir()
	t.Setenv("PERSONA_EXTENSION_DATA", data)
	var seen Host
	old := Configure
	Configure = func(h Host, _ *core.Options) error { seen = h; return nil }
	t.Cleanup(func() { Configure = old })
	club, h, err := NewClub("0.0.0", discard())
	if err != nil {
		t.Fatal(err)
	}
	club.Close()
	if h.DataDir != data || seen.KeyFile != filepath.Join(data, KeyFileName) || seen.HTTP == nil {
		t.Fatalf("host %+v", seen)
	}
	if fi, err := os.Stat(h.SpaceDir(core.DefaultSpace)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("space directory: %v %v", fi, err)
	}
}

// The providers' client goes out through the proxy the hub names in the environment, as the sandbox requires. The
// environment is read once per process, so the check runs in a child.
func TestHTTPClientUsesTheHubProxy(t *testing.T) {
	if os.Getenv("PAC_PROXY_CHILD") != "" {
		res, err := HTTPClient().Get("http://api.groq.com/openai/v1/models")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusTeapot {
			t.Fatalf("status %d", res.StatusCode)
		}
		return
	}
	hosts := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts <- r.Host
		w.WriteHeader(http.StatusTeapot)
	}))
	defer proxy.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHTTPClientUsesTheHubProxy$")
	cmd.Env = append(os.Environ(), "PAC_PROXY_CHILD=1", "HTTP_PROXY="+proxy.URL, "http_proxy="+proxy.URL, "NO_PROXY=", "no_proxy=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	select {
	case host := <-hosts:
		if host != "api.groq.com" {
			t.Fatalf("the proxy saw %q", host)
		}
	default:
		t.Fatal("the request did not go through the proxy")
	}
}
