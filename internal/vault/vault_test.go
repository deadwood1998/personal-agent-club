package vault

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestMemory(t *testing.T) {
	ctx := context.Background()
	v := NewMemory()
	if _, ok, _ := v.Get(ctx, "a000000000001"); ok {
		t.Fatal("empty vault has a credential")
	}
	if err := v.Put(ctx, "a000000000001", "secret-value-1"); err != nil {
		t.Fatal(err)
	}
	if c, ok, _ := v.Get(ctx, "a000000000001"); !ok || c != "secret-value-1" {
		t.Fatalf("got %q %v", c, ok)
	}
	for _, s := range []string{fmt.Sprint(v), fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v)} {
		if strings.Contains(s, "secret-value-1") {
			t.Fatalf("formatting leaks the credential: %s", s)
		}
	}
	_ = v.Delete(ctx, "a000000000001")
	if _, ok, _ := v.Get(ctx, "a000000000001"); ok {
		t.Fatal("deleted credential still there")
	}
}
