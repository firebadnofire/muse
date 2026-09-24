// Package app owns only the composer. The user's terminal emulator owns shell rendering.
package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-shellwords"
	"muse/internal/composer"
	"muse/internal/config"
	"muse/internal/inference"
)

type modelsMsg struct {
	names []string
	err   error
}
type streamMsg struct {
	id             int
	text           string
	thinking, done bool
	err            error
}
type editedMsg struct {
	text string
	err  error
}
type Result struct{ Text, File string }
type Model struct {
	cfg                                 config.Config
	configPath, shell                   string
	backend                             inference.Backend
	ctx                                 context.Context
	tty                                 *os.File
	input                               textinput.Model
	preview                             viewport.Model
	models                              []string
	selection                           int
	choosing, busy, ready               bool
	status, connection, raw, suggestion string
	cancel                              context.CancelFunc
	events                              chan streamMsg
	generation                          int
	width, height                       int
	draft                               *composer.Draft
	result                              Result
}

func New(ctx context.Context, c config.Config, path, shell string, b inference.Backend, tty *os.File) *Model {
	in := textinput.New()
	in.Placeholder = "Describe a command or script…"
	in.CharLimit = 8192
	in.SetVirtualCursor(true)
	in.Focus()
	return &Model{cfg: c, configPath: path, shell: shell, backend: b, ctx: ctx, tty: tty, input: in, preview: viewport.New(viewport.WithWidth(76), viewport.WithHeight(12)), status: "Enter a request", connection: "checking server", width: 80, height: 24}
}
func (m *Model) discover() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		names, e := m.backend.Models(ctx)
		return modelsMsg{names, e}
	}
}
func (m *Model) Init() tea.Cmd         { return tea.Batch(m.discover(), textinput.Blink) }
func wait(ch <-chan streamMsg) tea.Cmd { return func() tea.Msg { return <-ch } }
func (m *Model) start() tea.Cmd {
	if strings.TrimSpace(m.input.Value()) == "" {
		m.status = "Enter a request first"
		return nil
	}
	if !slices.Contains(m.models, m.cfg.Model) {
		m.status = "Model unavailable; Ctrl+L to discover/select an installed model"
		return nil
	}
	m.clearDraft()
	m.raw = ""
	m.suggestion = ""
	m.ready = false
	m.busy = true
	m.status = "Generating — Esc cancels"
	m.preview.SetContent("")
	m.generation++
	timeout, _ := time.ParseDuration(m.cfg.Timeout)
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	m.cancel = cancel
	id := m.generation
	ch := make(chan streamMsg, 16)
	m.events = ch
	q := inference.Request{Model: m.cfg.Model, System: composer.System(m.cfg.Mode, filepath.Base(m.shell)), Prompt: m.input.Value(), Temperature: m.cfg.Temperature, MaxTokens: m.cfg.MaxTokens}
	backend := m.backend
	return func() tea.Msg {
		go func() {
			e := backend.Generate(ctx, q, func(d inference.Delta) error {
				select {
				case ch <- streamMsg{id: id, text: d.Text, thinking: d.Thinking}:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			select {
			case ch <- streamMsg{id: id, done: true, err: e}:
			case <-ctx.Done():
				// Do not retain a producer blocked behind abandoned preview
				// events when a user cancels and starts another generation.
			drain:
				for {
					select {
					case <-ch:
					default:
						break drain
					}
				}
				ch <- streamMsg{id: id, done: true, err: ctx.Err()}
			case <-m.ctx.Done():
			}
			cancel()
		}()
		return <-ch
	}
}
func (m *Model) clearDraft() {
	if m.draft != nil {
		m.draft.Close()
		m.draft = nil
	}
}
func (m *Model) edit() tea.Cmd {
	if m.suggestion == "" {
		return nil
	}
	m.ready = false
	if m.draft == nil {
		d, e := composer.NewDraft(m.suggestion)
		if e != nil {
			m.status = e.Error()
			return nil
		}
		m.draft = d
	}
	editor := m.cfg.Editor
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// Parse arguments without a shell. Neither substitutions nor environment expansion are enabled.
	args, e := shellwords.Parse(editor)
	if e != nil || len(args) == 0 {
		m.status = "Invalid editor command"
		return nil
	}
	cmd := exec.Command(args[0], append(args[1:], m.draft.Path)...)
	cmd.Stdin = m.tty
	cmd.Stdout = m.tty
	cmd.Stderr = m.tty
	d := m.draft
	m.status = "Editing draft"
	return tea.ExecProcess(cmd, func(e error) tea.Msg {
		if e != nil {
			return editedMsg{err: e}
		}
		s, e := d.Read()
		return editedMsg{s, e}
	})
}
func (m *Model) accept() tea.Cmd {
	if !m.ready || m.busy || m.width < 32 || m.height < 10 {
		return nil
	}
	if e := composer.Validate(m.suggestion); e != nil {
		m.status = e.Error()
		return nil
	}
	if m.cfg.Mode == "compose" || strings.Contains(m.suggestion, "\n") {
		p, e := composer.Preserve(m.suggestion)
		if e != nil {
			m.status = e.Error()
			return nil
		}
		m.result = Result{Text: composer.Quote(m.shell) + " " + composer.Quote(p), File: p}
	} else {
		m.result.Text = m.suggestion
	}
	return tea.Quit
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.input.SetWidth(max(1, v.Width-4))
		m.preview.SetWidth(max(1, v.Width-2))
		m.preview.SetHeight(max(1, v.Height-9))
		return m, nil
	case modelsMsg:
		if v.err != nil {
			m.connection = "server unavailable"
			m.status = v.err.Error()
			m.models = nil
			return m, nil
		}
		m.models = v.names
		m.connection = "connected"
		if !slices.Contains(m.models, m.cfg.Model) {
			m.status = "Configured model unavailable. Ctrl+L, arrows, Enter to select."
			m.choosing = true
			m.selection = 0
		} else {
			m.status = "Connected — Enter generates"
		}
		return m, nil
	case streamMsg:
		if v.id != m.generation {
			return m, nil
		}
		if v.done {
			m.busy = false
			if v.err != nil {
				m.status = "Generation failed: " + v.err.Error()
				m.ready = false
				return m, nil
			}
			s, e := composer.Normalize(m.raw, m.cfg.Mode)
			if e != nil {
				m.status = e.Error()
				return m, nil
			}
			m.suggestion = s
			m.preview.SetContent(composer.Display(s))
			m.preview.GotoTop()
			m.ready = true
			m.status = "Review suggestion — Ctrl+A accepts; never executes"
			if m.cfg.Mode == "compose" {
				return m, m.edit()
			}
			return m, nil
		}
		if v.thinking {
			m.status = "Thinking (reasoning is not a command) — Esc cancels"
		}
		m.raw += v.text
		// Inline thinking, if a legacy model emits it in content, is preview-only and fails normalization.
		m.preview.SetContent(composer.Display(m.raw))
		m.preview.GotoBottom()
		return m, wait(m.events)
	case editedMsg:
		if v.err != nil {
			m.status = "Editor/read failed: " + v.err.Error()
			m.ready = false
			return m, nil
		}
		m.suggestion = v.text
		m.preview.SetContent(composer.Display(v.text))
		m.preview.GotoTop()
		m.ready = true
		m.status = "Edited draft — review, Ctrl+E revise, Ctrl+A accept"
		return m, nil
	case tea.KeyPressMsg:
		key := v.String()
		if key == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if key == "esc" {
			if m.busy {
				m.cancel()
				m.generation++
				m.busy = false
				m.ready = false
				m.status = "Canceled; partial output cannot be accepted"
				return m, nil
			}
			if m.choosing {
				m.choosing = false
				return m, nil
			}
			return m, tea.Quit
		}
		if m.choosing {
			switch key {
			case "up", "k":
				m.selection = max(0, m.selection-1)
			case "down", "j":
				m.selection = min(len(m.models)-1, m.selection+1)
			case "enter":
				if len(m.models) > 0 {
					m.cfg.Model = m.models[m.selection]
					m.choosing = false
					m.ready = false
					m.suggestion = ""
					m.preview.SetContent("")
					m.status = "Selected " + m.cfg.Model
					if e := config.SaveModel(m.configPath, m.cfg.Model); e != nil {
						m.status += " (could not persist: " + e.Error() + ")"
					}
				}
			}
			return m, nil
		}
		if key == "ctrl+l" && !m.busy {
			m.choosing = true
			m.selection = 0
			return m, m.discover()
		}
		if key == "ctrl+t" && !m.busy {
			m.clearDraft()
			m.ready = false
			m.suggestion = ""
			m.preview.SetContent("")
			if m.cfg.Mode == "shotgun" {
				m.cfg.Mode = "compose"
			} else {
				m.cfg.Mode = "shotgun"
			}
			m.status = "Mode: " + m.cfg.Mode
			return m, nil
		}
		if key == "pgup" || key == "pgdown" || key == "ctrl+up" || key == "ctrl+down" {
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		}
		if m.busy {
			return m, nil
		}
		switch key {
		case "enter", "ctrl+r":
			return m, m.start()
		case "ctrl+e":
			return m, m.edit()
		case "ctrl+a":
			return m, m.accept()
		case "ctrl+d":
			m.clearDraft()
			m.suggestion = ""
			m.raw = ""
			m.ready = false
			m.preview.SetContent("")
			m.status = "Discarded"
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
func (m *Model) View() tea.View {
	if m.width < 32 || m.height < 10 {
		v := tea.NewView("Muse: enlarge terminal (32×10 minimum).\nEsc returns to shell.")
		v.AltScreen = true
		return v
	}
	title := fmt.Sprintf("Muse | %s | %s | %s", composer.Display(m.cfg.Model), m.cfg.Mode, m.connection)
	body := m.preview.View()
	if m.choosing {
		var b strings.Builder
		b.WriteString("Installed models (arrows, Enter; Esc back):\n")
		if len(m.models) == 0 {
			b.WriteString("No models available. Esc, Ctrl+L retries.")
		}
		start := max(0, m.selection-max(1, (m.height-12)/2))
		end := min(len(m.models), start+max(1, m.height-11))
		for i := start; i < end; i++ {
			prefix := "  "
			if i == m.selection {
				prefix = "> "
			}
			b.WriteString(prefix + composer.Display(m.models[i]) + "\n")
		}
		body = b.String()
	}
	s := lipgloss.NewStyle().Bold(true).Render(title) + "\n" + m.input.View() + "\n" + composer.Display(m.status) + "\n\n" + body + "\n" +
		"Enter generate | Ctrl+L models | Ctrl+T mode | Esc cancel/back\nCtrl+A accept | Ctrl+E editor | Ctrl+R retry | Ctrl+D discard\nPgUp/PgDn review | Ctrl+C close | Acceptance NEVER executes"
	// Clip the final view to the physical terminal; long errors cannot displace controls.
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(s))
	v.AltScreen = true
	return v
}
func Run(c config.Config, path, shell string, b inference.Backend) (Result, error) {
	tty, e := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if e != nil {
		return Result{}, fmt.Errorf("composer needs an interactive terminal: %w", e)
	}
	defer tty.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := New(ctx, c, path, shell, b, tty)
	defer m.clearDraft()
	_, e = tea.NewProgram(m, tea.WithInput(tty), tea.WithOutput(tty)).Run()
	return m.result, e
}
