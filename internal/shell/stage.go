package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"muse/internal/composer"
)

// StageDirectory identifies an explicitly enabled integration in the immediate
// parent shell. It must not be reused from a nested shell or background process.
func StageDirectory() (string, error) {
	dir := os.Getenv("MUSE_STAGE_DIR")
	if dir == "" || os.Getenv("MUSE_STAGE_PID") != strconv.Itoa(os.Getppid()) {
		exe, e := os.Executable()
		if e != nil {
			exe = "muse"
		}
		return "", fmt.Errorf("prompt staging requires shell integration: in Zsh run eval \"$(%s integration zsh)\", then rerun your command; in Bash enable integration bash and use Ctrl+X g. For text-only output use composer --manual or generate --print", composer.Quote(exe))
	}
	st, e := os.Lstat(dir)
	if e != nil {
		return "", fmt.Errorf("staging session unavailable: %w", e)
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !st.IsDir() || st.Mode().Perm() != 0700 || !ok || int(owner.Uid) != os.Getuid() {
		return "", fmt.Errorf("staging directory must be owned by you with mode 0700")
	}
	tty, e := os.Open("/dev/tty")
	if e != nil {
		return "", e
	}
	defer tty.Close()
	pg, e := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP)
	if e != nil || pg != unix.Getpgrp() {
		return "", fmt.Errorf("prompt staging requires a foreground composer in the integrated shell")
	}
	if _, e = os.Lstat(filepath.Join(dir, "pending")); !os.IsNotExist(e) {
		return "", fmt.Errorf("a staged suggestion is already pending; return to your prompt first")
	}
	return dir, nil
}

// Queue publishes one complete accepted line atomically, without replacing a
// pending suggestion. The parent Zsh precmd hook consumes it via print -rz.
func Queue(dir, text string) error {
	if e := composer.Validate(text); e != nil {
		return e
	}
	if strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("refusing multiline prompt staging")
	}
	f, e := os.CreateTemp(dir, ".stage-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.WriteString(text); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Link(f.Name(), filepath.Join(dir, "pending"))
}
