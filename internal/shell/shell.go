package shell

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"archuser.org/muse/internal/composer"
	"github.com/creack/pty"
	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

func Detect() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/bash"
}
func Supported(s string) bool { b := filepath.Base(s); return b == "bash" || b == "zsh" }

// Integration is printed only on explicit request. No startup files are modified.
// Assignments do not re-evaluate expansions in the captured text. No accept-line
// widget, eval of output, PTY injection or Enter is used.
func Integration(name, exe string) (string, error) {
	call := composer.Quote(exe) + " composer --transfer --shell " + name
	switch name {
	case "bash":
		return `_muse_compose() {
    if [[ -n $READLINE_LINE ]]; then
        printf '\nMuse: existing input preserved; clear the line first.\n' >&2
        return
    fi
    local suggestion
    suggestion=$(` + call + ` </dev/tty) || return
    [[ -z $READLINE_LINE ]] || return
    READLINE_LINE=$suggestion
    READLINE_POINT=${#READLINE_LINE}
}
bind -x '"\C-xg":_muse_compose'
`, nil
	case "zsh":
		return `# Private, per-shell handoff for direct "muse composer" invocations.
if [[ ${MUSE_STAGE_PID-} != $$ || ! -d ${MUSE_STAGE_DIR-} ]]; then
    export MUSE_STAGE_DIR=$(umask 077; command mktemp -d "${TMPDIR:-/tmp}/muse-stage.XXXXXXXXXX") || return
    export MUSE_STAGE_PID=$$
fi
_muse_stage_prompt() {
    emulate -L zsh
    [[ ${MUSE_STAGE_PID-} == $$ && -f $MUSE_STAGE_DIR/pending ]] || return 0
    local suggestion
    suggestion=$(<"$MUSE_STAGE_DIR/pending")
    command rm -- "$MUSE_STAGE_DIR/pending" || return
    # Raw text goes to the native editing stack, never to shell evaluation.
    [[ -n $suggestion ]] && builtin print -rz -- "$suggestion"
    return 0
}
_muse_stage_cleanup() {
    [[ ${MUSE_STAGE_PID-} == $$ && -d ${MUSE_STAGE_DIR-} ]] || return 0
    command rm -f -- "$MUSE_STAGE_DIR/pending"
    command rmdir -- "$MUSE_STAGE_DIR" 2>/dev/null
    return 0
}
autoload -Uz add-zsh-hook
add-zsh-hook precmd _muse_stage_prompt
add-zsh-hook zshexit _muse_stage_cleanup
_muse_compose() {
    if [[ -n $BUFFER ]]; then
        zle -M 'Muse: existing input preserved; clear the line first.'
        return
    fi
    local suggestion
    suggestion=$(` + call + ` </dev/tty) || { zle redisplay; return; }
    [[ -z $BUFFER ]] || return
    BUFFER=$suggestion
    CURSOR=${#BUFFER}
    zle redisplay
}
zle -N _muse_compose
bindkey '^Xg' _muse_compose
`, nil
	}
	return "", fmt.Errorf("shell %q has no verified staging adapter; use muse composer and manual transfer", name)
}
func Run(path string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("muse requires a terminal; use muse generate for text output")
	}
	cmd := exec.Command(path, "-i")
	size, e := pty.GetsizeFull(os.Stdin)
	if e != nil {
		return e
	}
	pt, e := pty.StartWithSize(cmd, size)
	if e != nil {
		return e
	}
	defer pt.Close()
	state, e := term.MakeRaw(int(os.Stdin.Fd()))
	if e != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return e
	}
	defer term.Restore(int(os.Stdin.Fd()), state)
	// Restore display modes on normal exit and handled termination signals.
	defer fmt.Fprint(os.Stdout, "\x1b[?1049l\x1b[?25h\x1b[?2004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[0m")
	reader, e := cancelreader.NewReader(os.Stdin)
	if e != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return e
	}
	defer reader.Close()
	defer reader.Cancel()
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)
	defer signal.Stop(sig)
	copied := make(chan struct{})
	go func() { io.Copy(os.Stdout, pt); close(copied) }()
	go func() { io.Copy(pt, reader) }()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case e := <-done:
			select {
			case <-copied:
			case <-time.After(200 * time.Millisecond):
			}

			return e
		case s := <-sig:
			if s == syscall.SIGWINCH {
				_ = pty.InheritSize(os.Stdin, pt)
				continue
			}
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
			_ = pt.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				<-done
			}
			return fmt.Errorf("shell session ended by %s", s)
		}
	}
}
