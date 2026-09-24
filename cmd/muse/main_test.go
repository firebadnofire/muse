package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArguments(t *testing.T) {
	got, e := arguments([]string{"generate", "--model", "model:tag", "request", "--temperature=0.2"})
	want := []string{"--model", "model:tag", "--temperature=0.2", "generate", "request"}
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, e)
	}
	got, e = arguments([]string{"generate", "--", "-literal request"})
	if e != nil || !reflect.DeepEqual(got, []string{"generate", "-literal request"}) {
		t.Fatal(got, e)
	}
}

func TestCLIOverridesEnvironmentAndFile(t *testing.T) {
	t.Setenv("MUSE_MODEL", "environment:tag")
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprint(w, `{"models":[{"name":"cli:exact-tag"}]}`)
			return
		}
		var q struct{ Model string }
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
		}
		received = q.Model
		fmt.Fprintln(w, `{"message":{"content":"echo reviewed"},"done":true}`)
	}))
	defer server.Close()
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("model='file:tag'\n"), 0600)
	oldArgs := os.Args
	oldOut := os.Stdout
	defer func() { os.Args = oldArgs; os.Stdout = oldOut }()
	out, e := os.CreateTemp(t.TempDir(), "output")
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	os.Stdout = out
	os.Args = []string{"muse", "generate", "--print", "--config", p, "--model", "cli:exact-tag", "--endpoint", server.URL, "request"}
	if e := run(); e != nil {
		t.Fatal(e)
	}
	if received != "cli:exact-tag" {
		t.Fatal(received)
	}
	b, e := os.ReadFile(out.Name())
	if e != nil || string(b) != "echo reviewed\n" {
		t.Fatal(string(b), e)
	}
}

func TestComposerRequiresIntegrationBeforeGeneration(t *testing.T) {
	t.Setenv("MUSE_STAGE_DIR", "")
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"muse", "composer"}
	if e := run(); e == nil || !strings.Contains(e.Error(), "prompt staging requires shell integration") {
		t.Fatal(e)
	}
}

func TestGenerateRequiresIntegrationBeforeRequest(t *testing.T) {
	t.Setenv("MUSE_STAGE_DIR", "")
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"muse", "generate", "--mode", "shotgun", "test request"}
	if e := run(); e == nil || !strings.Contains(e.Error(), "prompt staging requires shell integration") {
		t.Fatal(e)
	}
}
func TestGenerateComposeNeedsReviewOrPrint(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"muse", "generate", "--mode", "compose", "test request"}
	if e := run(); e == nil || !strings.Contains(e.Error(), "script review") {
		t.Fatal(e)
	}
}
