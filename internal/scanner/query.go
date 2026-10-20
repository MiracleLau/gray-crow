package scanner

import (
	"fmt"
	"strings"
	"time"
)

const (
	// DefaultTemplate 用 "||" 分隔多条查询：上游 /search/code 的解析器会拒绝括号（422），
	// 且把 OR/AND 当普通词忽略（裸词隐式 AND），所以「域名 + 任一凭证词」只能拆成多条查询。
	DefaultTemplate = `"{kw}" password -is:fork || "{kw}" secret -is:fork || "{kw}" token -is:fork || "{kw}" apikey -is:fork`
	MaxKeywords     = 10
	MaxPages        = 10
	PageSize        = 100
	MaxResults      = MaxPages * PageSize
)

// Query 一条待执行的查询。Keyword 用于告警归属（同一关键词的多条分支共用）。
type Query struct {
	Keyword string
	Text    string
}

// BuildQueries 关键词去重去空后逐个套用模板的每条分支，最多 MaxKeywords 个关键词。
func BuildQueries(keywords []string, template string) []Query {
	if strings.TrimSpace(template) == "" {
		template = DefaultTemplate
	}
	branches := []string{}
	for _, part := range strings.Split(template, "||") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		if !strings.Contains(part, "{kw}") {
			part = part + " {kw}"
		}
		branches = append(branches, part)
	}
	out := []Query{}
	seen := map[string]bool{}
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" || seen[kw] {
			continue
		}
		seen[kw] = true
		for _, branch := range branches {
			out = append(out, Query{Keyword: kw, Text: strings.ReplaceAll(branch, "{kw}", kw)})
		}
		if len(seen) >= MaxKeywords {
			break
		}
	}
	return out
}

// Window 追加提交日期窗口，用于结果超过 1000 条上限时的切分。
func Window(query string, from, to time.Time) string {
	return fmt.Sprintf("%s committed:>=%s committed:<%s", query, from.UTC().Format("2006-01-02"), to.UTC().Format("2006-01-02"))
}

// WindowSize 单个日期窗口长度：先 30 天，窗口过宽时由调度侧继续收窄。
const WindowSize = 30 * 24 * time.Hour
