// Command personal-agent-club runs club: started by a Persona hub as an extension (no arguments; the hub speaks the
// extension protocol on stdin and stdout), or on its own with `serve` for the Lark gateway.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nwverse/persona/sdk/go/ext"

	pac "github.com/nwverse/personal-agent-club"
	"github.com/nwverse/personal-agent-club/internal/core"
	"github.com/nwverse/personal-agent-club/internal/extension"
	"github.com/nwverse/personal-agent-club/internal/protocol"
	"github.com/nwverse/personal-agent-club/internal/provider"
	"github.com/nwverse/personal-agent-club/internal/provider/echo"
	"github.com/nwverse/personal-agent-club/internal/serve"
)

// version may be set at build time (-ldflags "-X main.version=..."); by default it is the manifest's.
var version = ""

const usage = `personal-agent-club: personal agents as one source, for Persona and the Lark gateway.

Usage:
  personal-agent-club                 run as a Persona extension (the hub starts it this way)
  personal-agent-club serve [flags]   serve the protocol over HTTP on a unix socket (and optionally TCP)
  personal-agent-club version         print the version
  personal-agent-club manifest        print the Persona extension manifest

Run "personal-agent-club serve -h" for serve's flags.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if version == "" {
		version = pac.Version()
	}
	if len(args) == 0 || args[0] == "manifest" {
		if len(args) == 0 && isTerminal(os.Stdin) && ext.Env().ID == "" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		club := core.New(core.Options{Version: version, Registry: provider.NewRegistry(), Log: logger(stderr)})
		ext.Main(extension.Program(club, version))
		return 0
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "personal-agent-club %s (protocol %d)\n", version, protocol.Version)
		return 0
	case "serve":
		return runServe(args[1:], stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
	return 2
}

func runServe(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	home, _ := os.UserHomeDir()
	data := fs.String("data", filepath.Join(home, ".personal-agent-club"), "data directory (this version keeps its stores in memory; the default socket lives here)")
	socket := fs.String("socket", "", "unix socket path (default <data>/club.sock; \"none\" to listen on TCP only)")
	listen := fs.String("listen", "", "TCP address to listen on as well, e.g. 127.0.0.1:7655 (needs -tokens)")
	tokensFile := fs.String("tokens", "", `tokens file: {"read": [...], "write": [...], "admin": [...]}; required with -listen`)
	demoEcho := fs.Bool("demo-echo", false, "register the in-memory echo provider (for demos and tests only)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	cfg := serve.Config{Socket: *socket, Addr: *listen}
	switch cfg.Socket {
	case "":
		cfg.Socket = filepath.Join(*data, "club.sock")
	case "none":
		cfg.Socket = ""
	}
	if *tokensFile != "" {
		t, err := serve.LoadTokens(*tokensFile)
		if err != nil {
			fmt.Fprintln(stderr, "serve:", err)
			return 1
		}
		cfg.Tokens = t
	}
	if err := cfg.Check(); err != nil {
		fmt.Fprintln(stderr, "serve:", err)
		return 1
	}
	log := logger(stderr)
	reg := provider.NewRegistry()
	if *demoEcho {
		reg.Register(echo.New())
	}
	club := core.New(core.Options{Version: version, Registry: reg, Log: log})
	defer club.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	addrs, done, err := serve.Run(ctx, club, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "serve:", err)
		return 1
	}
	for _, a := range addrs {
		log.Info("listening", "network", a.Network(), "address", a.String(), "tokens", cfg.Tokens != nil)
	}
	if err := <-done; err != nil {
		fmt.Fprintln(stderr, "serve:", err)
		return 1
	}
	return 0
}

func logger(w io.Writer) *slog.Logger { return slog.New(slog.NewTextHandler(w, nil)) }

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
