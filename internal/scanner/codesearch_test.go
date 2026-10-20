package scanner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func withServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	original := apiBase
	apiBase = server.URL
	t.Cleanup(func() {
		apiBase = original
		server.Close()
	})
	return server
}

func searchBody(items int) string {
	rows := []string{}
	for i := 0; i < items; i++ {
		rows = append(rows, fmt.Sprintf(`{"path":"app/conf%d.yaml","html_url":"https://github.com/acme/backend/blob/main/app/conf%d.yaml",
			"repository":{"full_name":"acme/backend","html_url":"https://github.com/acme/backend"},
			"text_matches":[{"fragment":"password: \"S3cr3t-Pa55w0rd\""},{"fragment":""}]}`, i, i))
	}
	return fmt.Sprintf(`{"total_count":150,"incomplete_results":false,"items":[%s]}`, strings.Join(rows, ","))
}

func TestSearchCodeParsesFragmentsAndQuota(t *testing.T) {
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page 应为 100，实际 %s", got)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "text-match") {
			t.Errorf("未请求 text-match: %s", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("鉴权头错误: %s", got)
		}
		w.Header().Set("x-ratelimit-limit", "10")
		w.Header().Set("x-ratelimit-remaining", "7")
		w.Header().Set("x-ratelimit-reset", fmt.Sprint(time.Now().Add(time.Minute).Unix()))
		fmt.Fprint(w, searchBody(2))
	})

	result, apiErr := SearchCode(context.Background(), "tok", `"acme.com"`, 1)
	if apiErr != nil {
		t.Fatalf("搜索失败: %v", apiErr)
	}
	if result.Total != 150 {
		t.Fatalf("总数解析错误: %d", result.Total)
	}
	if len(result.Items) != 2 {
		t.Fatalf("空 fragment 应被丢弃，期望 2 条，实际 %d", len(result.Items))
	}
	if result.Items[0].Repo != "acme/backend" || result.Items[0].Path != "app/conf0.yaml" {
		t.Fatalf("条目解析错误: %+v", result.Items[0])
	}
	if result.Quota.Limit != 10 || result.Quota.Remaining != 7 || result.Quota.ResetAt.IsZero() {
		t.Fatalf("余量头解析错误: %+v", result.Quota)
	}
}

func TestSearchCodeErrorKinds(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		headers    map[string]string
		wantKind   string
		wantStatus int
		wantCool   time.Duration
	}{
		{"401 失效", http.StatusUnauthorized, nil, ErrKindUnauthorized, 401, 0},
		{"403 限流", http.StatusForbidden, map[string]string{"retry-after": "42"}, ErrKindRateLimit, 403, 42 * time.Second},
		{"429 限流", http.StatusTooManyRequests, map[string]string{"x-ratelimit-reset": fmt.Sprint(time.Now().Add(90 * time.Second).Unix())}, ErrKindRateLimit, 429, 0},
		{"404 其他", http.StatusNotFound, nil, ErrKindOther, 404, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			withServer(t, func(w http.ResponseWriter, r *http.Request) {
				attempts++
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"message":"nope"}`)
			})
			_, apiErr := SearchCode(context.Background(), "tok", "q", 1)
			if apiErr == nil {
				t.Fatal("应返回错误")
			}
			if apiErr.Kind != tc.wantKind || apiErr.Status != tc.wantStatus {
				t.Fatalf("错误类型错误: kind=%s status=%d", apiErr.Kind, apiErr.Status)
			}
			// 4xx 重试只会白烧额度，必须只请求一次
			if attempts != 1 {
				t.Fatalf("4xx 不应重试，实际请求 %d 次", attempts)
			}
			if tc.wantCool > 0 && apiErr.RetryAfter != tc.wantCool {
				t.Fatalf("冷却时长应取 retry-after: %v", apiErr.RetryAfter)
			}
			if tc.status == http.StatusTooManyRequests && apiErr.RetryAfter < 80*time.Second {
				t.Fatalf("无 retry-after 时应回落到额度窗口重置: %v", apiErr.RetryAfter)
			}
		})
	}
}

func TestSearchCodeRetriesServerErrors(t *testing.T) {
	attempts := 0
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, searchBody(1))
	})
	result, apiErr := SearchCode(context.Background(), "tok", "q", 1)
	if apiErr != nil {
		t.Fatalf("5xx 后应重试成功: %v", apiErr)
	}
	if attempts != 3 || len(result.Items) != 1 {
		t.Fatalf("重试次数或结果异常: attempts=%d items=%d", attempts, len(result.Items))
	}
}
