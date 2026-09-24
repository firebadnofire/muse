package composer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const Limit = 1 << 20

func System(mode, shell string) string {
	s := "You compose shell text for a human to review. You have no tools and no authority to execute commands or modify the environment. Never claim to have run anything. Target Linux and the user's shell: " + shell + ". Return only final shell source, no Markdown fences, reasoning, explanations, or terminal controls. Do not request or assume access to files, history, environment, or terminal output. "
	if mode == "compose" {
		return s + "Write a readable shell script; multiline source is allowed."
	}
	return s + "Write one shell command on a single line. Do not include literal newlines."
}

// Display replaces all terminal controls, including bidi formatting, visibly.
func Display(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
		} else if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func Validate(s string) error {
	if len(s) > Limit {
		return fmt.Errorf("draft exceeds 1 MiB")
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("draft is not valid UTF-8")
	}
	for _, r := range s {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.In(r, unicode.Cf) {
			return fmt.Errorf("draft contains control or invisible formatting characters; edit them out")
		}
	}
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("empty response")
	}
	return nil
}
func Normalize(s, mode string) (string, error) {
	if e := Validate(s); e != nil {
		return "", e
	}
	// Legacy inline reasoning is never accepted: its boundaries are ambiguous.
	if strings.Contains(s, "<think") || strings.Contains(s, "</think>") {
		return "", fmt.Errorf("inline thinking detected; regenerate with another model")
	}
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
		if len(lines) < 3 || lines[len(lines)-1] != "```" {
			return "", fmt.Errorf("incomplete Markdown fence")
		}
		switch lines[0] {
		case "```", "```sh", "```bash", "```zsh", "```shell":
		default:
			return "", fmt.Errorf("unexpected code fence")
		}
		s = strings.Join(lines[1:len(lines)-1], "\n") + "\n"
	}
	if strings.Contains(s, "```") {
		return "", fmt.Errorf("mixed Markdown and command text; edit or regenerate")
	}
	// Preserve shell-significant whitespace, including escaped trailing spaces.
	// One final response line terminator is permitted in Shotgun, except a
	// continuation, which must remain a multiline draft.
	if mode == "shotgun" && strings.HasSuffix(s, "\n") && !strings.HasSuffix(s, "\\\n") {
		s = strings.TrimSuffix(s, "\n")
	}
	if mode == "shotgun" && strings.Contains(s, "\n") {
		return "", fmt.Errorf("Shotgun returned multiple lines; switch to Compose or regenerate")
	}
	return s, Validate(s)
}
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

type Draft struct{ Path string }

func NewDraft(text string) (*Draft, error) {
	if e := Validate(text); e != nil {
		return nil, e
	}
	d, e := os.MkdirTemp("", "muse-draft-*")
	if e != nil {
		return nil, e
	}
	p := filepath.Join(d, "draft.sh")
	if e = os.WriteFile(p, []byte(text), 0600); e != nil {
		os.RemoveAll(d)
		return nil, e
	}
	return &Draft{p}, nil
}
func (d *Draft) Read() (string, error) {
	st, e := os.Lstat(d.Path)
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() || st.Size() > Limit {
		return "", fmt.Errorf("editor draft must be a regular file at most 1 MiB")
	}
	f, e := os.Open(d.Path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	st, e = f.Stat()
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() || st.Size() > Limit {
		return "", fmt.Errorf("editor draft must be a regular file at most 1 MiB")
	}
	b := make([]byte, st.Size())
	_, e = io.ReadFull(f, b)
	if len(b) == 0 {
		return "", fmt.Errorf("empty draft")
	}
	if e != nil {
		return "", e
	}
	return string(b), Validate(string(b))
}
func (d *Draft) Close() { os.RemoveAll(filepath.Dir(d.Path)) }

// Preserve creates a private, non-executable file. It never runs shell source.
func Preserve(text string) (string, error) {
	if e := Validate(text); e != nil {
		return "", e
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".local", "share")
	}
	dir := filepath.Join(base, "muse", "scripts")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(dir, "reviewed-*.sh")
	if e != nil {
		return "", e
	}
	if _, e = f.WriteString(text); e != nil {
		f.Close()
		os.Remove(f.Name())
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	return f.Name(), nil
}
