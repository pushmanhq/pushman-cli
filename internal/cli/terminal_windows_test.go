//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsTerminalFileHandles(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "input.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if IsTerminalFile(file)() {
		t.Fatal("ordinary redirected file was detected as a terminal")
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	write.Close()
	if IsTerminalFile(read)() {
		t.Fatal("pipe at EOF was detected as a terminal")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	// Preserve the existing character-device rule, including NUL. This does
	// not assert that NUL is an interactive Windows console.
	if !IsTerminalFile(null)() {
		t.Fatal("NUL no longer follows the character-device input rule")
	}
	closed, err := os.Open(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if IsTerminalFile(closed)() {
		t.Fatal("closed handle was detected as a terminal")
	}
}
