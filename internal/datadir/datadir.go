// Package datadir is club's data directory on disk:
//
//	<data>/key                          the vault key (unless --key-file puts it elsewhere), 0600
//	<data>/club.lock                    held by the one process that owns the directory
//	<data>/spaces/<space>/club.db       the space's SQLite store (WAL)
//	<data>/spaces/<space>/credentials.enc  the space's credentials, AES-256-GCM under the key
//
// One process owns a data directory at a time (a serve, the Persona extension, or an import): Open takes an
// exclusive lock and fails with ErrInUse while another holds it.
package datadir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/nwverse/personal-agent-club/internal/store"
	"github.com/nwverse/personal-agent-club/internal/vault"
)

// ErrInUse is Open's error while another process owns the directory.
var ErrInUse = errors.New("the data directory is in use by another personal-agent-club process (serve or the Persona extension); stop it first")

// Dir is an open data directory.
type Dir struct {
	root string
	key  []byte
	lock *os.File

	mu     sync.Mutex
	stores []*store.SQLite
	closed bool
}

// KeyPath is the default key file of a data directory.
func KeyPath(root string) string { return filepath.Join(root, "key") }

// SpacePath is a space's directory.
func SpacePath(root, space string) string { return filepath.Join(root, "spaces", space) }

// Open opens (creating it, 0700) the data directory at root and takes its lock. keyFile "" is <root>/key; a missing
// key file is created with a fresh key.
func Open(root, keyFile string) (*Dir, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(root, "club.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrInUse
		}
		return nil, fmt.Errorf("lock %s: %w", root, err)
	}
	if keyFile == "" {
		keyFile = KeyPath(root)
	}
	key, err := vault.LoadKey(keyFile, true)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	return &Dir{root: root, key: key, lock: lock}, nil
}

// Root is the directory's path.
func (d *Dir) Root() string { return d.root }

// OpenSpace opens a space's store and vault: the core.Options.OpenSpace of a club kept on disk.
func (d *Dir) OpenSpace(name string) (store.Store, vault.Vault, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, nil, errors.New("the data directory is closed")
	}
	dir := SpacePath(d.root, name)
	st, err := store.OpenSQLite(filepath.Join(dir, "club.db"))
	if err != nil {
		return nil, nil, err
	}
	v, err := vault.OpenFile(filepath.Join(dir, "credentials.enc"), d.key, name)
	if err != nil {
		_ = st.Close()
		return nil, nil, err
	}
	d.stores = append(d.stores, st)
	return st, v, nil
}

// Close closes the stores and releases the lock. Close the club first.
func (d *Dir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	var errs []error
	for _, s := range d.stores {
		errs = append(errs, s.Close())
	}
	errs = append(errs, d.lock.Close())
	return errors.Join(errs...)
}

// HasSpace says whether a space exists in the directory at root.
func HasSpace(root, space string) bool {
	_, err := os.Stat(filepath.Join(SpacePath(root, space), "club.db"))
	return err == nil
}

// OpenReadOnly opens a space's store read-only, without the lock: for reading while the owner runs.
func OpenReadOnly(root, space string) (*store.SQLite, error) {
	if !HasSpace(root, space) {
		return nil, fmt.Errorf("no space %q in %s", space, root)
	}
	return store.OpenSQLiteReadOnly(filepath.Join(SpacePath(root, space), "club.db"))
}
