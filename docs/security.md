# Security model

Muse separates four operations:

1. **Generate:** an inference backend returns untrusted text. It has no PTY,
   filesystem-context, process-execution, or shell-tool interface.
2. **Review/accept:** the user inspects a preview or editor result and presses
   Ctrl+A in the composer. Editor exit, errors, and cancellation do not approve
   text. The `generate REQUEST` shortcut explicitly requests a suggestion in the
   next editable prompt; review and editing happen there before the user's Enter.
   Only complete, validated one-line responses can be staged by this shortcut.
   `generate --print` only emits text and never stages it.
3. **Stage:** an explicitly enabled Zsh prompt hook consumes a private, atomic
   handoff file and uses `print -rz` to push accepted text onto the editing-buffer
   stack for the next prompt. Handoffs require the immediate parent shell's
   session ID, a 0700 directory owned by the user, and foreground terminal control.
   They contain a validated single line in a 0600 file, cannot replace another
   pending result, and are consumed once. Alternatively, a native widget assigns accepted single-line
   text to an empty Bash `READLINE_LINE` or Zsh `BUFFER`. Shell expansions in a
   variable's contents are not evaluated by assignment. No Enter, carriage
   return, newline, accept-line widget, or model text is written to the PTY.
4. **Execute:** the user deliberately presses Enter in their shell. Only that
   shell interprets the staged command. Muse provides no execution action.

Multiline text goes to a private, non-executable file after review. Only a
shell-quoted invocation of that file is offered for staging/manual transfer.
Temporary directories are 0700 and files 0600; discarded drafts are cleaned up.
Accepted scripts remain until the user deletes them. Treat them as sensitive.

Model content is never run for validation. Metacharacters such as `;`, `$()`,
backticks, quotes, and redirects remain literal text until user execution.
Native-widget tests assert that marker files are not created by such payloads.
Existing input blocks the widget, and unsupported shells receive a manual
fallback via `--manual`. Optional hooks are printed on request, never installed or sourced by
Muse. The user explicitly sources only the static hook, not its generated text.

Preview rendering replaces terminal controls and Unicode formatting characters
with visible escapes. Transfer rejects those characters, invalid UTF-8,
empty responses, and oversized drafts. Inference output is capped at 1 MiB.
Separate thinking content is ignored; ambiguous inline thinking is rejected.
Only complete outer shell fences are normalized. Shell-significant spaces are
preserved. There is no way to reliably infer intent or correctness from arbitrary
prose: the human review boundary remains necessary.

The shell PTY intentionally passes user-program output, including terminal
controls, to the terminal emulator. This is ordinary terminal behavior and is a
separate path from model output. Muse does not sanitize shell output or promise
to defend against a hostile program you choose to run.

The editor is a trusted user-selected program and may have plugins, modelines,
or its own AI features. Muse parses its arguments without executing an
intermediate shell and does not evaluate `$()` or backticks in editor settings.
Muse's guarantees do not sandbox editors, protect against another process running
as the same user, validate command safety, or stop users explicitly executing
text (including piping `muse generate --print` into a shell).

Only a user-entered request, target shell name, mode instructions, and generation
settings reach the configured endpoint. No history/files/secrets/terminal output
are collected. No telemetry or application prompt logging is implemented.
Ollama or a remote endpoint may maintain its own logs. Remote HTTP is unencrypted;
choose an appropriate trusted HTTPS endpoint when leaving the local machine.
