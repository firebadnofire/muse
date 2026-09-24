package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrecedenceAndPersistence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "muse", "config.toml")
	c, e := Load(p, false)
	if e != nil || c.Model != "qwen2.5-coder:7b" {
		t.Fatal(c, e)
	}
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, []byte("model = 'disk:tag'\ntemperature = 0.3\nendpoint = 'http://localhost:1'\n"), 0600)
	t.Setenv("MUSE_MODEL", "env:exact")
	c, e = Load(p, true)
	if e != nil || c.Model != "env:exact" || c.Temperature != 0.3 {
		t.Fatal(c, e)
	}
	if e = SaveModel(p, "selected:tag"); e != nil {
		t.Fatal(e)
	}
	os.Unsetenv("MUSE_MODEL")
	c, e = Load(p, true)
	if e != nil || c.Endpoint != "http://localhost:1" || c.Model != "selected:tag" {
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
	c.Timeout = "0s"
	if c.Validate() == nil {
		t.Fatal("zero timeout")
	}
	c = Default()
	c.Endpoint = "file:///etc/passwd"
	if c.Validate() == nil {
		t.Fatal("URL")
	}
	if _, e := Load(filepath.Join(t.TempDir(), "absent"), true); e == nil {
		t.Fatal("explicit path")
	}
	t.Setenv("MUSE_TEMPERATURE", "oops")
	if _, e := Load("/nonexistent-muse-config", false); e == nil {
		t.Fatal("bad env")
	}
}
func TestXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if Path() != filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "muse", "config.toml") {
		t.Fatal(Path())
	}
}
