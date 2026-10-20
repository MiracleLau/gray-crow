package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/smtp"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"graycrow/internal/model"
)

// batchListCap 汇总消息里最多列出的明细条数，再多只给计数（避免消息体过大被上游截断）。
const batchListCap = 20

type Payload struct {
	Title    string
	Repo     string
	Path     string
	Detector string
	Keyword  string
	URL      string
	Snippet  string
}

func (p Payload) Text() string {
	return fmt.Sprintf("【灰鸦告警】%s\n仓库：%s\n文件：%s\n判定：%s\n关键词：%s\n链接：%s\n片段：%s",
		p.Title, p.Repo, p.Path, p.Detector, p.Keyword, p.URL, truncate(p.Snippet, 300))
}

func (p Payload) Vars() map[string]string {
	return map[string]string{
		"title":    p.Title,
		"repo":     p.Repo,
		"path":     p.Path,
		"detector": p.Detector,
		"keyword":  p.Keyword,
		"url":      p.URL,
		"snippet":  truncate(p.Snippet, 300),
	}
}

// BatchItem 汇总消息里的单条命中。
type BatchItem struct {
	Repo     string `json:"repo"`
	Path     string `json:"path"`
	Detector string `json:"detector"`
	Keyword  string `json:"keyword"`
	URL      string `json:"url"`
}

// Batch 一次扫描的汇总通知：一条消息覆盖本次全部新命中。
type Batch struct {
	Title string
	Items []BatchItem
}

func (b Batch) Text() string {
	lines := []string{
		fmt.Sprintf("【灰鸦告警汇总】%s：本次扫描发现 %d 条新问题", b.Title, len(b.Items)),
	}
	if repos := b.repos(); repos != "" {
		lines = append(lines, "仓库："+repos)
	}
	lines = append(lines, b.detailLines()...)
	if rest := len(b.Items) - batchListCap; rest > 0 {
		lines = append(lines, fmt.Sprintf("…其余 %d 条请在控制台查看", rest))
	}
	return strings.Join(lines, "\n")
}

func (b Batch) Vars() map[string]string {
	vars := map[string]string{
		"title": b.Title,
		"count": strconv.Itoa(len(b.Items)),
		"items": strings.Join(b.detailLines(), "\n"),
		"repos": b.repos(),
	}
	return vars
}

func (b Batch) detailLines() []string {
	lines := []string{}
	for i, it := range b.Items {
		if i >= batchListCap {
			break
		}
		lines = append(lines, fmt.Sprintf("%d) %s: %s [%s] %s", i+1, it.Repo, it.Path, it.Detector, it.URL))
	}
	return lines
}

func (b Batch) repos() string {
	counts := map[string]int{}
	for _, it := range b.Items {
		counts[it.Repo]++
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s (%d)", name, counts[name]))
	}
	return strings.Join(parts, " / ")
}

// Credentials 渠道的解密后敏感值；空字段表示沿用旧配置或未设置。
type Credentials struct {
	URL        string
	SignSecret string
	Headers    map[string]string
	Password   string
}

// Message 一次投递的正文：Subject 仅邮件使用，Text 为已渲染的内容。
type Message struct {
	Subject string
	Text    string
}

// Render 把模板里的 {{key}} 替换为变量值。jsonMode 时按 JSON 字符串转义，
// 避免片段里的引号把请求体拼成非法 JSON。
func Render(tpl string, vars map[string]string, jsonMode bool) string {
	out := tpl
	for k, v := range vars {
		if jsonMode {
			if raw, err := json.Marshal(v); err == nil {
				v = string(raw[1 : len(raw)-1])
			}
		}
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// Send 投递一条消息。返回上游 HTTP 状态码与响应体片段；SMTP 成功返回 200/“sent”。
func Send(ch *model.Channel, creds Credentials, msg Message, vars map[string]string) (statusCode int, respBody string, err error) {
	switch ch.Type {
	case "feishu", "wecom", "dingtalk":
		hook := creds.URL
		if hook == "" {
			hook, _ = ch.Config["webhook"].(string)
		}
		if hook == "" {
			return 0, "", fmt.Errorf("缺少 webhook 地址")
		}
		body := platformBody(ch, msg, vars)
		if signEnabled(ch) && creds.SignSecret != "" {
			switch ch.Type {
			case "feishu":
				addFeishuSign(body, creds.SignSecret)
			case "dingtalk":
				hook = withDingtalkSign(hook, creds.SignSecret)
			}
		}
		status, raw, err := postJSON(hook, body, creds.Headers, 10*time.Second)
		if err == nil {
			if reason := businessError(raw); reason != "" {
				return status, raw, fmt.Errorf("%s", reason)
			}
		}
		return status, raw, err
	case "webhook":
		hook := creds.URL
		if hook == "" {
			hook, _ = ch.Config["url"].(string)
		}
		if hook == "" {
			return 0, "", fmt.Errorf("缺少 webhook 地址")
		}
		format, _ := ch.Config["body_format"].(string)
		if format == "" {
			format = "json"
		}
		raw := msg.Text
		if raw == "" {
			buf, _ := json.Marshal(vars)
			raw = string(buf)
		}
		if format == "json" && !json.Valid([]byte(raw)) {
			return 0, "", fmt.Errorf("消息体不是合法 JSON")
		}
		method, _ := ch.Config["method"].(string)
		if method == "" {
			method = http.MethodPost
		}
		contentType := map[string]string{"json": "application/json", "markdown": "text/markdown; charset=utf-8"}[format]
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		status, body, err := postRaw(hook, method, []byte(raw), creds.Headers, contentType, 10*time.Second)
		return status, truncate(body, 500), err
	case "smtp":
		host, _ := ch.Config["host"].(string)
		port := toInt(ch.Config["port"])
		from, _ := ch.Config["from"].(string)
		to := configList(ch.Config, "to")
		if host == "" || port == 0 || from == "" || len(to) == 0 {
			return 0, "", fmt.Errorf("SMTP 配置不完整")
		}
		subject := msg.Subject
		if subject == "" {
			subject = "【灰鸦告警】" + vars["title"]
		}
		msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
			from, strings.Join(to, ","), mime.QEncoding.Encode("utf-8", subject), msg.Text)
		addr := host + ":" + strconv.Itoa(port)
		var auth smtp.Auth
		if creds.Password != "" {
			user := strings.Split(from, "@")[0]
			auth = smtp.PlainAuth("", user, creds.Password, host)
		}
		if err := smtp.SendMail(addr, auth, from, to, []byte(msg)); err != nil {
			return 0, "", err
		}
		return 200, "sent", nil
	}
	return 0, "", fmt.Errorf("不支持的渠道类型: %s", ch.Type)
}

// platformBody 按平台组装报文：飞书用 msg_type，企微/钉钉用 msgtype。
// Markdown 在飞书走 interactive 卡片（官方支持 markdown 元素）。
func platformBody(ch *model.Channel, msg Message, vars map[string]string) map[string]any {
	format, _ := ch.Config["body_format"].(string)
	if format == "" {
		format = "text"
	}
	text := truncateBytes(msg.Text, platformLimit(ch.Type, format))
	switch ch.Type {
	case "feishu":
		if format == "markdown" {
			return map[string]any{
				"msg_type": "interactive",
				"card": map[string]any{
					"elements": []map[string]any{{"tag": "markdown", "content": text}},
				},
			}
		}
		return map[string]any{"msg_type": "text", "content": map[string]any{"text": text}}
	case "wecom":
		if format == "markdown" {
			return map[string]any{"msgtype": "markdown", "markdown": map[string]any{"content": text}}
		}
		return map[string]any{"msgtype": "text", "text": map[string]any{"content": text}}
	default: // dingtalk
		if format == "markdown" {
			title := msg.Subject
			if title == "" {
				title = vars["title"]
			}
			if title == "" {
				title = "灰鸦告警"
			}
			return map[string]any{"msgtype": "markdown", "markdown": map[string]any{"title": title, "text": text}}
		}
		at := map[string]any{"atMobiles": configList(ch.Config, "at_mobiles")}
		return map[string]any{"msgtype": "text", "text": map[string]any{"content": text}, "at": at}
	}
}

// signEnabled 只认当前平台对应的加签方式，避免换类型后误用旧配置。
func signEnabled(ch *model.Channel) bool {
	mode, _ := ch.Config["sign_mode"].(string)
	return mode != "" && mode == ch.Type
}

// platformLimit 上游单条消息的字节上限；超长直接截断，避免整条被拒。
func platformLimit(kind, format string) int {
	switch kind {
	case "wecom":
		if format == "markdown" {
			return 4096
		}
		return 2048
	case "feishu":
		return 20000
	default:
		return 18000
	}
}

// feishuSign：以 timestamp+"\n"+secret 为密钥，对空串做 HMAC-SHA256（飞书官方算法）。
func feishuSign(secret string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(fmt.Sprintf("%d\n%s", ts, secret)))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// addFeishuSign 按飞书要求把 timestamp/sign 放到报文顶层。
func addFeishuSign(body map[string]any, secret string) {
	ts := time.Now().Unix()
	body["timestamp"] = strconv.FormatInt(ts, 10)
	body["sign"] = feishuSign(secret, ts)
}

// truncateBytes 按字节截断且不切断 UTF-8 字符，给省略号留出空间。
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	limit := n - len("…")
	if limit < 0 {
		limit = 0
	}
	cut := s[:limit]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…"
}

// dingtalkSign：以 secret 为密钥，对 timestamp+"\n"+secret 做 HMAC-SHA256 再 URL 编码（钉钉官方算法）。
func dingtalkSign(secret string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d\n%s", ts, secret)))
	return url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

// withDingtalkSign 把 timestamp/sign 追加到 webhook 地址上，已有 query 时用 & 连接。
func withDingtalkSign(hook, secret string) string {
	ts := time.Now().Unix()
	sep := "?"
	if strings.Contains(hook, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%stimestamp=%d&sign=%s", hook, sep, ts, dingtalkSign(secret, ts))
}

// businessError 解析钉钉/企微/飞书返回体里的业务错误码：HTTP 200 也可能是失败。
func businessError(body string) string {
	var r struct {
		Errcode *int   `json:"errcode"`
		Code    *int   `json:"code"`
		Errmsg  string `json:"errmsg"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &r) != nil {
		return ""
	}
	if r.Errcode != nil && *r.Errcode != 0 {
		return fmt.Sprintf("%d: %s", *r.Errcode, firstNonEmpty(r.Errmsg, r.Msg, r.Message))
	}
	if r.Code != nil && *r.Code != 0 {
		return fmt.Sprintf("%d: %s", *r.Code, firstNonEmpty(r.Msg, r.Errmsg, r.Message))
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return "上游返回失败"
}

func postJSON(url string, body any, headers map[string]string, timeout time.Duration) (int, string, error) {
	raw, _ := json.Marshal(body)
	return postRaw(url, http.MethodPost, raw, headers, "application/json", timeout)
}

func postRaw(url, method string, raw []byte, headers map[string]string, contentType string, timeout time.Duration) (int, string, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(raw))
	if err != nil {
		return 0, "", err
	}
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, truncate(buf.String(), 500), nil
}

func configList(cfg map[string]any, key string) []string {
	out := []string{}
	switch v := cfg[key].(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = v
	}
	return out
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
