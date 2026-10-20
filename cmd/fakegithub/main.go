// fakegithub 本地演示/冒烟用的假 GitHub：返回固定的 code search 结果与限流头。
// 用法: go run ./cmd/fakegithub -addr 127.0.0.1:18081
// 配合 GITHUB_API_BASE=http://127.0.0.1:18081 就能在没有真实 Token 的情况下跑通整条链路。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "监听地址")
	items := flag.Int("items", 100, "首页返回的条目数")
	repos := flag.String("repo", "acme/backend", "结果仓库名")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-oauth-scopes", "repo")
		writeJSON(w, map[string]any{"login": "fake-bot", "id": 4242}, nil)
	})
	mux.HandleFunc("/rate_limit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"resources": map[string]any{
			"code_search": map[string]any{"limit": 10, "remaining": 9, "reset": time.Now().Add(time.Minute).Unix()},
		}}, nil)
	})
	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		size := *items
		if page > 1 {
			size = size / 3
		}
		rows := make([]map[string]any, 0, size)
		for i := 0; i < size; i++ {
			rows = append(rows, map[string]any{
				"path":     fmt.Sprintf("config/app%d.yaml", i),
				"html_url": fmt.Sprintf("https://github.com/%s/blob/main/config/app%d.yaml", *repos, i),
				"repository": map[string]any{
					"full_name": *repos,
					"html_url":  "https://github.com/" + *repos,
				},
				"text_matches": []map[string]any{
					{"fragment": "db_url: postgres://svc:hunter2@db.internal:5432/app"},
					{"fragment": `aws_key: "AKIAJ7QK3M9PLZ2XV8NR"`},
					{"fragment": "note: our domain is acme.com"},
				},
			})
		}
		writeJSON(w, map[string]any{
			"total_count":        *items + size,
			"incomplete_results": false,
			"items":              rows,
		}, map[string]string{"x-ratelimit-remaining": "6"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"message": "Not Found"}, nil)
	})

	log.Printf("假 GitHub 监听 %s（结果仓库 %s，首页 %d 条）", *addr, *repos, *items)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func writeJSON(w http.ResponseWriter, body any, headers map[string]string) {
	raw, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-ratelimit-limit", "10")
	if w.Header().Get("x-ratelimit-remaining") == "" {
		w.Header().Set("x-ratelimit-remaining", "8")
	}
	w.Header().Set("x-ratelimit-reset", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10))
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
