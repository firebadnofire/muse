# Muse: Project Vision

> Your shell. Your commands. A little inspiration.

**Status:** Initial product vision  
**Primary language:** Go  
**Initial platform:** Linux  
**Initial inference backend:** Ollama  
**Interface:** Interactive terminal application

## 1. What Muse Is

Muse is a local-first, AI-assisted terminal application. It combines a **real, interactive shell** with an LLM-powered command composer. The user describes an intended task in natural language; Muse proposes shell commands or scripts; the user reviews, edits, and explicitly decides whether to run them.

Muse is **not an autonomous agent**. The model does not own the terminal, execute its suggestions, or silently alter the user's environment. Muse exists to make writing commands easier without removing the user's control over their shell.

This distinction is the project's defining feature, not an optional safety setting:

**The model composes. The human executes.**

## 2. Product Principles

1. **The terminal is real.** Run the user's actual `$SHELL` in a PTY. Preserve normal interactive behavior rather than simulating a shell or reimplementing its command language.
2. **No AI-initiated execution.** Model output is untrusted text. It must never be interpreted as a command to execute, a simulated keystroke, or authorization to press Enter.
3. **Human-controlled by default.** Every proposed command or script must be visible and editable before execution. Accepting a suggestion means placing it in the user's editing context, **not running it**.
4. **Local-first, backend-flexible.** Ollama is the first supported backend, but the application's core must not depend on Ollama-specific concepts. Local models should work without any cloud account.
5. **A shell companion, not a replacement shell.** Preserve existing aliases, functions, startup files, history, environment, shell configuration, and native interactive programs as far as a nested shell permits.
6. **Fast when wanted, deliberate when needed.** Offer a low-friction mode for one-liners and an editor-centered mode for longer or riskier work.
7. **Transparent and unsurprising.** No hidden system prompts that grant operational authority, silent filesystem scraping, background tool use, telemetry, or unexpected network calls.
8. **Useful without the AI.** The shell should remain usable when Ollama is offline, slow, or producing errors.

## 3. Intended Experience

Muse starts as a terminal application. Its principal view is approximately **20% composer and 80% terminal**, adjustable where practical:

```text
┌─────────────────────────────────────────────────────────────────────┐
│ muse  •  Ollama: selected-model                         [Shotgun]  │
│ Describe what you want to do...                                     │
│ > Find the ten largest files on this filesystem                     │
├─────────────────────────────────────────────────────────────────────┤
│ Suggestion                                                          │
│ find . -xdev -type f -printf '%s %p\n' | sort -nr | head -10          │
│ [Review] [Insert into shell] [Regenerate] [Cancel]                   │
├─────────────────────────────────────────────────────────────────────┤
│ $                                                                   │
│                                                                     │
│              REAL SHELL, RUNNING IN A PTY                           │
│                                                                     │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

The drawing illustrates the hierarchy, not a mandatory pixel-perfect layout. The terminal should take priority when space is limited. Prompts, responses, and status must never obscure running terminal programs or unexpectedly steal focus.

### 3.1 Shotgun mode

For quick commands, Muse converts a natural-language request into a proposed shell command. The user can inspect or revise it, regenerate it, or insert it into the **live shell's editable input buffer**. Muse must not submit the line.

Shotgun should feel fast, but speed must never come from bypassing the user's opportunity to inspect the command.

### 3.2 Compose mode

For complex commands or scripts, Muse generates a draft in a temporary file and opens it in the user's `$EDITOR` (with a documented fallback when `$EDITOR` is unset). When the editor closes, Muse presents the revised draft for review. The user may discard it, reopen the editor, or explicitly choose how to transfer the draft into their working shell.

For multiline scripts, never assume a raw PTY paste is safe. The supported transfer mechanism must leave the content editable and unexecuted. Where the active shell cannot safely stage multiline text, offer a reviewed script file or a manual copy/paste workflow rather than pretending automatic insertion is safe.

### 3.3 Ordinary shell use

The terminal operates independently of the composer. The user can run commands, launch SSH, use a full-screen editor, interrupt processes, and resize the window without involving a model. If the model backend fails, only AI-assisted functionality degrades.

## 4. Non-Negotiable Trust Boundary

Muse consists of a **user-controlled terminal** and a **model-controlled text generator**. These are separate trust domains.

- Model responses are displayed as **untrusted suggestions**, never executed by Muse.
- The model cannot directly write to the live PTY, synthesize shell input, call shell tools, or approve its own output.
- Clicking or typing "Insert" must **not** transmit a newline, carriage return, Enter key event, or other implicit execution signal.
- Do not overwrite existing editable shell input without the user's explicit consent.
- Do not run generated content in a subprocess for validation, formatting, preview, or convenience.
- A model response may contain terminal-control sequences, misleading formatting, or hostile instructions. Render it as text and sanitize control sequences before displaying it in application-owned UI.
- No autonomous tool calling, command chaining by an agent, unattended correction loops, or AI-driven background shell activity in the initial product.
- Read-only context access is not automatically harmless. Any optional file, working-directory, history, or environment context supplied to a model must be narrowly scoped, disclosed, and explicitly enabled by the user. Never send secrets or entire environments by default.
- If using a remote Ollama endpoint, make the data boundary clear: submitted prompts and enabled context leave the local machine for that endpoint.

**Important implementation detail:** Writing a command directly to a PTY is not a reliable substitute for editing a shell's current input line. Shells and terminal applications differ in their behavior, and pasted newlines can execute. Implement verified shell-line-editor integration for supported shells, or use a clearly labeled manual transfer fallback. Never claim a safety guarantee that the implementation cannot enforce.

These are architecture and testing requirements. A cosmetic approval dialog does not make unsafe terminal injection acceptable.

## 5. Technical Direction

### 5.1 Language and dependencies

Use **Go** for the initial implementation. Prefer a small, understandable dependency graph, the standard library when suitable, and maintained components for difficult terminal problems.

Candidate dependencies, to be evaluated rather than adopted blindly:

- **Bubble Tea** for the interactive application and event loop.
- **Lip Gloss** for presentation and layout.
- **creack/pty** for creating and managing the child shell's PTY.
- **Cobra**, only if the CLI grows enough to justify a subcommand framework. Simple flags should remain simple.
- Go's `net/http`, `encoding/json`, and `context` for the initial Ollama client.

**Bubble Tea is not a terminal emulator.** Codex must choose and document a real terminal-rendering strategy that handles ANSI/VT sequences, alternate screen, cursor movement, colors, mouse behavior where supported, and resizing. Recreating terminal emulation from scratch is not an MVP task. If a dependable embedded-terminal solution cannot be established, reduce the first milestone to a working external-terminal or shell-integration approach rather than shipping a broken imitation.

Do not commit to specific dependency versions without checking compatibility and upstream maintenance at implementation time.

### 5.2 Core components

Maintain boundaries between these responsibilities:

```text
muse
├── app             TUI state, focus, keybindings, lifecycle
├── terminal        PTY lifecycle, resize, input/output, rendering
├── shell           Shell detection and safe editable-buffer integration
├── composer        Prompt construction, modes, draft lifecycle
├── inference       Backend-neutral request/response/streaming interface
│   └── ollama      Initial backend adapter
├── config          XDG configuration, flags, validation
└── security        Transfer invariants, sanitization, context policy
```

The exact package structure may evolve. Preserve the responsibility boundaries even if the directory names change.

### 5.3 Shell integration

Prioritize **Bash and Zsh** for a dependable initial insertion experience; consider Fish subsequently. Their interactive line editors differ, so integration may require shell-specific adapters or user-installable hooks.

The acceptance contract for an adapter is simple: take reviewed text and place it in the shell's editable command line **without executing it**. The adapter should preserve existing user input, or require explicit confirmation before replacing it. Support a safe fallback when a shell is unsupported or an integration is unavailable.

Do not assume bracketed paste alone solves this requirement. Do not silently install shell hooks or modify dotfiles. If setup is required, make it opt-in and document the changes.

### 5.4 Model interface

The inference layer should support:

- A configurable Ollama base URL and model name.
- Streaming output and user cancellation with `context.Context`.
- Timeouts, connection failures, and actionable error messages.
- A request format that identifies the selected shell and requested mode.
- Explicitly defined response expectations: command text or script text, not tool calls.
- Backend-neutral interfaces so additional inference providers can be added later without rewriting the terminal or composer.

Avoid elaborate agent frameworks. Muse needs reliable text generation, not an agent runtime.

### 5.5 Configuration

Use XDG paths on Linux, such as `$XDG_CONFIG_HOME/muse/config.toml` with an appropriate fallback. Support straightforward command-line overrides for the model, backend URL, and optional configuration file. Do not require an account or daemon.

Choose a documented, predictable precedence order for defaults, config file, environment variables, and CLI flags. Never log prompts, shell history, model responses, or environment variables by default. Do not add telemetry.

## 6. MVP Scope

The first usable release should deliver:

- A Linux application written in Go that launches the user's configured shell in a PTY.
- A reliable, usable terminal pane, or an explicitly documented reduced-scope integration if full embedding is not yet dependable.
- Configurable Ollama URL and model selection.
- Natural-language input with streaming responses, cancellation, and clear error handling.
- Shotgun mode for a single command and Compose mode using `$EDITOR` for longer drafts.
- Review, edit, regenerate, discard, and **safe, non-executing transfer** of proposed text.
- Verified command-buffer insertion for at least one supported shell, with a safe fallback for others.
- Graceful degradation when Ollama is unreachable.
- Basic help, configuration examples, and a short threat-model document explaining the execution boundary.

**MVP means a small, working product, not every envisioned feature.** If the integrated terminal or shell transfer mechanism is not reliable, narrow scope and document the limitation instead of claiming a finished MVP.

## 7. Explicit Non-Goals

Do **not** spend initial implementation effort on:

- Autonomous agents or giving the model shell-execution permissions.
- Running AI-generated commands in a sandbox as a substitute for user review.
- Automatically fetching the entire shell history, filesystem tree, or environment.
- Cloud accounts, synchronization, analytics, or hosted infrastructure.
- A graphical desktop application or browser frontend.
- Plugin ecosystems, workflow automation, multi-agent orchestration, or embedded coding agents.
- Supporting every shell, operating system, or model provider on day one.
- Building a new terminal emulator from first principles.
- A large configuration system before the basic interaction is proven.

## 8. Delivery Plan for Codex

Implement in small, verifiable stages. At the end of each stage, leave the repository building, tests passing, and documentation aligned with reality.

### Phase 1: Establish the foundations

Set up the Go module, directory layout, development commands, CI, and basic configuration. Implement an Ollama adapter with a fake backend for deterministic tests. Demonstrate generation, response streaming, cancellation, and useful errors without touching a real shell.

### Phase 2: Prove the execution boundary

Prototype safe insertion into the **editable shell buffer** for one shell. Write automated tests and a manual verification procedure demonstrating that accepting suggestions does not execute them. Confirm behavior with existing typed input, commands containing semicolons, command substitutions, control characters, and multiline drafts. Include an unsupported-shell fallback.

Treat this proof as a blocker. Do not build a polished UI around an unsafe insertion mechanism.

### Phase 3: Provide a real terminal

Add PTY process management and a reliable rendering solution. Validate interactive Bash/Zsh sessions, shell startup, Ctrl-C, Ctrl-D, resizing, full-screen applications, SSH, and shutdown behavior. Never route model output into the PTY.

### Phase 4: Combine the interfaces

Build the split-screen application with keyboard-driven focus, prompt submission, streamed suggestions, review, and both generation modes. Maintain sensible terminal behavior while generation is in progress. Keep the layout responsive to small windows.

### Phase 5: Harden and document

Test failure conditions, unsupported shells, remote Ollama configuration, cancellation, terminal state restoration, control-sequence sanitization, and file cleanup. Add usage examples, installation instructions, a documented configuration file, architecture notes, and a transparent list of known limitations.

Reorder phases if needed to reduce risk, but **prove safe insertion and real-shell viability early**.

## 9. Definition of Done

Muse is ready for its first public release only when the following are true:

1. A user can launch Muse, interact with their normal shell, and exit without leaving their terminal in a broken state.
2. A user can ask Ollama for a command, review or modify the suggestion, and stage it **without Muse executing it**.
3. Compose mode can open an actual draft in `$EDITOR`, return the edited content, and preserve explicit user control over transfer and execution.
4. The terminal remains usable through model connection failures, slow responses, and generation cancellation.
5. At least one supported shell passes insertion tests covering pre-existing input, shell metacharacters, multiline content, control characters, and accidental Enter/newline submission.
6. Full-screen terminal applications, process interruption, and window resizing work within the chosen terminal strategy, or the reduced-scope release clearly states which capabilities it does not yet offer.
7. No prompts, shell contents, secrets, or environment context are silently transmitted to a model backend beyond the submitted request and explicitly enabled context.
8. Build and tests run in CI; installation and configuration steps have been manually verified on Linux.
9. Documentation never describes aspirational features as implemented functionality.

## 10. Instructions for Codex

Treat this document as the product's design intent. Prioritize correctness, a reliable real-shell experience, and the user/model trust boundary over feature count or visual polish.

When faced with a design tradeoff:

- Prefer an explicit, understandable implementation over hidden magic.
- Prefer fewer dependencies and focused interfaces over premature abstraction.
- Prefer a documented limitation over an unsafe shortcut.
- Prefer an opt-in integration over modifying the user's shell configuration automatically.
- Prefer tests for terminal behavior and the execution boundary over screenshot-perfect TUI tests.
- Preserve the distinction between **accepting text** and **executing text** everywhere in code, UI labels, documentation, and tests.

If a proposed implementation would let model output trigger command execution without a separate, deliberate user action, **do not implement it**. Redesign the interaction instead.

Muse should feel like the user's ordinary terminal gained a thoughtful writing assistant, not like the user handed their terminal to a robot.
