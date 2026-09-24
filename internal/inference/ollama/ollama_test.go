package ollama

import (
	"archuser.org/muse/internal/inference"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModelsAndStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprint(w, `{"models":[{"name":"custom:Q4_tag"}]}`)
			return
		}
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body["model"] != "custom:Q4_tag" || body["stream"] != true {
			t.Error(body)
		}
		fmt.Fprintln(w, `{"message":{"thinking":"secret reasoning"}}`)
		w.(http.Flusher).Flush()
		fmt.Fprintln(w, `{"message":{"content":"echo "}}`)
		fmt.Fprintln(w, `{"message":{"content":"ok"},"done":true}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	names, e := c.Models(context.Background())
	if e != nil || len(names) != 1 || names[0] != "custom:Q4_tag" {
		t.Fatal(names, e)
	}
	var text string
	thinking := false
	e = c.Generate(context.Background(), inference.Request{Model: names[0]}, func(d inference.Delta) error { text += d.Text; thinking = thinking || d.Thinking; return nil })
	if e != nil || text != "echo ok" || !thinking {
		t.Fatal(text, thinking, e)
	}
}
func TestFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{{"http", "missing", 404}, {"error", `{"error":"bad model"}`, 200}, {"interrupted", `{"message":{"content":"partial"}}`, 200}, {"malformed", "{", 200}, {"limit", `{"done":true,"done_reason":"length"}`, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprintln(w, tc.body) }))
			defer s.Close()
			if e := New(s.URL).Generate(context.Background(), inference.Request{}, func(inference.Delta) error { return nil }); e == nil {
				t.Fatal("expected failure")
			}
		})
	}
}
func TestCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"content":"start"}}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := New(s.URL).Generate(ctx, inference.Request{}, func(inference.Delta) error { cancel(); return nil })
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestOffline(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := New(s.URL).Models(ctx); e == nil {
		t.Fatal("offline")
	}
}
