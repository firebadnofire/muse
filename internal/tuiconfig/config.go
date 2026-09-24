package tuiconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Model string `toml:"model"`
	Mode  string `toml:"mode"`
}

func Default() Config {
	return Config{Model: "qwen2.5-coder:7b", Mode: "shotgun"}
}

func Path() string {
	p := os.Getenv("XDG_CONFIG_HOME")
	if p == "" {
		h, _ := os.UserHomeDir()
		p = filepath.Join(h, ".config")
	}
	return filepath.Join(p, "muse", "muse.conf")
}

func Load(path string) (Config, error) {
	c := Default()
	b, e := os.ReadFile(path)
	if e == nil {
		e = toml.Unmarshal(b, &c)
	}
	if e != nil && !os.IsNotExist(e) {
		return c, e
	}
	return c, nil
}

func Save(path string, config Config) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	b, e := toml.Marshal(config)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".muse.conf-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}

func SetModel(path, model string) error {
	c, err := Load(path)
	if err != nil {
		return err
	}
	c.Model = model
	return Save(path, c)
}

func SetMode(path, mode string) error {
	c, err := Load(path)
	if err != nil {
		return err
	}
	c.Mode = mode
	return Save(path, c)
}

func (c Config) Validate() error {
	if c.Model == "" {
		return fmt.Errorf("model must not be empty")
	}
	if c.Mode != "shotgun" && c.Mode != "compose" {
		return fmt.Errorf("mode must be shotgun or compose")
	}
	return nil
}