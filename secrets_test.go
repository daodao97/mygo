package mygo

import (
	"bytes"
	"errors"
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestSecureStoreValidationIsolationAndErrors(t *testing.T) {
	for _, name := range []string{"", "  ", "has\x00nul"} {
		if _, err := NewSecureStore(name, SecureStoreOptions{}); err == nil {
			t.Fatalf("accepted namespace %q", name)
		}
	}
	if _, err := NewSecureStore("test", SecureStoreOptions{Accessibility: "invalid"}); err == nil {
		t.Fatal("accepted unknown access policy")
	}
	a, _ := NewSecureStore(t.Name()+".a", SecureStoreOptions{})
	b, _ := NewSecureStore(t.Name()+".b", SecureStoreOptions{Accessibility: SecretAfterFirstUnlock})
	t.Cleanup(func() { a.Delete("key"); b.Delete("key"); onMain(func() { fb.SecretError = nil }) })
	for _, key := range []string{"", "has\x00nul"} {
		if err := a.Set(key, []byte("secret")); err == nil {
			t.Fatal("accepted invalid key")
		}
	}
	if _, err := (&SecureStore{}).Get("key"); err == nil {
		t.Fatal("accepted zero store")
	}
	if _, err := a.Get("key"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("missing key: %v", err)
	}
	input := []byte{0, 255, 1, 240, 159, 145, 139}
	if err := a.Set("key", input); err != nil {
		t.Fatal(err)
	}
	expected := bytes.Clone(input)
	input[0] = 42
	onMain(func() {
		value, err := a.Get("key")
		if err != nil || !bytes.Equal(value, expected) {
			t.Errorf("binary read on UI thread: %x %v", value, err)
		}
		if len(value) > 0 {
			value[0] = 99
		}
	})
	value, err := a.Get("key")
	if err != nil || !bytes.Equal(value, expected) {
		t.Fatal("read/write aliases caller memory")
	}
	if _, err := b.Get("key"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatal("namespaces are not isolated")
	}
	if err := a.Set("key", nil); err != nil {
		t.Fatal(err)
	}
	if value, err := a.Get("key"); err != nil || len(value) != 0 {
		t.Fatalf("empty value: %x %v", value, err)
	}
	if err := a.Delete("key"); err != nil {
		t.Fatal(err)
	}
	if err := a.Delete("key"); err != nil {
		t.Fatal("delete is not idempotent")
	}
	onMain(func() { fb.SecretError = platform.ErrUnsupported })
	if err := a.Set("key", []byte("no fallback")); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported: %v", err)
	}
	if _, err := a.Get("key"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("get error: %v", err)
	}
	if err := a.Delete("key"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("delete error: %v", err)
	}
}
