package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Endpoint    string  `toml:"endpoint"`
	Model       string  `toml:"model"`
	Mode        string  `toml:"mode"`
	Editor      string  `toml:"editor"`
	Timeout     string  `toml:"timeout"`
	Temperature float64 `toml:"temperature"`
	MaxTokens   int     `toml:"max_tokens"`
}

func Default() Config {
	return Config{Endpoint: "http://localhost:11434", Model: "qwen2.5-coder:7b", Mode: "shotgun", Timeout: "2m", Temperature: 0.1, MaxTokens: 4096}
}
func Path() string {
	if p := os.Getenv("MUSE_CONFIG"); p != "" {
		return p
	}
	p := os.Getenv("XDG_CONFIG_HOME")
	if p == "" {
		h, _ := os.UserHomeDir()
		p = filepath.Join(h, ".config")
	}
	return filepath.Join(p, "muse", "config.toml")
}
func Load(path string, explicit bool) (Config, error) {
	c := Default()
	b, e := os.ReadFile(path)
	if e == nil {
		e = toml.Unmarshal(b, &c)
	}
	if e != nil && !(os.IsNotExist(e) && !explicit) {
		return c, e
	}
	for k, p := range map[string]*string{"MUSE_ENDPOINT": &c.Endpoint, "MUSE_MODEL": &c.Model, "MUSE_MODE": &c.Mode, "MUSE_EDITOR": &c.Editor, "MUSE_TIMEOUT": &c.Timeout} {
		if v, ok := os.LookupEnv(k); ok {
			*p = v
		}
	}
	if v, ok := os.LookupEnv("MUSE_TEMPERATURE"); ok {
		c.Temperature, e = strconv.ParseFloat(v, 64)
		if e != nil {
			return c, e
		}
	}
	if v, ok := os.LookupEnv("MUSE_MAX_TOKENS"); ok {
		c.MaxTokens, e = strconv.Atoi(v)
		if e != nil {
			return c, e
		}
	}
	return c, nil
}
func (c Config) Validate() error {
	u, e := url.Parse(c.Endpoint)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("endpoint must be an http(s) URL without query or fragment")
	}
	if c.Model == "" {
		return fmt.Errorf("model must not be empty")
	}
	if c.Mode != "shotgun" && c.Mode != "compose" {
		return fmt.Errorf("mode must be shotgun or compose")
	}
	d, e := time.ParseDuration(c.Timeout)
	if e != nil || d <= 0 {
		return fmt.Errorf("timeout must be a positive duration, e.g. 2m")
	}
	if c.Temperature < 0 || c.Temperature > 2 || c.Temperature != c.Temperature {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if c.MaxTokens < 1 || c.MaxTokens > 65536 {
		return fmt.Errorf("max_tokens must be between 1 and 65536")
	}
	return nil
}

// SaveModel preserves file settings and does not persist environment or CLI overrides.
func SaveModel(path, model string) error {
	values := map[string]any{}
	b, e := os.ReadFile(path)
	if e == nil {
		if e = toml.Unmarshal(b, &values); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	values["model"] = model
	b, e = toml.Marshal(values)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".config-*")
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
