package app

import (
	"archuser.org/muse/internal/config"
	"archuser.org/muse/internal/inference"
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fake struct {
	chunks []inference.Delta
	err    error
}

func (f fake) Models(context.Context) ([]string, error) {
	return []string{"qwen2.5-coder:7b", "other:tag"}, nil
}
func (f fake) Generate(ctx context.Context, q inference.Request, emit func(inference.Delta) error) error {
	for _, d := range f.chunks {
		if e := emit(d); e != nil {
			return e
		}
	}
	return f.err
}
func fixture(t *testing.T, f fake) *Model {
	t.Helper()
	m := New(context.Background(), config.Default(), filepath.Join(t.TempDir(), "config.toml"), "bash", f, nil)
	m.models = []string{m.cfg.Model}
	m.input.SetValue("test request")
	t.Cleanup(m.clearDraft)
	return m
}
func TestGenerateDoesNotAccept(t *testing.T) {
	m := fixture(t, fake{chunks: []inference.Delta{{Thinking: true}, {Text: "echo $(touch NEVER); echo ok"}}})
	cmd := m.start()
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if !m.ready || m.result.Text != "" || m.suggestion != "echo $(touch NEVER); echo ok" {
		t.Fatal(m)
	}
	if m.accept() == nil || m.result.Text != m.suggestion {
		t.Fatal("explicit acceptance missing")
	}
}
func TestFailureCannotAccept(t *testing.T) {
	m := fixture(t, fake{chunks: []inference.Delta{{Text: "partial"}}, err: errors.New("interrupted")})
	cmd := m.start()
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if m.ready || m.accept() != nil || m.result.Text != "" {
		t.Fatal("accepted failed generation")
	}
}
func TestCancelIgnoresStaleCompletion(t *testing.T) {
	m := fixture(t, fake{})
	m.busy = true
	m.generation = 2
	m.cancel = func() {}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.Update(streamMsg{id: 2, done: true})
	if m.ready || m.result.Text != "" {
		t.Fatal("stale generation accepted")
	}
}
func TestComposeTransferCreatesFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := fixture(t, fake{})
	m.cfg.Mode = "compose"
	m.suggestion = "echo first\necho second"
	m.ready = true
	if m.accept() == nil {
		t.Fatal("accept")
	}
	b, e := os.ReadFile(m.result.File)
	if e != nil || string(b) != m.suggestion || strings.Contains(m.result.Text, "\n") {
		t.Fatal(m.result, e)
	}
}
func TestEditorFailureAndSmallWindow(t *testing.T) {
	m := fixture(t, fake{})
	m.Update(editedMsg{err: errors.New("editor crashed")})
	if m.ready {
		t.Fatal("editor failure accepted")
	}
	m.suggestion = "echo hi"
	m.ready = true
	m.width = 10
	if m.accept() != nil {
		t.Fatal("hidden acceptance")
	}
}
