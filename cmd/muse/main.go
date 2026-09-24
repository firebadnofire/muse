package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"muse/internal/app"
	"muse/internal/composer"
	"muse/internal/config"
	"muse/internal/inference"
	"muse/internal/inference/ollama"
	"muse/internal/shell"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var version = "0.1.0"

const help = `Muse — the model composes; the human executes.

Usage: muse [options] [shell|composer|models|doctor|generate|integration bash|zsh]

  muse                  Start your normal interactive shell in a PTY
  muse composer         Open AI composer; accept into integrated shell prompt
  muse models           Discover installed models
  muse doctor           Diagnose configuration, shell and Ollama
  muse generate REQUEST Generate into the editable Zsh prompt; never execute
  muse integration bash Print opt-in native line-editor widget

Options (accepted before or after the subcommand):
  --model NAME          Exact installed model name, including tag
  --endpoint URL        Ollama URL (default http://localhost:11434)
  --mode shotgun|compose
  --editor COMMAND      Editor and arguments (default $EDITOR, then vi)
  --timeout DURATION    Generation timeout (default 2m)
  --temperature NUMBER  Sampling temperature (default 0.1)
  --max-tokens NUMBER   Output token limit (default 4096)
  --config PATH         Config file (default XDG config/muse/config.toml)
  --shell PATH          Shell override
  --manual              Explicit copy/paste composer fallback
  --print               Generate text to stdout instead of staging
  --help, --version

Composer: Enter generates; Ctrl+L models; Ctrl+T mode; Esc cancels;
Ctrl+E editor; Ctrl+A accepts; Ctrl+R regenerates; Ctrl+D discards;
PgUp/PgDn scroll; Ctrl+C returns to shell. Acceptance never executes.

Optional integration (run yourself inside Bash/Zsh):
  eval "$(muse integration bash)"   # or zsh
Then Ctrl+X followed by g opens the composer at an EMPTY shell prompt.
In Zsh, direct muse composer also stages into the next prompt.
No dotfiles are modified. Without integration, use muse composer --manual.
`

// Reorder recognized flags so common 'muse generate --model NAME request' works.
func arguments(args []string) ([]string, error) {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
			if !strings.Contains(a, "=") && name != "help" && name != "h" && name != "version" && name != "transfer" && name != "manual" && name != "print" {
				if i+1 == len(args) {
					return nil, fmt.Errorf("missing value for %s", a)
				}
				i++
				flags = append(flags, args[i])
			}
		} else {
			rest = append(rest, a)
		}
	}
	return append(flags, rest...), nil
}
func run() error {
	args, e := arguments(os.Args[1:])
	if e != nil {
		return e
	}
	fs := flag.NewFlagSet("muse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var model, endpoint, mode, editor, timeout, shellPath, cfgPath string
	var temp float64
	var tokens int
	var showHelp, showVersion, transfer, manual, printOnly bool
	fs.StringVar(&model, "model", "", "")
	fs.StringVar(&endpoint, "endpoint", "", "")
	fs.StringVar(&mode, "mode", "", "")
	fs.StringVar(&editor, "editor", "", "")
	fs.StringVar(&timeout, "timeout", "", "")
	fs.StringVar(&shellPath, "shell", shell.Detect(), "")
	fs.StringVar(&cfgPath, "config", config.Path(), "")
	fs.Float64Var(&temp, "temperature", 0, "")
	fs.IntVar(&tokens, "max-tokens", 0, "")
	fs.BoolVar(&showHelp, "help", false, "")
	fs.BoolVar(&showHelp, "h", false, "")
	fs.BoolVar(&showVersion, "version", false, "")
	fs.BoolVar(&transfer, "transfer", false, "")
	fs.BoolVar(&manual, "manual", false, "")
	fs.BoolVar(&printOnly, "print", false, "")
	fs.Usage = func() { fmt.Fprint(os.Stderr, help) }
	if e = fs.Parse(args); e != nil {
		return e
	}
	if showHelp {
		fmt.Print(help)
		return nil
	}
	if showVersion {
		fmt.Println("muse " + version)
		return nil
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	command := "shell"
	if fs.NArg() > 0 {
		command = fs.Arg(0)
	}
	if command == "integration" {
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		s, e := shell.Integration(fs.Arg(1), exe)
		if e != nil {
			return e
		}
		fmt.Print(s)
		return nil
	}
	c, e := config.Load(cfgPath, visited["config"])
	if e != nil {
		if command != "shell" {
			return fmt.Errorf("configuration: %w", e)
		}
		fmt.Fprintln(os.Stderr, "Muse: configuration error; shell remains available:", composer.Display(e.Error()))
		c = config.Default()
	}
	for k, p := range map[string]*string{"model": &c.Model, "endpoint": &c.Endpoint, "mode": &c.Mode, "editor": &c.Editor, "timeout": &c.Timeout} {
		if visited[k] {
			switch k {
			case "model":
				*p = model
			case "endpoint":
				*p = endpoint
			case "mode":
				*p = mode
			case "editor":
				*p = editor
			case "timeout":
				*p = timeout
			}
		}
	}
	if visited["temperature"] {
		c.Temperature = temp
	}
	if visited["max-tokens"] {
		c.MaxTokens = tokens
	}
	// Ordinary shell startup does not depend on backend configuration or availability.
	if command == "shell" {
		_ = os.Setenv("MUSE_CONFIG", cfgPath)
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		_ = os.Setenv("PATH", filepath.Dir(exe)+":"+os.Getenv("PATH"))
		fmt.Fprintln(os.Stderr, "Muse shell — enable prompt staging: eval \"$(muse integration "+filepath.Base(shellPath)+")\"\nCtrl+X g opens composer after opt-in. Exit/Ctrl+D leaves the shell.")
		// Carry launch overrides into composer subprocesses without changing the config file.
		for k, v := range map[string]string{"MUSE_ENDPOINT": c.Endpoint, "MUSE_MODEL": c.Model, "MUSE_MODE": c.Mode, "MUSE_EDITOR": c.Editor, "MUSE_TIMEOUT": c.Timeout, "MUSE_TEMPERATURE": fmt.Sprint(c.Temperature), "MUSE_MAX_TOKENS": fmt.Sprint(c.MaxTokens)} {
			if k == "MUSE_MODEL" && !visited["model"] {
				continue
			}
			os.Setenv(k, v)
		}
		return shell.Run(shellPath)
	}

	if e = c.Validate(); e != nil {
		return e
	}
	b := ollama.New(c.Endpoint)
	switch command {
	case "composer":
		if transfer && manual {
			return errors.New("--manual and --transfer cannot be combined")
		}
		var stageDir string
		if !transfer && !manual {
			stageDir, e = shell.StageDirectory()
			if e != nil {
				return e
			}
		}
		result, e := app.Run(c, cfgPath, shellPath, b)
		if e != nil {
			return e
		}
		if result.Text == "" {
			return nil
		}
		if transfer {
			if strings.ContainsAny(result.Text, "\r\n") {
				return errors.New("refusing multiline line-editor transfer")
			}
			fmt.Print(result.Text)
		} else if !manual {
			if e = shell.Queue(stageDir, result.Text); e != nil {
				return fmt.Errorf("could not stage suggestion: %w", e)
			}
		} else {
			fmt.Println("Accepted for manual transfer — nothing executed. Review, then type/copy into your shell:\n" + composer.Display(result.Text))
		}
		if result.File != "" {
			fmt.Fprintln(os.Stderr, "Reviewed script retained at "+composer.Display(result.File)+" (0600; remove when finished).")
		}
		return nil
	case "models", "doctor":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		names, e := b.Models(ctx)
		if command == "models" {
			if e != nil {
				return e
			}
			for _, n := range names {
				fmt.Println(composer.Display(n))
			}
			return nil
		}
		fmt.Println("Config:", composer.Display(cfgPath))
		fmt.Println("Endpoint:", composer.Display(c.Endpoint))
		fmt.Println("Shell:", composer.Display(shellPath), "native adapter:", shell.Supported(shellPath))
		_, se := exec.LookPath(shellPath)
		if se != nil {
			fmt.Println("Shell error:", se)
		}
		fmt.Println("Integration: opt-in only; doctor cannot inspect parent shell bindings. Use bind -X (Bash) or bindkey '^Xg' (Zsh).")
		if e != nil {
			return e
		}
		fmt.Printf("Ollama: connected; %d installed models\n", len(names))
		if !slices.Contains(names, c.Model) {
			return fmt.Errorf("model unavailable: %s; use muse composer → Ctrl+L, or --model", c.Model)
		}
		fmt.Println("Model:", composer.Display(c.Model), "available")
		return se
	case "generate":
		prompt := strings.Join(fs.Args()[1:], " ")
		if prompt == "" {
			return fmt.Errorf("usage: muse generate [options] REQUEST")
		}
		var stageDir string
		if !printOnly {
			if c.Mode != "shotgun" {
				return errors.New("generate prompt staging requires --mode shotgun; use composer --mode compose for script review, or generate --print --mode compose for text output")
			}
			stageDir, e = shell.StageDirectory()
			if e != nil {
				return e
			}
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		d, _ := time.ParseDuration(c.Timeout)
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		names, e := b.Models(ctx)
		if e != nil {
			return e
		}
		if !slices.Contains(names, c.Model) {
			return fmt.Errorf("model unavailable: %s; run muse models and select --model", c.Model)
		}
		var out strings.Builder
		e = b.Generate(ctx, inference.Request{Model: c.Model, System: composer.System(c.Mode, filepath.Base(shellPath)), Prompt: prompt, Temperature: c.Temperature, MaxTokens: c.MaxTokens}, func(d inference.Delta) error { out.WriteString(d.Text); return nil })
		if e != nil {
			return e
		}
		s, e := composer.Normalize(out.String(), c.Mode)
		if e != nil {
			return e
		}
		if printOnly {
			fmt.Println(s)
			return nil
		}
		if e = shell.Queue(stageDir, s); e != nil {
			return fmt.Errorf("could not stage suggestion: %w", e)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q; use muse --help", command)
	}
}
func main() {
	if e := run(); e != nil {
		var exitErr *exec.ExitError
		if errors.As(e, &exitErr) {
			code := exitErr.ExitCode()
			if code < 0 {
				code = 1
			}
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, "muse:", composer.Display(e.Error()))
		os.Exit(1)
	}
}
