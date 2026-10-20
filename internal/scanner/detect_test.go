package scanner

import (
	"strings"
	"testing"
	"time"
)

func TestDetectPatternsAndFallback(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		detector string
		severity string
	}{
		{"aws", `aws_access_key_id = "AKIAJ7QK3M9PLZ2XV8NR"`, "aws-access-key-id", "high"},
		{"github", "const token = 'ghp_" + strings.Repeat("a1B2", 9) + "'", "github-pat", "high"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEow", "private-key", "high"},
		{"db dsn", "DB=postgres://svc:hunter2@db.internal:5432/app", "db-connection-string", "high"},
		{"anthropic", "sk-ant-" + strings.Repeat("x7Q", 10), "anthropic-api-key", "high"},
		{"generic", `password: "S3cr3t-Pa55w0rd"`, "generic-credential", "medium"},
		{"entropy with context", "aws secret Zq7Yb2NvLm4Xk9Rt6Wp1Hs8Ed3Cf5Jg0", "high-entropy", "high"},
		{"entropy noise", "AAAA AAAA AAAA AAAA AAAA", "", "low"},
		{"placeholder", `password: "your_password_here"`, "", "low"},
		{"plain text", "function add(a, b) { return a + b }", "", "low"},
	}
	for _, tc := range cases {
		got := Detect(tc.text)
		if got.DetectorID != tc.detector || got.Severity != tc.severity {
			t.Errorf("%s: 期望 %q/%q，实际 %q/%q", tc.name, tc.detector, tc.severity, got.DetectorID, got.Severity)
		}
		if got.Snippet == "" {
			t.Errorf("%s: 片段不应为空", tc.name)
		}
		if len(got.Snippet) > snippetLimit {
			t.Errorf("%s: 片段超长 %d", tc.name, len(got.Snippet))
		}
	}
	// 官方示例值不算泄露：占位符命中后跳过该规则，降级为低置信度
	if got := Detect(`AKIAIOSFODNN7EXAMPLE2`); got.DetectorID != "" || got.Severity != "low" {
		t.Errorf("占位符应被跳过，实际 %q/%q", got.DetectorID, got.Severity)
	}
}

func TestDetectWindowKeepsContext(t *testing.T) {
	prefix := strings.Repeat("x", 400)
	text := prefix + ` password: "S3cr3t-Pa55w0rd"` + strings.Repeat("y", 400)
	got := Detect(text)
	if !strings.Contains(got.Snippet, "S3cr3t-Pa55w0rd") {
		t.Fatalf("片段应包含命中内容: %q", got.Snippet)
	}
	if len(got.Snippet) > contextMargin*2+40 {
		t.Fatalf("片段应截取上下文而非全量: %d", len(got.Snippet))
	}
}

func TestBuildQueries(t *testing.T) {
	branches := strings.Count(DefaultTemplate, "||") + 1
	if strings.ContainsAny(DefaultTemplate, "()") {
		t.Fatalf("默认模板不能含括号，上游会 422：%s", DefaultTemplate)
	}
	queries := BuildQueries([]string{"acme.com", " acme.com ", "", "@acme.com"}, "")
	if len(queries) != 2*branches {
		t.Fatalf("应去重去空得到 %d 条，实际 %d: %+v", 2*branches, len(queries), queries)
	}
	if !strings.Contains(queries[0].Text, `"acme.com"`) || !strings.Contains(queries[0].Text, "-is:fork") {
		t.Fatalf("默认模板未套用: %s", queries[0].Text)
	}
	if queries[branches].Keyword != "@acme.com" {
		t.Fatalf("关键词归属错误: %+v", queries[branches])
	}

	many := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		many = append(many, "kw"+string(rune('a'+i)))
	}
	if got := len(BuildQueries(many, "")); got != MaxKeywords*branches {
		t.Fatalf("关键词数量应封顶 %d，实际 %d", MaxKeywords, got)
	}

	custom := BuildQueries([]string{"secret"}, "org:acme {kw} language:go")
	if custom[0].Text != "org:acme secret language:go" {
		t.Fatalf("自定义模板未生效: %s", custom[0].Text)
	}
	if got := BuildQueries([]string{"secret"}, "no placeholder")[0].Text; !strings.Contains(got, "secret") {
		t.Fatalf("模板缺少 {kw} 时应追加: %s", got)
	}

	if multi := BuildQueries([]string{"secret"}, `"{kw}" a || "{kw}" b`); len(multi) != 2 || multi[1].Text != `"secret" b` {
		t.Fatalf("|| 分支未逐条展开：%+v", multi)
	}

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	window := Window(`"acme.com" -is:fork`, from, from.Add(WindowSize))
	if !strings.Contains(window, "committed:>=2026-03-01") || !strings.Contains(window, "committed:<2026-03-31") {
		t.Fatalf("窗口格式错误: %s", window)
	}
}
