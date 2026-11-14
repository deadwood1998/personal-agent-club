package datadir

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwverse/personal-agent-club/internal/protocol"
	"github.com/nwverse/personal-agent-club/internal/store"
)

func TestOneOwnerAndDataThatLasts(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	d, err := Open(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root, ""); !errors.Is(err, ErrInUse) {
		t.Fatalf("a second owner: %v", err)
	}
	st, v, err := d.OpenSpace("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertAccount(ctx, store.Account{AccountID: "a000000000001", Provider: "groq", DisplayName: "Groq",
		Status: protocol.AccountActive, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := v.Put(ctx, "a000000000001", "fixture-key-not-real"); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := ro.ListAccounts(ctx); len(list) != 1 {
		t.Fatalf("%+v", list)
	}
	if err := ro.InsertAccount(ctx, store.Account{AccountID: "a000000000002", Provider: "x", Status: "active"}); err == nil {
		t.Fatal("a read-only store wrote")
	}
	_ = ro.Close()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{KeyPath(root), filepath.Join(root, "spaces", "default", "club.db"), filepath.Join(root, "spaces", "default", "credentials.enc")} {
		fi, err := os.Stat(p)
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", p, fi, err)
		}
	}
	d, err = Open(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	st, v, err = d.OpenSpace("default")
	if err != nil {
		t.Fatal(err)
	}
	if a, ok, _ := st.Account(ctx, "a000000000001"); !ok || a.Provider != "groq" {
		t.Fatalf("%+v", a)
	}
	if c, ok, _ := v.Get(ctx, "a000000000001"); !ok || c != "fixture-key-not-real" {
		t.Fatal("the credential did not survive a restart")
	}
	if _, err := OpenReadOnly(root, "nope"); err == nil {
		t.Fatal("opened a missing space")
	}
}

func TestAKeyFileElsewhere(t *testing.T) {
	root, keyDir := t.TempDir(), t.TempDir()
	d, err := Open(root, filepath.Join(keyDir, "club.key"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := os.Stat(filepath.Join(keyDir, "club.key")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(KeyPath(root)); err == nil {
		t.Fatal("the default key was created as well")
	}
}
