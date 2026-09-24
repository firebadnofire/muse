package composer

import (
	"os"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	for _, s := range []string{"echo hi; echo $(date)", "find . -printf '%s %p\\n' | sort -nr"} {
		got, e := Normalize(s, "shotgun")
		if e != nil || got != s {
			t.Fatal(got, e)
		}
	}
	s, e := Normalize("```bash\necho hi\n```", "shotgun")
	if e != nil || s != "echo hi" {
		t.Fatal(s, e)
	}
	if _, e = Normalize("echo a\necho b", "shotgun"); e == nil {
		t.Fatal("multiline shotgun")
	}
	if _, e = Normalize("echo a\necho b", "compose"); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"<think>reason</think>echo hi", "```bash\necho hi", "echo\x1b[2Jhi", "echo\rhi", "echo\u202ehi", "echo\x00hi"} {
		if _, e = Normalize(s, "compose"); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
func TestSanitize(t *testing.T) {
	s := Display("a\x1b]52;c;payload\a\r\u202eb\n\t")
	if strings.ContainsAny(s, "\x1b\a\r\u202e") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "\\u001b") {
		t.Fatal(s)
	}
}
func TestDraftLifecycle(t *testing.T) {
	d, e := NewDraft("echo original")
	if e != nil {
		t.Fatal(e)
	}
	p := d.Path
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	os.WriteFile(p, []byte("echo edited\necho second"), 0600)
	s, e := d.Read()
	if e != nil || s != "echo edited\necho second" {
		t.Fatal(s, e)
	}
	d.Close()
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p, e = Preserve(s)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	if string(b) != s {
		t.Fatal(string(b))
	}
	st, _ = os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}
func TestWhitespaceMeaningPreserved(t *testing.T) {
	for _, s := range []string{"echo escaped\\ ", "  echo hi  ", "printf '%s' 'a  '"} {
		got, e := Normalize(s, "shotgun")
		if e != nil || got != s {
			t.Fatalf("%q became %q: %v", s, got, e)
		}
	}
	if _, e := Normalize("echo continuation\\\n", "shotgun"); e == nil {
		t.Fatal("removed continuation")
	}
}
func TestDraftRejectsSymlinkAndControls(t *testing.T) {
	d, e := NewDraft("echo safe")
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	os.Remove(d.Path)
	os.Symlink("/etc/passwd", d.Path)
	if _, e = d.Read(); e == nil {
		t.Fatal("editor replaced draft with symlink")
	}
	if _, e = NewDraft("echo\x1b[0m"); e == nil {
		t.Fatal("unsafe temporary draft")
	}
}

func TestFencedContinuationIsNotFlattened(t *testing.T) {
	if _, e := Normalize("```bash\necho continuation\\\n```", "shotgun"); e == nil {
		t.Fatal("removed shell continuation inside fence")
	}
	s, e := Normalize("```sh\nprintf hello\n```", "compose")
	if e != nil || s != "printf hello\n" {
		t.Fatal(s, e)
	}
}
