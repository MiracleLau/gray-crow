package scanner

import (
	"math"
	"regexp"
	"strings"

	"graycrow/internal/scanner/patterns"
)

const (
	snippetLimit  = 500
	contextMargin = 120
	entropyMin    = 4.0
	entropyLenMin = 20
)

type Verdict struct {
	DetectorID string
	Severity   string
	Snippet    string
	Matched    bool
}

var (
	placeholderRe = regexp.MustCompile(`(?i)(example|placeholder|your[_-]|changeme|redacted|dummy|sample|fake|xxxx|aaaa|abcabc|1234567890|token_here|password_here|test[_-]?(key|token|secret))`)
	highEntropyRe = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{20,}`)
)

// Detect 二次判定：正则规则集优先，其次高熵串，都没有则降级为低置信度。
func Detect(text string) Verdict {
	text = strings.TrimSpace(text)
	if text == "" {
		return Verdict{Severity: "low"}
	}
	for i := range patterns.All {
		p := &patterns.All[i]
		start, end, ok := p.Locate(text)
		if !ok || placeholderRe.MatchString(text[start:end]) {
			continue
		}
		return Verdict{
			DetectorID: p.ID,
			Severity:   p.Severity,
			Snippet:    window(text, start, end),
			Matched:    true,
		}
	}

	for _, loc := range highEntropyRe.FindAllStringIndex(text, -1) {
		token := text[loc[0]:loc[1]]
		if placeholderRe.MatchString(token) || shannon(token) < entropyMin {
			continue
		}
		severity := "medium"
		if hasSecretContext(text) {
			severity = "high"
		}
		return Verdict{
			DetectorID: "high-entropy",
			Severity:   severity,
			Snippet:    window(text, loc[0], loc[1]),
			Matched:    true,
		}
	}

	return Verdict{Severity: "low", Snippet: truncate(text, snippetLimit)}
}

func hasSecretContext(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range []string{"key", "secret", "token", "password", "passwd", "pwd", "credential"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// window 取命中位置前后各 120 字符作为展示上下文，整体不超过 500 字符。
func window(text string, start, end int) string {
	from := start - contextMargin
	if from < 0 {
		from = 0
	}
	to := end + contextMargin
	if to > len(text) {
		to = len(text)
	}
	out := text[from:to]
	if len(out) > snippetLimit {
		out = out[:snippetLimit]
	}
	return out
}

func shannon(s string) float64 {
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	total := float64(len([]rune(s)))
	var sum float64
	for _, n := range counts {
		p := float64(n) / total
		sum -= p * math.Log2(p)
	}
	return sum
}
