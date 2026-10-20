package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	userAgent = "graycrow"
	timeout   = 15 * time.Second
)

var apiBase = "https://api.github.com"

// SetBase 指向自建 GitHub（Enterprise）实例；空值保持官方端点。
func SetBase(base string) {
	if strings.TrimSpace(base) != "" {
		apiBase = strings.TrimRight(base, "/")
	}
}

type Quota struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
}

type ProbeResult struct {
	Login      string
	ID         int64
	Scopes     string
	CodeSearch Quota
}

func newClient() *http.Client {
	return &http.Client{Timeout: timeout}
}

func get(ctx context.Context, token, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", userAgent)
	return newClient().Do(req)
}

// Probe 探活：GET /user + GET /rate_limit（两者都不计额度之外的消耗；rate_limit 本身免费）。
func Probe(ctx context.Context, token string) (*ProbeResult, error) {
	resp, err := get(ctx, token, "/user")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("token 无效(%d): %s", resp.StatusCode, truncate(string(body), 200))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("上游异常(%d): %s", resp.StatusCode, truncate(string(body), 200))
	}

	var user struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	rl, err := get(ctx, token, "/rate_limit")
	if err != nil {
		return nil, err
	}
	defer rl.Body.Close()

	result := &ProbeResult{
		Login:  user.Login,
		ID:     user.ID,
		Scopes: resp.Header.Get("x-oauth-scopes"),
	}
	if rl.StatusCode == http.StatusOK {
		var payload struct {
			Resources struct {
				CodeSearch struct {
					Limit     int   `json:"limit"`
					Remaining int   `json:"remaining"`
					Reset     int64 `json:"reset"`
				} `json:"code_search"`
			} `json:"resources"`
		}
		if err := json.NewDecoder(rl.Body).Decode(&payload); err == nil {
			result.CodeSearch = Quota{
				Limit:     payload.Resources.CodeSearch.Limit,
				Remaining: payload.Resources.CodeSearch.Remaining,
				ResetAt:   time.Unix(payload.Resources.CodeSearch.Reset, 0).UTC(),
			}
		}
	}
	return result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
