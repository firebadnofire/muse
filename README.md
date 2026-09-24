# Muse

[![CI](https://pubcode.archuser.org/firebadnofire/muse/badges/workflows/test.yml/badge.svg?branch=main)](https://pubcode.archuser.org/firebadnofire/muse/actions)

Your shell. Your commands. A little inspiration.

Muse is a Linux terminal companion written in Go. It runs your real shell in a
PTY and offers an on-demand AI command composer backed by Ollama. **The model
composes; the human executes.** Accepting a suggestion never runs it.

This first release uses a full-size shell and a separate, temporary composer
screen. Your terminal emulator renders the shell directly. There is no embedded
terminal emulator or persistent 20/80 split pane. This deliberate reduction keeps
normal terminal applications usable; see [architecture](docs/architecture.md).

## Build and install

Requirements: Linux, Go 1.26 or newer, a terminal such as Kitty, Bash or Zsh for
optional native staging, Ollama for generation, and `vi` or another editor for
Compose. Shell use does not require Ollama. No root privileges are required.

```sh
go build -o bin/muse ./cmd/muse
./bin/muse --help
./bin/muse doctor
./bin/muse
```

Optional installation (does not install hooks or edit your dotfiles):

```sh
mkdir -p ~/.local/bin
install -m 755 bin/muse ~/.local/bin/muse
export PATH="$HOME/.local/bin:$PATH"
```

On Fedora Atomic, use an existing Go toolchain, a development toolbox, or an
upstream Go archive under your home directory. Run the resulting binary on the
host so `localhost:11434`, your shell configuration, and terminal are accessible.

For this development machine a checksum-verified Go toolchain was installed at
`~/.local/share/muse-toolchain/go`. To use it:

```sh
export PATH="$HOME/.local/share/muse-toolchain/go/bin:$PATH"
make build
./bin/muse
```

## Ollama and initial configuration

If your Ollama server is already running, leave it running. Otherwise start your
installed Ollama with `ollama serve` (or its existing service). Muse does not
install Ollama or download, replace, or delete models.

```sh
./bin/muse models
./bin/muse doctor
./bin/muse --model qwen3:8b
```

Muse queries `GET /api/tags`; every installed model is eligible and exact tags
are preserved. The initial preferred model is `qwen2.5-coder:7b`. If it is absent,
Muse asks you to select an installed alternative instead of silently switching.
Use **Ctrl+L**, arrows, Enter in the composer to refresh/select models, including
while the application remains open. Selection writes the preferred model to
configuration; other file settings are preserved (TOML comments are not).

No configuration file is required. See [config.example.toml](config.example.toml).
Default path: `$XDG_CONFIG_HOME/muse/config.toml`, falling back to
`~/.config/muse/config.toml`.

Precedence, lowest to highest: defaults → TOML file → environment → CLI flags.
An interactive model selection overrides the active model for that composer and
updates the file. Explicit environment/CLI model overrides still apply on the
next invocation; unset `MUSE_MODEL` to use the stored preference.

| Setting | Environment | CLI |
| --- | --- | --- |
| endpoint | `MUSE_ENDPOINT` | `--endpoint` |
| model | `MUSE_MODEL` | `--model` |
| mode | `MUSE_MODE` | `--mode` |
| editor | `MUSE_EDITOR` (then `EDITOR`) | `--editor` |
| timeout | `MUSE_TIMEOUT` | `--timeout` |
| temperature | `MUSE_TEMPERATURE` | `--temperature` |
| max_tokens | `MUSE_MAX_TOKENS` | `--max-tokens` |

`MUSE_CONFIG` or `--config` selects the file; `--config` requires an existing
file. `--shell` overrides `$SHELL` (fallback `/bin/bash`). Shell startup passes
launch settings to composer subprocesses. The normal nested shell reads its
usual interactive startup files; it is not a login shell.

Only the request you type, mode/shell instructions, and generation parameters
are sent to Ollama. A remote `--endpoint` receives this data over the configured
connection; prefer HTTPS outside your machine. No shell history, working
directory, files, environment values, or terminal output are sent.

## Shell and optional integration

`muse` starts your shell. Run commands, use aliases/functions, Vim, htop, SSH, and
normal history as usual. `exit` or Ctrl+D exits. Ctrl+C reaches the foreground
program. Muse forwards terminal bytes and resize events; it does not interpret
shell output. Its own executable directory is added to the child shell's PATH.

Acceptance stages editable text; it never executes. Enable the matching shell
integration once per session:

```sh
# Bash:
eval "$(muse integration bash)"
# Zsh (choose this instead when using Zsh):
eval "$(muse integration zsh)"
```

In Zsh you can then use the normal command, including from this checkout:

```sh
./bin/muse composer
```

After Ctrl+A, the accepted command appears at your next prompt, ready to edit.
Press Enter separately to execute. The Zsh hook consumes a private handoff file
and uses its native editing-buffer stack (`print -rz`); no keystrokes are injected.
The handoff is restricted to a foreground composer launched by that same shell.
It also supports Compose's reviewed-script invocation.

Bash uses **Ctrl+X, then g** for this workflow; direct-command staging is currently
Zsh-only. Without integration, `muse composer` explains setup before generation.
For an intentional copy/paste fallback, use `muse composer --manual`.

These commands evaluate Muse's static integration code, never model output.
Nothing is installed automatically. The printed hook binds **Ctrl+X, then g**;
it may replace an existing binding for that key. You may choose to put the
appropriate line in your own `.bashrc` or `.zshrc` after inspecting
`muse integration bash`/`zsh`. Muse itself never changes those files.

The widget opens the composer only when the input buffer is empty. With existing
input, it refuses and preserves it. To stage a suggestion, accept it in the
composer; the widget assigns Bash's `READLINE_LINE` or Zsh's `BUFFER`. You can
continue editing. **You must press Enter yourself to execute.** There is no
PTY paste or simulated Enter. Keys inside Vim/SSH remain that application's keys;
Muse does not intercept a global composer shortcut.

To remove integration, reopen the shell. For Bash you can also use
`bind -r '\C-xg'`. For Zsh, remove the binding and prompt hook with
`bindkey -r '^Xg'; add-zsh-hook -d precmd _muse_stage_prompt`, then run
`_muse_stage_cleanup; unset MUSE_STAGE_DIR MUSE_STAGE_PID`. The exit hook cleans
up the private session directory on normal exit. Other shells can use
`muse composer --manual`; automatic staging is unsupported.

## Shotgun

1. Open `muse composer`, or Ctrl+X g after enabling integration.
2. Enter a natural-language request and press Enter.
3. Watch the streamed preview. Esc cancels; partial/failed output cannot be accepted.
4. Inspect the result. Ctrl+E opens your editor, Ctrl+R regenerates, Ctrl+D discards.
5. Ctrl+A stages editable text in your prompt (Zsh command or Bash/Zsh widget).
   Only an explicit `--manual` invocation displays copy/paste text. Nothing executes.

Shotgun normally produces one line. Multiline model responses are rejected with
a suggestion to use Compose. If you deliberately edit a Shotgun draft into
multiple lines, acceptance uses the same private-file transfer as Compose.

## Compose

Ctrl+T switches modes, or start `muse composer --mode compose`. After generation,
Muse creates a 0700 temporary directory and a 0600 draft file, then opens your
configured editor automatically. Editor commands support quoted arguments,
without shell expansion or command substitution. Fallback is `$EDITOR`, then `vi`.

After saving and closing the editor, Muse returns to a scrollable review. Ctrl+E
reopens the draft, Ctrl+D discards it, and Ctrl+A explicitly accepts. Acceptance
saves a **non-executable 0600 script** under
`$XDG_DATA_HOME/muse/scripts/` (default `~/.local/share/muse/scripts/`) and returns
a quoted shell invocation referring to that file. With integration, only that
one-line invocation is staged. With `--manual`, it is shown for you to
manually enter. Neither the script nor the invocation runs automatically.

Draft files are removed on normal exit/discard. Accepted scripts remain so the
staged invocation stays valid; remove them yourself when finished. A crash or
SIGKILL may leave a private `muse-draft-*` directory in the temporary directory.

## Composer shortcuts

| Key | Action |
| --- | --- |
| Enter | Generate from the request |
| Ctrl+L | Refresh installed models; arrows/Enter select, Esc returns |
| Ctrl+T | Toggle Shotgun/Compose, discard current preview |
| Esc | Cancel generation; leave selector; otherwise close composer |
| Ctrl+C | Close composer and cancel generation |
| Ctrl+A | Explicitly accept reviewed result, never execute |
| Ctrl+E | Edit/revise draft in editor |
| Ctrl+R | Regenerate |
| Ctrl+D | Discard suggestion |
| PgUp / PgDn | Scroll suggestion |

The request field has keyboard focus. During generation, only cancel/close and
preview scrolling act; model/mode changes wait for completion or cancellation.
Shell focus returns when the composer closes. Windows below 32×10 show a resize
notice and cannot accept suggestions.

## Generate directly into your prompt

With Zsh integration enabled, `generate` places the completed single-line
suggestion into your next editable prompt:

```sh
./bin/muse generate 'Write a Zsh loop for echoing "test1" through "test5"'
```

Review or edit it there, then press Enter yourself to execute. Invoking `generate`
requests staging; it does not execute the result or require opening the composer.
Without integration, Muse explains setup before making an inference request.
Errors, canceled requests, and invalid/multiline Shotgun output never stage text.
Use `composer --mode compose` for the script editor/review workflow.

For **text-only stdout output**, including pipes and noninteractive use, explicitly
choose `--print`. This works without integration and never stages anything:

```sh
muse generate --print --model qwen2.5-coder:7b \
  'Use GNU find from / with -xdev, print byte sizes and paths, sort descending, show ten files'
muse generate --print --mode compose 'Write a script to summarize disk usage'
```

Responses are buffered until complete and validated. Errors go to stderr with a
nonzero exit status; Ctrl+C cancels. **Do not pipe output to a shell or use
command substitution to execute it.** The interactive composer streams previews;
`generate` intentionally withholds partial output.

## Troubleshooting and limits

- **Server unavailable:** run `muse doctor`, check `ollama serve` and endpoint.
  The ordinary shell still works. Ctrl+L retries discovery in the composer.
- **Model unavailable:** run `muse models`, then select an exact name/tag with
  Ctrl+L or `--model`. No automatic fallback or download occurs.
- **Timeout/interrupted stream:** Esc cancels; Ctrl+R retries. Raise `--timeout`
  for a large model or `--max-tokens` for longer scripts. Failed/partial responses
  cannot be accepted. Separate thinking fields are not used as command text;
  legacy inline `<think>` output is rejected.
- **Editor failed:** fix `$EDITOR`/`--editor`, then Ctrl+E to retry. Use a
  terminal editor or a GUI editor's wait option (such as `code --wait`).
- **Shortcut does nothing:** enable the matching shell integration, clear the
  line, check conflicting shell bindings. `doctor` cannot inspect a parent
  shell's active bindings. Zsh vi-mode custom keymaps may require rebinding.
- **Long or invalid output:** control/invisible formatting characters are
  displayed visibly and rejected for transfer. Complete outer shell Markdown
  fences are removed. Muse cannot prove arbitrary model prose is valid shell
  source or that a proposed command is correct; review and edit every result.
- **Visual interference:** the composer is a modal foreground program. Shell
  background jobs that write to the terminal can disrupt it, as with any TUI.
  Close the composer and stop/redirect those jobs.
- **Terminal behavior:** this release has no split pane, detached sessions,
  background AI generation while using the shell, or mouse composer controls.
  Kitty/Hyprland visual behavior and an actual remote SSH session still need
  human verification. Ctrl+Z suspending the composer is not a supported workflow;
  close it to return to the shell.
- **History:** the nested shell owns history. Concurrent shell history merging
  follows your shell configuration. Accepted text enters history only if you
  later submit it.

See [security model](docs/security.md), [architecture](docs/architecture.md), and
[verification procedures and results](docs/testing.md).

## Development

```sh
make check
make race                    # requires a C compiler on Linux
make smoke                   # optional: Python 3 + pexpect; real PTYs, fake HTTP
```

Unit tests use an HTTP test server and fake inference backend; Ollama is not
required. Native staging tests exercise actual Bash/Zsh PTYs and skip a shell
only when it is not installed. [Forgejo CI](.forgejo/workflows/test.yml) uses
Pubcode's mirrored checkout/setup-go actions and the `ubuntu-22.04` Linux runner
label. It installs both shells and runs checks and PTY smokes in an explicit
Node 20/Debian job container. No GitHub-hosted runner is required.
