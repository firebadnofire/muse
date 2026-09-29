// SPDX-License-Identifier: MPL-2.0

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestQueryHistoryPersistsPrivatelyAndRetainsRecentEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".config", "muse", "query-history.jsonl")
	for i := 0; i < queryHistoryLimit+3; i++ {
		if _, err := saveQueryHistory(path, fmt.Sprintf("request %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	history, err := loadQueryHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != queryHistoryLimit {
		t.Fatalf("history has %d entries, want %d", len(history), queryHistoryLimit)
	}
	if history[0] != "request 3" || history[len(history)-1] != "request 502" {
		t.Fatalf("unexpected retained range: first=%q last=%q", history[0], history[len(history)-1])
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("history mode = %o, want 600", info.Mode().Perm())
	}
	if queryHistoryPath(filepath.Join(filepath.Dir(path), "config.toml")) != path {
		t.Fatal("history is not beside the config")
	}
}

func TestQueryHistoryConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query-history.jsonl")
	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := saveQueryHistory(path, fmt.Sprintf("request %d", i))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	history, err := loadQueryHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != writers {
		t.Fatalf("history has %d entries, want %d", len(history), writers)
	}
	for i := 0; i < writers; i++ {
		want := fmt.Sprintf("request %d", i)
		found := false
		for _, entry := range history {
			if entry == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %q", want)
		}
	}
}

func TestQueryHistoryCorruptionIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query-history.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadQueryHistory(path); err == nil || !strings.Contains(err.Error(), "invalid query history") {
		t.Fatalf("load error = %v, want invalid history", err)
	}
	if _, err := saveQueryHistory(path, "new request"); err == nil {
		t.Fatal("save unexpectedly replaced corrupt history")
	}
}
