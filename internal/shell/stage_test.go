package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueueLiteralAtomicAndExclusive(t *testing.T) {
	dir := t.TempDir()
	line := "echo $(touch NEVER); printf '%s\\n' 'quoted' "
	if e := Queue(dir, line); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "pending"))
	if e != nil || string(b) != line {
		t.Fatal(string(b), e)
	}
	st, _ := os.Stat(filepath.Join(dir, "pending"))
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	if e = Queue(dir, "replacement"); e == nil {
		t.Fatal("overwrote pending suggestion")
	}
	b, _ = os.ReadFile(filepath.Join(dir, "pending"))
	if string(b) != line {
		t.Fatal("changed pending")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal("temporary file leaked")
	}
}
func TestQueueRejectsControlsAndMultiline(t *testing.T) {
	for _, s := range []string{"echo a\necho b", "echo\r", "echo\x1b[2J", "echo\u202ehi"} {
		dir := t.TempDir()
		if e := Queue(dir, s); e == nil {
			t.Fatalf("accepted %q", s)
		}
		files, _ := os.ReadDir(dir)
		if len(files) > 0 {
			t.Fatal("published invalid text")
		}
	}
}
func TestMissingAndInheritedSession(t *testing.T) {
	t.Setenv("MUSE_STAGE_DIR", t.TempDir())
	t.Setenv("MUSE_STAGE_PID", "-1")
	if _, e := StageDirectory(); e == nil || !strings.Contains(e.Error(), "integration") {
		t.Fatal(e)
	}
}
