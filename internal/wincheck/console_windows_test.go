//go:build windows

package wincheck

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/pushmanhq/pushman-cli/internal/cli"
)

const newHiddenConsole = 0x10 | 0x200 // CREATE_NEW_CONSOLE | CREATE_NEW_PROCESS_GROUP

func consoleHelper(mode string) int {
	dll := syscall.NewLazyDLL("kernel32.dll")
	if mode == "input" {
		file, err := os.Open("CONIN$")
		if err != nil {
			return 1
		}
		defer file.Close()
		var inputMode uint32
		ok, _, _ := dll.NewProc("GetConsoleMode").Call(file.Fd(), uintptr(unsafe.Pointer(&inputMode)))
		if ok == 0 || !cli.IsTerminalFile(file)() {
			return 1
		}
		return 0
	}
	pid, err := strconv.Atoi(mode)
	if err != nil || pid <= 0 || pid == os.Getpid() {
		return 1
	}
	// Detach this short-lived helper from its inherited console; the parent
	// test process keeps its own console and signal handlers unchanged.
	dll.NewProc("FreeConsole").Call()
	ok, _, _ := dll.NewProc("AttachConsole").Call(uintptr(pid))
	if ok == 0 {
		return 2
	}
	defer dll.NewProc("FreeConsole").Call()
	// CTRL_BREAK can target only the test child's new process group. CTRL_C
	// cannot be targeted this way and would affect other console processes.
	ok, _, _ = dll.NewProc("GenerateConsoleCtrlEvent").Call(1, uintptr(pid))
	if ok == 0 {
		return 3
	}
	return 0
}

func TestWindowsNativeConsoleInputHandle(t *testing.T) {
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
	command.Env = append(os.Environ(), "PUSHMAN_WIN_CONSOLE_HELPER=input")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: newHiddenConsole, HideWindow: true}
	if err := command.Run(); err != nil {
		t.Fatalf("real hidden console input handle test failed: %v", err)
	}
}

func TestWindowsNativeLoginInterrupt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/device-authorizations" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"device_code":"synthetic-code","user_code":"TEST-CODE","verification_uri":"https://example.com/activate","verification_uri_complete":"https://example.com/activate?code=TEST-CODE","expires_in":600,"interval":30}`)
	}))
	defer server.Close()
	command := cliCommand(t, server.URL, "login", "--no-browser")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: newHiddenConsole, HideWindow: true}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "Login code:") {
		command.Process.Kill()
		command.Wait()
		t.Fatal("no-browser login did not reach its synthetic challenge")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	helper := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	helper.Env = append(os.Environ(), fmt.Sprintf("PUSHMAN_WIN_CONSOLE_HELPER=%d", command.Process.Pid))
	if err := helper.Run(); err != nil {
		command.Process.Kill()
		command.Wait()
		t.Fatalf("could not direct a console event to the test child: %v", err)
	}
	err = command.Wait()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 130 || !strings.Contains(stderr.String(), "context canceled") {
		t.Fatal("native console interrupt did not stop login with exit 130")
	}
}

func TestWindowsShellExitCodes(t *testing.T) {
	for _, shell := range []string{"pwsh", "powershell", "cmd"} {
		t.Run(shell, func(t *testing.T) {
			for _, arg := range []string{"version", "help", "--not-a-pushman-flag"} {
				var command *exec.Cmd
				if shell == "cmd" {
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()
					command = exec.CommandContext(ctx, "cmd")
					command.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d /s /c \"\"" + executable + "\" " + arg + "\""}
					command.Env = isolatedEnvironment("http://127.0.0.1:1")
				} else {
					script := "$ProgressPreference = 'SilentlyContinue'\n$ErrorActionPreference = 'Continue'\n& " + psLiteral(executable) + " " + arg + "\nexit $LASTEXITCODE"
					command = powerShellCommand(t, shell, script, "http://127.0.0.1:1")
				}
				stdout, stderr, code := runCommand(t, command)
				if arg == "--not-a-pushman-flag" {
					if code != 2 || stdout != "" || !strings.Contains(stderr, "unknown flag") {
						t.Fatalf("%s failed to preserve usage exit/stdout/stderr", shell)
					}
				} else if code != 0 || stdout == "" || stderr != "" {
					t.Fatalf("%s %s failed its success contract", shell, arg)
				}
			}
		})
	}
}
