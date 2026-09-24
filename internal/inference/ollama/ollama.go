package ollama

import (
	"archuser.org/muse/internal/inference"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	URL  string
	HTTP *http.Client
}

func New(url string) *Client { return &Client{URL: strings.TrimRight(url, "/"), HTTP: &http.Client{}} }
func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		r = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, c.URL+path, r)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, fmt.Errorf("server unavailable or request interrupted: %w", e)
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		kind := "request failed"
		if resp.StatusCode == 404 && path == "/api/chat" {
			kind = "model unavailable"
		}
		return nil, fmt.Errorf("%s (HTTP %d): %s", kind, resp.StatusCode, b)
	}
	return resp, nil
}
func (c *Client) Models(ctx context.Context) ([]string, error) {
	r, e := c.request(ctx, "GET", "/api/tags", nil)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	var v struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if e = json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&v); e != nil {
		return nil, fmt.Errorf("invalid model list: %w", e)
	}
	names := make([]string, 0, len(v.Models))
	for _, m := range v.Models {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	return names, nil
}
func (c *Client) Generate(ctx context.Context, q inference.Request, emit func(inference.Delta) error) error {
	payload := map[string]any{"model": q.Model, "stream": true, "messages": []map[string]string{{"role": "system", "content": q.System}, {"role": "user", "content": q.Prompt}}, "options": map[string]any{"temperature": q.Temperature, "num_predict": q.MaxTokens}}
	r, e := c.request(ctx, "POST", "/api/chat", payload)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	s := bufio.NewScanner(r.Body)
	s.Buffer(make([]byte, 4096), 1<<20)
	total := 0
	for s.Scan() {
		var v struct {
			Message struct {
				Content  string `json:"content"`
				Thinking string `json:"thinking"`
			} `json:"message"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
			Error      string `json:"error"`
		}
		if e = json.Unmarshal(s.Bytes(), &v); e != nil {
			return fmt.Errorf("invalid response stream: %w", e)
		}
		if v.Error != "" {
			return fmt.Errorf("generation failed: %s", v.Error)
		}
		total += len(v.Message.Content) + len(v.Message.Thinking)
		if total > 1<<20 {
			return fmt.Errorf("generation exceeded 1 MiB limit")
		}
		if v.Message.Thinking != "" {
			if e = emit(inference.Delta{Thinking: true}); e != nil {
				return e
			}
		}
		if v.Message.Content != "" {
			if e = emit(inference.Delta{Text: v.Message.Content}); e != nil {
				return e
			}
		}
		if v.Done {
			if v.DoneReason == "length" {
				return fmt.Errorf("generation truncated at token limit; regenerate or increase max_tokens")
			}
			return nil
		}
	}
	if e = s.Err(); e != nil {
		return fmt.Errorf("generation interrupted: %w", e)
	}
	return fmt.Errorf("generation interrupted: missing completion marker")
}
