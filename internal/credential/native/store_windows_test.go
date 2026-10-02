//go:build windows

package native

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/pushmanhq/pushman-cli/internal/credential"
)

func TestNativeKeyringLifecycle(t *testing.T) {
	if os.Getenv("PUSHMAN_TEST_NATIVE_KEYRING") != "1" {
		t.Skip("NOT RUN: set PUSHMAN_TEST_NATIVE_KEYRING=1 to test an isolated native Windows entry")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	service := "com.pushman.test." + hex.EncodeToString(nonce[:])
	store := credential.NewKeyring(service)
	// Register cleanup before writing, even if a later assertion fails.
	t.Cleanup(func() {
		if err := store.Delete(); err != nil {
			t.Errorf("isolated native entry cleanup failed: %v", err)
		}
	})
	if _, err := store.Get(); !errors.Is(err, credential.ErrNotFound) {
		t.Fatalf("isolated entry missing lookup: %v", err)
	}
	secret := "synthetic-" + hex.EncodeToString(nonce[:])
	if err := store.Set(secret); err != nil {
		t.Fatalf("native write failed (unverified store): %v", err)
	}
	if got, err := store.Get(); err != nil || got != secret {
		t.Fatal("native read did not return the synthetic value")
	}
	// A second test process proves persistence without sending the secret in
	// process arguments, environment variables, or output.
	digest := sha256.Sum256([]byte(secret))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeKeyringHelper$")
	command.Env = append(os.Environ(),
		"PUSHMAN_TEST_KEYRING_SERVICE="+service,
		"PUSHMAN_TEST_KEYRING_DIGEST="+hex.EncodeToString(digest[:]),
	)
	if err := command.Run(); err != nil {
		t.Fatalf("native read after process restart failed: %v", err)
	}
	other := credential.NewKeyring(service + ".dev")
	if _, err := other.Get(); !errors.Is(err, credential.ErrNotFound) {
		t.Fatal("isolated namespaces unexpectedly shared a credential")
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(); !errors.Is(err, credential.ErrNotFound) {
		t.Fatal("native deleted entry still exists")
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("native delete is not idempotent: %v", err)
	}
	t.Log("PASS: isolated native set/get/restart/namespace separation/delete/cleanup; no production entries accessed")
}

func TestNativeKeyringHelper(t *testing.T) {
	service := os.Getenv("PUSHMAN_TEST_KEYRING_SERVICE")
	if service == "" {
		t.Skip("native restart helper")
	}
	if !strings.HasPrefix(service, "com.pushman.test.") {
		os.Exit(1)
	}
	value, err := credential.NewKeyring(service).Get()
	if err != nil {
		os.Exit(1)
	}
	digest := sha256.Sum256([]byte(value))
	if hex.EncodeToString(digest[:]) != os.Getenv("PUSHMAN_TEST_KEYRING_DIGEST") {
		os.Exit(1)
	}
	os.Exit(0)
}
