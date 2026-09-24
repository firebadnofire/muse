# Architecture decision: native terminal, modal composer

The vision permits a reduced-scope shell integration when dependable embedded
terminal rendering cannot be established. Muse uses that option for its first
release. It does not treat a Bubble Tea viewport as a terminal emulator.

`muse` launches `$SHELL -i` with creack/pty, copies terminal input/output without
parsing it, propagates terminal dimensions/SIGWINCH, and restores termios on
return. The existing emulator (for example Kitty) remains responsible for ANSI,
VT, colors, alternate screen, cursor positioning, and mouse reporting. No PTY
output is routed through Bubble Tea. Interactive shell startup files are read
normally; no generated rc file or automatic hook is injected.

The composer is a separate foreground process launched at a prompt, either by
`muse composer` or an explicitly enabled native line-editor widget. Bubble Tea v2
owns only this temporary alternate screen. Bubbles v2 provides input and a
scrollable review viewport; Lip Gloss v2 provides layout. External editing uses
Bubble Tea's ExecProcess handoff and the controlling terminal, while accepted
text uses a separate stdout pipe captured by the native widget. Direct Zsh
composer invocations instead publish a private single-line file; an explicitly
enabled `precmd` hook consumes it using `print -rz` for the next editing buffer.
A child executable cannot directly change its parent's ZLE state, which is why
this session hook is required. Normal acceptance never silently falls back to
copy/paste; the latter is available only through `--manual`. This distinction
is essential: application UI and editor output cannot become staged commands.

No background shell pane is emulated, and no global key steals input from Vim or
SSH. This sacrifices the 20/80 split and concurrent shell/composer use. Closing
the composer returns control to the existing shell and editable input line.

## Modules

- `cmd/muse`: standard-library CLI, precedence, diagnostics, text generation.
- `internal/config`: XDG TOML defaults, environment, atomic model preference write.
- `internal/inference`: backend-neutral model discovery and streaming interface.
- `internal/inference/ollama`: native tags/chat HTTP adapter and completion checks.
- `internal/composer`: prompts, formatting, control rejection, private draft files.
- `internal/shell`: PTY lifecycle and static Bash/Zsh widgets.
- `internal/app`: cancelable generation, model selection, editor and review states.

The Ollama adapter receives a context and emits text/thinking notifications. It
never receives a terminal handle or process executor. HTTP errors, unavailable
models, invalid streams, truncated responses, and missing completion markers are
errors. Cancellation invalidates the generation ID so delayed events cannot
approve an old result. Timeouts cover the whole request. The composer requires explicit acceptance
before transfer. The `generate` shortcut requests one-line prompt staging directly,
using the same private handoff; `--print` retains text-only output. Multiline
script staging goes through the composer's editor/review workflow.

## Upstream references consulted

- [Bubble Tea v2 API](https://pkg.go.dev/charm.land/bubbletea/v2) and
  [ExecProcess example](https://github.com/charmbracelet/bubbletea/blob/main/examples/exec/main.go).
- [creack/pty](https://github.com/creack/pty) for real PTY creation and resizing.
- [Bash variables](https://www.gnu.org/software/bash/manual/html_node/Bash-Variables.html)
  and [bind -x](https://www.gnu.org/software/bash/manual/html_node/Bash-Builtins.html).
- [Zsh line editor](https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html)
  for BUFFER/CURSOR and user widgets; [Zsh print -z](https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html)
  for the next-prompt editing stack.
- [Ollama chat API](https://docs.ollama.com/api/chat) for streamed content and thinking.

Resolved dependency versions are pinned in go.mod/go.sum. The application uses
current `charm.land/.../v2` imports and no agent framework.
