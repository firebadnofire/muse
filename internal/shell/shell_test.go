package shell

import (
	"github.com/creack/pty"
	"io"
	"muse/internal/composer"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnsupported(t *testing.T) {
	if Supported("/bin/fish") {
		t.Fatal("fish")
	}
	if _, e := Integration("fish", "muse"); e == nil {
		t.Fatal("must use fallback")
	}
}

// A real PTY drives native line-editor widgets; no LLM is involved. The fixture
// includes shell metacharacters whose effects would be visible if staging ran it.
func TestNativeStaging(t *testing.T) {
	for _, name := range []string{"bash", "zsh"} {
		t.Run(name, func(t *testing.T) {
			bin, e := exec.LookPath(name)
			if e != nil {
				t.Skip(name + " not installed")
			}
			for _, tc := range []struct {
				name     string
				existing bool
				suffix   string
			}{{"empty", false, ""}, {"existing", true, ""}, {"multiline", false, "\nprintf second"}} {
				existing := tc.existing
				t.Run(tc.name, func(t *testing.T) {
					dir := t.TempDir()
					marker := filepath.Join(dir, "EXECUTED")
					capture := filepath.Join(dir, "buffer")
					called := filepath.Join(dir, "called")
					payload := "echo $(touch " + composer.Quote(marker) + "); touch " + composer.Quote(marker) + tc.suffix
					fake := filepath.Join(dir, "muse-fixture")
					os.WriteFile(fake, []byte("#!/bin/sh\nprintf called > "+composer.Quote(called)+"\nprintf %s "+composer.Quote(payload)+"\n"), 0700)
					hook, _ := Integration(name, fake)
					rc := filepath.Join(dir, "rc")
					script := "PS1='READY> '\n" + hook
					if name == "bash" {
						script += "_capture() { printf %s \"$READLINE_LINE\" > " + composer.Quote(capture) + "; }; bind -x '\"\\C-xv\":_capture'\n"
					} else {
						script += "_capture() { print -rn -- \"$BUFFER\" > " + composer.Quote(capture) + "; }; zle -N _capture; bindkey '^Xv' _capture\n"
					}
					os.WriteFile(rc, []byte(script), 0600)
					var cmd *exec.Cmd
					if name == "bash" {
						cmd = exec.Command(bin, "--noprofile", "--rcfile", rc, "-i")
					} else {
						os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(script), 0600)
						cmd = exec.Command(bin, "-d", "-i")
						cmd.Env = append(os.Environ(), "ZDOTDIR="+dir, "TMPDIR="+dir)
					}
					pt, e := pty.Start(cmd)
					if e != nil {
						t.Fatal(e)
					}
					defer func() { pt.Close(); cmd.Process.Kill(); cmd.Wait() }()
					go io.Copy(io.Discard, pt)
					time.Sleep(150 * time.Millisecond)
					if existing {
						pt.WriteString("existing input")
					}
					pt.WriteString("\x18g")
					time.Sleep(150 * time.Millisecond)
					pt.WriteString("\x18v")
					var b []byte
					deadline := time.Now().Add(3 * time.Second)
					for time.Now().Before(deadline) {
						b, e = os.ReadFile(capture)
						if e == nil {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					if e != nil {
						t.Fatal(e)
					}
					want := payload
					if existing {
						want = "existing input"
						if _, e = os.Stat(called); !os.IsNotExist(e) {
							t.Fatal("composer called over existing input")
						}
					}
					if string(b) != want {
						t.Fatalf("buffer=%q want=%q", b, want)
					}
					if _, e = os.Stat(marker); !os.IsNotExist(e) {
						t.Fatal("STAGING EXECUTED SOURCE")
					}
				})
			}
		})
	}
}
func TestHookDoesNotSubmit(t *testing.T) {
	for _, n := range []string{"bash", "zsh"} {
		s, _ := Integration(n, "/muse path/muse")
		for _, bad := range []string{"accept-line", "eval ", "xdotool"} {
			if strings.Contains(s, bad) {
				t.Fatal(s)
			}
		}
	}
}
