# Verification

## Automated checks

```sh
go fmt ./...
go vet ./...
go test ./...
go build ./...
go test -race ./...
go build -o bin/muse ./cmd/muse
python3 scripts/pty-smoke.py ./bin/muse
MUSE_TEST_SHELL=zsh python3 scripts/pty-smoke.py ./bin/muse
```

The Go suite covers XDG/config/env/CLI precedence, exact model names and model
persistence, streaming content versus thinking, malformed HTTP/NDJSON, missing
completion, model errors, cancellation, token truncation, and offline servers.
Composer tests cover normalization, metacharacters, whitespace, control/bidi
sanitization, private-file permissions/cleanup, editor failure, small windows,
explicit acceptance, stale canceled events, and Compose file transfer. Native
Bash/Zsh PTY tests assert that command substitutions and semicolons stay literal
in editable buffers, create no execution marker, and preserve existing input.
Unsupported-shell behavior has a deterministic test.

The optional Python check requires `pexpect`. It launches the built application
in real Linux PTYs and runs a local deterministic HTTP server. It exercises shell
resize, Ctrl+C/Ctrl+D, Vim and htop when installed, the actual composer and native
widget (and direct `muse composer` command staging in Zsh), existing input,
direct `generate` staging, `--print` without staging, rejection of invalid
multiline generations, Compose editor save/review/file transfer, model selection
and persistence, and clean shell exit. Generated fixture source is never submitted
to the shell. Temporary HOME/XDG directories isolate this test from your dotfiles. The Zsh
fixture disables global rc scripts to avoid distribution-specific terminal
probes in a PTY without a visual emulator.

## Live Ollama smoke (opt-in)

These commands make network/inference requests but never execute generated text:

```sh
curl --fail http://localhost:11434/api/tags
./bin/muse doctor
./bin/muse models
./bin/muse generate --print --model qwen2.5-coder:7b \
  'Use GNU find starting at / with -xdev and -type f, print numeric byte size and path using -printf, sort numerically descending and show ten results. This is a read-only request; return one command.'
```

On 2026-09-24, the local server was reachable, returned nine installed models,
and included `qwen2.5-coder:7b`. A real request returned:

```sh
find / -xdev -type f -printf "%s %p\n" | sort -nr | head -n 10
```

This text was displayed and **not executed**. This is evidence of working live
inference, not certification that any generated command correctly handles every
filename or meets your intended semantics. Human review remains necessary.

## Human terminal checklist

Use an ordinary Kitty window in your Hyprland session. This checklist still needs
human visual verification; automated PTY traffic does not prove rendering quality.

1. Run `./bin/muse`. Confirm your usual prompt, aliases, functions, environment,
   completion, and history. Run an ordinary command with Ollama stopped or an
   invalid endpoint and verify the shell works independently.
2. Resize the window and run `stty size`. Open Vim, htop, and an SSH session to a
   host you control. Check cursor movement, colors, alternate-screen restoration,
   mouse behavior where applicable, and Ctrl+C. Close each normally.
3. At an empty Bash or Zsh prompt, explicitly enable the matching hook from the
   README. Press Ctrl+X g. Check request focus, connection/model/mode indicators,
   model selector, and readable streaming preview. Select a different installed
   model, close and reopen (without an environment/CLI model override), and
   confirm persistence. No model should be downloaded.
4. In Zsh, also invoke `./bin/muse composer` as a normal command after enabling
   integration. Generate a harmless one-liner such as printing a word. Accept with Ctrl+A.
   Confirm it appears editable and has **not run**. Move the cursor and change
   the word. Only your separate Enter should execute it.
5. Type existing shell input and press Ctrl+X g. Confirm it stays unchanged and
   the composer does not open. Clear the input and try again.
6. In the editor, use a harmless fixture such as
   `printf '%s\n' "$(printf substitution)"; printf second`. Accept and verify no
   output until you separately press Enter. Do not test with destructive commands.
7. Switch to Compose. Generate a two-line script, edit it in `$EDITOR`, save,
   review all lines (PgUp/PgDn), reopen with Ctrl+E, then accept. Confirm the shell
   shows a quoted file invocation, not raw multiline text, and nothing ran.
   Inspect file permissions; discard another draft and check cleanup.
8. Cancel generation with Esc, immediately regenerate, and confirm no stale
   result can be accepted. Interrupt server connectivity during a request; expect
   an error and no acceptable partial suggestion. Use Ctrl+L to reconnect.
9. Shrink below 32×10 while composing. Confirm the resize notice and inability
   to accept while the preview is hidden. Enlarge and review again.
10. Close composer with Esc/Ctrl+C. Exit shell with Ctrl+D or `exit`. Confirm echo,
    cursor visibility, keyboard input, and terminal modes are restored. Repeat
    with an external SIGTERM to the Muse process. SIGKILL cannot run cleanup.

## Recorded results

The Go format/vet/test/build checks and race tests passed on this Linux machine
with Go 1.27.1. Both Bash and Zsh PTY smokes passed, including real Vim editing, htop,
offline-server recovery, and terminal echo/canonical mode restoration. Native
Bash and Zsh staging tests passed, including multiline fixture assignments. The live Ollama procedure above passed.
The workflow lives in `.forgejo/workflows/test.yml` and targets Pubcode's
`ubuntu-22.04` label on the Linux runner shown in the runner configuration.
Its action references are explicitly
`https://pubcode.archuser.org/actions/checkout@v4` and
`https://pubcode.archuser.org/actions/setup-go@v5`; both manifests were verified
on Pubcode. The job uses `node:20-bookworm` so Node 20 and Debian package
installation do not depend on the openSUSE runner host's installed tools. The
runner must support job containers and have registry/package/toolchain network
access. Go setup may download Go from upstream; mirrored actions do not imply
an offline build. Go caching is disabled so no cache service is required.

Pushes, pull requests, and manual dispatch trigger the workflow. Only Linux runs
are defined because Muse currently targets Linux; the macOS, FreeBSD, and Windows
runners are not used. YAML and shell syntax were checked locally, but this
workflow has not yet been executed on Pubcode's runner.
No human visual Kitty/Hyprland test or live remote SSH session was performed.
