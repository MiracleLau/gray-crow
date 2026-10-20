package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// searchTimeout 同时覆盖请求与响应体读取（http.Client.Timeout 语义）。
const searchTimeout = 20 * time.Second

type APIError struct {
	Status     int
	Kind       string
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string { return e.Message }

const (
	ErrKindUnauthorized = "unauthorized"
	ErrKindRateLimit    = "ratelimit"
	ErrKindServer       = "server"
	ErrKindNetwork      = "network"
	ErrKindQuery        = "query"
	ErrKindOther        = "other"
)

type CodeItem struct {
	Repo     string
	RepoURL  string
	Path     string
	HTMLURL  string
	Fragment string
}

type SearchResult struct {
	Total      int // 上游 total_count
	ItemCount  int // 本页条目数（一个条目可能带多个 text_match 片段）
	Items      []CodeItem
	Quota      Quota
	Incomplete bool
}

// SearchCode 单页代码搜索。5xx 与网络错误重试 3 次（1s/2s/4s + 抖动），429/403 不重试，返回冷却信号。
func SearchCode(ctx context.Context, token, query string, page int) (*SearchResult, *APIError) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("per_page", strconv.Itoa(PageSize))
	params.Set("page", strconv.Itoa(page))

	var lastErr *APIError
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1))*time.Second + time.Duration(rand.Int63n(int64(500*time.Millisecond)))
			select {
			case <-ctx.Done():
				return nil, &APIError{Kind: ErrKindNetwork, Message: ctx.Err().Error()}
			case <-time.After(backoff):
			}
		}
		resp, err := searchRequest(ctx, token, params)
		if err != nil {
			lastErr = &APIError{Kind: ErrKindNetwork, Message: err.Error()}
			continue
		}
		result, apiErr := decodeSearch(resp)
		if apiErr == nil {
			return result, nil
		}
		lastErr = apiErr
		if apiErr.Kind != ErrKindServer && apiErr.Kind != ErrKindNetwork {
			return nil, apiErr
		}
	}
	return nil, lastErr
}

func searchRequest(ctx context.Context, token string, params url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/search/code?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.text-match+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: searchTimeout}
	return client.Do(req)
}

func decodeSearch(resp *http.Response) (*SearchResult, *APIError) {
	defer resp.Body.Close()
	quota := quotaFromHeader(resp.Header)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		apiErr := &APIError{
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("上游 %d: %s", resp.StatusCode, truncate(strings.TrimSpace(string(body)), 200)),
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			apiErr.Kind = ErrKindUnauthorized
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
			apiErr.Kind = ErrKindRateLimit
		case resp.StatusCode >= 500:
			apiErr.Kind = ErrKindServer
		case resp.StatusCode == http.StatusUnprocessableEntity:
			// 查询本身被上游拒绝（如括号），不是密钥的问题
			apiErr.Kind = ErrKindQuery
		default:
			apiErr.Kind = ErrKindOther
		}
		// code search 额度耗尽时 GitHub 也可能返回 403 + remaining=0
		if apiErr.Kind != ErrKindUnauthorized && quota.Limit > 0 && quota.Remaining == 0 {
			apiErr.Kind = ErrKindRateLimit
		}
		if apiErr.Kind == ErrKindRateLimit {
			apiErr.RetryAfter = retryAfter(resp.Header, quota)
		}
		return nil, apiErr
	}

	var payload struct {
		TotalCount        int  `json:"total_count"`
		IncompleteResults bool `json:"incomplete_results"`
		Items             []struct {
			Path       string `json:"path"`
			HTMLURL    string `json:"html_url"`
			Repository struct {
				FullName string `json:"full_name"`
				HTMLURL  string `json:"html_url"`
			} `json:"repository"`
			TextMatches []struct {
				Fragment string `json:"fragment"`
			} `json:"text_matches"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, &APIError{Kind: ErrKindServer, Message: "解析上游响应失败: " + err.Error()}
	}

	result := &SearchResult{Total: payload.TotalCount, ItemCount: len(payload.Items), Quota: quota, Incomplete: payload.IncompleteResults, Items: []CodeItem{}}
	for _, item := range payload.Items {
		for _, match := range item.TextMatches {
			if match.Fragment == "" {
				continue
			}
			result.Items = append(result.Items, CodeItem{
				Repo:     item.Repository.FullName,
				RepoURL:  item.Repository.HTMLURL,
				Path:     item.Path,
				HTMLURL:  item.HTMLURL,
				Fragment: match.Fragment,
			})
		}
	}
	return result, nil
}

func quotaFromHeader(h http.Header) Quota {
	limit, _ := strconv.Atoi(h.Get("x-ratelimit-limit"))
	remaining, _ := strconv.Atoi(h.Get("x-ratelimit-remaining"))
	reset, _ := strconv.ParseInt(h.Get("x-ratelimit-reset"), 10, 64)
	q := Quota{Limit: limit, Remaining: remaining}
	if reset > 0 {
		q.ResetAt = time.Unix(reset, 0).UTC()
	}
	return q
}

// retryAfter 优先用 Retry-After，其次用限流窗口重置时刻。
func retryAfter(h http.Header, q Quota) time.Duration {
	if seconds, err := strconv.Atoi(h.Get("retry-after")); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if !q.ResetAt.IsZero() {
		if d := time.Until(q.ResetAt); d > 0 {
			return d
		}
	}
	return time.Minute
}
