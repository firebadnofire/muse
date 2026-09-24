package tuiconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrecedenceAndPersistence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "muse", "muse.conf")
	c, e := Load(p)
	if e != nil || c.Model != "qwen2.5-coder:7b" || c.Mode != "shotgun" {
		t.Fatal(c, e)
	}
	
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, []byte("model = 'disk:tag'\nmode = 'compose'\n"), 0600)
	
	c, e = Load(p)
	if e != nil || c.Model != "disk:tag" || c.Mode != "compose" {
		t.Fatal(c, e)
	}
	
	if e = Save(p, Config{Model: "selected:tag", Mode: "shotgun"}); e != nil {
		t.Fatal(e)
	}
	
	c, e = Load(p)
	if e != nil || c.Model != "selected:tag" || c.Mode != "shotgun" {
		t.Fatal(c, e)
	}
	
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}

func TestInvalid(t *testing.T) {
	c := Default()
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	
	c.Model = ""
	if c.Validate() == nil {
		t.Fatal("empty model")
	}
	
	c = Default()
	c.Mode = "invalid"
	if c.Validate() == nil {
		t.Fatal("invalid mode")
	}
}

func TestXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if Path() != filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "muse", "muse.conf") {
		t.Fatal(Path())
	}
}