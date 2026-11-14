package extension

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/nwverse/persona/sdk/go/ext"

	"github.com/nwverse/personal-agent-club/internal/core"
	"github.com/nwverse/personal-agent-club/internal/provider"
)

// Where the Persona shell keeps its data. The hub gives every extension one data directory (PERSONA_EXTENSION_DATA,
// also HOME inside the sandbox, deleted on uninstall unless kept); club's lives there:
//
//	<data>/club.key            the key of the credential files, 0600
//	<data>/spaces/default/     the one space this shell serves
const (
	KeyFileName = "club.key"
	SpacesDir   = "spaces"
	keySize     = 32
)

// Host is what the Persona shell hands the core: where its data goes, and the HTTP client every provider must use.
type Host struct {
	// DataDir is the extension's data directory; "" when the program does not run under a hub (tests, a terminal).
	DataDir string
	// KeyFile is the credential key's file in DataDir, created (0600) before Configure is called; "" without DataDir.
	KeyFile string
	// HTTP goes out through the hub's egress proxy when there is one: inside the sandbox it is the only way out.
	HTTP *http.Client
}

// SpaceDir is a space's directory under DataDir.
func (h Host) SpaceDir(space string) string { return filepath.Join(h.DataDir, SpacesDir, space) }

// Configure is the seam between this shell and the core's persistent storage: it fills in opts (OpenSpace, and the
// providers' HTTP client once the core takes one) from h. The core's SQLite store and encrypted vault are being
// written alongside this shell, and their entry point is not settled; until they land this leaves opts as they are,
// so every space stays in memory and is empty again after a restart. Merging means replacing this function, nothing
// else in the shell.
var Configure = func(h Host, opts *core.Options) error { return nil }

// NewClub is the club the Persona shell serves, built from the program's environment: the data directory and key
// file prepared (when the hub gave a data directory) and handed to Configure.
func NewClub(version string, log *slog.Logger) (*core.Club, Host, error) {
	h := Host{DataDir: ext.Env().Data, HTTP: HTTPClient()}
	if h.DataDir != "" {
		if err := prepare(&h); err != nil {
			return nil, h, err
		}
	} else {
		log.Warn("no extension data directory (PERSONA_EXTENSION_DATA): keeping everything in memory")
	}
	opts := core.Options{Version: version, Registry: provider.NewRegistry(), Log: log}
	if err := Configure(h, &opts); err != nil {
		return nil, h, fmt.Errorf("storage: %w", err)
	}
	return core.New(opts), h, nil
}

// prepare makes the data layout: the default space's directory (0700) and the key file.
func prepare(h *Host) error {
	if err := os.MkdirAll(h.SpaceDir(core.DefaultSpace), 0o700); err != nil {
		return fmt.Errorf("data directory: %w", err)
	}
	h.KeyFile = filepath.Join(h.DataDir, KeyFileName)
	return EnsureKey(h.KeyFile)
}

// EnsureKey makes sure path holds a key: 32 random bytes, written once with mode 0600 and never rewritten. An
// existing key file is kept as it is; one others can read is narrowed to 0600, and one of the wrong size is an error
// rather than replaced, since replacing it would lose every credential sealed with it.
func EnsureKey(path string) error {
	fi, err := os.Lstat(path)
	switch {
	case err == nil:
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("key file %s is not a regular file", filepath.Base(path))
		}
		if fi.Size() != keySize {
			return fmt.Errorf("key file %s holds %d bytes, not %d", filepath.Base(path), fi.Size(), keySize)
		}
		if fi.Mode().Perm() != 0o600 {
			return os.Chmod(path, 0o600)
		}
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(key); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Link, not rename: two starts racing never replace a key the other already wrote.
	if err := os.Link(tmp.Name(), path); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

// HTTPClient is the client providers use. Inside the hub's sandbox the only way out is the egress proxy the hub
// names in HTTPS_PROXY / HTTP_PROXY (and only to the hosts in permissions.network), so the transport takes its proxy
// from the environment, as http.DefaultTransport does; a transport built from scratch without Proxy would dial
// directly and be refused by the sandbox. No overall timeout: a provider sets its own per request.
func HTTPClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: time.Second,
	}}
}
