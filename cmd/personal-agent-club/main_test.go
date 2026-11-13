package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwverse/persona/sdk/go/exttest"

	pac "github.com/nwverse/personal-agent-club"
)

func TestVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "protocol 1") {
		t.Fatalf("version: %d %q %q", code, out.String(), errOut.String())
	}
}

func TestServeRefusesTCPWithoutTokens(t *testing.T) {
	for _, args := range [][]string{
		{"serve", "-socket", "none", "-listen", "127.0.0.1:0"},
		{"serve", "-socket", t.TempDir() + "/c.sock", "-listen", "127.0.0.1:0"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "tokens") {
			t.Fatalf("%v: %d %q", args, code, errOut.String())
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"bogus"}, &out, &errOut); code != 2 {
		t.Fatalf("bogus: %d", code)
	}
}

// The built program, started the way the hub starts it, holds to the extension protocol.
func TestBinaryAsExtension(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the program")
	}
	bin := filepath.Join(t.TempDir(), "personal-agent-club")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	res := exttest.Check(t, pac.Manifest, exttest.Program(t, bin))
	if res.Version != pac.Version() {
		t.Fatalf("initialize answered version %q", res.Version)
	}
}
