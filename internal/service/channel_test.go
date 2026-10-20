package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"graycrow/internal/store"
)

func TestWebhookTemplateAndSign(t *testing.T) {
	db := newTestDB(t)
	channels := NewChannelService(db)
	templates := NewTemplateService(db)
	actor := adminActor()

	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tpl, err := templates.Create(actor, TemplateInput{
		Name: "飞书汇总", Title: "发现 {{count}} 条",
		Body: "[{{title}}] {{count}} 条\n{{items}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	secret := "demo"
	row, err := channels.Create(actor, ChannelInput{
		Name: "飞书群", Type: "feishu", TemplateID: &tpl.ID, Enabled: true,
		Config: map[string]any{"url": server.URL, "body_format": "text", "sign_mode": "feishu"},
		Secret: &secret,
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := channels.Test(row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("测试发送应成功: %s", res.Response)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("请求体应是合法 JSON: %v\n%s", err, body)
	}
	if sent["sign"] == "" || sent["timestamp"] == "" {
		t.Fatalf("飞书加签应写入 timestamp/sign: %s", body)
	}
	if sent["msg_type"] != "text" {
		t.Fatalf("飞书应使用 msg_type text: %s", body)
	}
	content, _ := sent["content"].(map[string]any)
	text, _ := content["text"].(string)
	if !strings.Contains(text, "测试规则") || !strings.Contains(text, "1 条") {
		t.Fatalf("模板变量未渲染: %s", text)
	}

	// 列表只返回打码后的地址
	rows, err := channels.List()
	if err != nil || len(rows) != 1 {
		t.Fatalf("列表异常: %v %+v", err, rows)
	}
	hint, _ := rows[0].Config["url_hint"].(string)
	if hint == "" || strings.Contains(hint, "127.0.0.1") == false {
		t.Fatalf("地址提示应保留域名: %+v", rows[0].Config)
	}
	if _, ok := rows[0].Config["url"]; ok {
		t.Fatal("列表不应返回完整 webhook 地址")
	}

	// 模板被引用时不能删除
	if err := templates.Delete(tpl.ID); !errors.Is(err, ErrTemplateInUse) {
		t.Fatalf("被引用模板应拒绝删除，实际 %v", err)
	}
}

func TestWebhookDingtalkSignAndDraft(t *testing.T) {
	db := newTestDB(t)
	channels := NewChannelService(db)

	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	secret := "demo"
	res, err := channels.TestDraft(ChannelInput{
		Name: "草稿", Type: "dingtalk", Enabled: true,
		Config: map[string]any{"url": server.URL + "/hook?access_token=x", "body_format": "text", "sign_mode": "dingtalk"},
		Secret: &secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("草稿测试应成功: %s", res.Response)
	}
	if !strings.Contains(query, "access_token=x") || !strings.Contains(query, "sign=") || !strings.Contains(query, "timestamp=") {
		t.Fatalf("钉钉加签应追加到 URL: %s", query)
	}

	// 关闭加签后不应带 timestamp/sign
	query = ""
	res, err = channels.TestDraft(ChannelInput{
		Name: "草稿2", Type: "dingtalk", Enabled: true,
		Config: map[string]any{"url": server.URL + "/hook?access_token=x", "body_format": "text"},
	})
	if err != nil || !res.OK {
		t.Fatalf("未加签草稿应成功: %v %s", err, res.Response)
	}
	if query != "access_token=x" {
		t.Fatalf("未开启加签不应追加参数: %s", query)
	}
}

func TestPlatformMarkdown(t *testing.T) {
	db := newTestDB(t)
	channels := NewChannelService(db)

	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = map[string]any{}
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wecom, err := channels.Create(adminActor(), ChannelInput{
		Name: "企微", Type: "wecom", Enabled: true,
		Config: map[string]any{"url": server.URL, "body_format": "markdown"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := channels.Send(wecom, "# 标题 2", nil, true); err != nil {
		t.Fatal(err)
	}
	if body["msgtype"] != "markdown" {
		t.Fatalf("企微应使用 msgtype markdown: %+v", body)
	}
	md, _ := body["markdown"].(map[string]any)
	if md["content"] != "# 标题 2" {
		t.Fatalf("企微 markdown 内容异常: %+v", md)
	}

	feishu, err := channels.Create(adminActor(), ChannelInput{
		Name: "飞书", Type: "feishu", Enabled: true,
		Config: map[string]any{"url": server.URL, "body_format": "markdown"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := channels.Send(feishu, "**加粗**", nil, true); err != nil {
		t.Fatal(err)
	}
	if body["msg_type"] != "interactive" {
		t.Fatalf("飞书 markdown 应走 interactive 卡片: %+v", body)
	}
	card, _ := body["card"].(map[string]any)
	elements, _ := card["elements"].([]any)
	if len(elements) != 1 {
		t.Fatalf("卡片元素异常: %+v", card)
	}
	el, _ := elements[0].(map[string]any)
	if el["tag"] != "markdown" || el["content"] != "**加粗**" {
		t.Fatalf("飞书 markdown 元素异常: %+v", el)
	}
}

func TestChannelSignNeedsSecret(t *testing.T) {
	db := newTestDB(t)
	channels := NewChannelService(db)
	_, err := channels.Create(adminActor(), ChannelInput{
		Name: "飞书", Type: "feishu", Enabled: true,
		Config: map[string]any{"url": "https://example.com/h", "sign_mode": "feishu"},
	})
	if !errors.Is(err, ErrChannelSign) {
		t.Fatalf("开启加签但没密钥应拒绝，实际 %v", err)
	}
}

func TestChannelRejectsBadWebhook(t *testing.T) {
	db := newTestDB(t)
	channels := NewChannelService(db)
	if _, err := channels.Create(adminActor(), ChannelInput{Name: "坏地址", Type: "webhook", Config: map[string]any{"url": "ftp://x"}}); !errors.Is(err, ErrChannelURL) {
		t.Fatalf("非法地址应拒绝，实际 %v", err)
	}
	if _, err := channels.Create(adminActor(), ChannelInput{Name: "坏头", Type: "webhook", Config: map[string]any{"url": "https://example.com/h"}, Headers: ptr("not-json")}); !errors.Is(err, ErrChannelHeaders) {
		t.Fatalf("非法 Header 应拒绝，实际 %v", err)
	}
}

func ptr(s string) *string { return &s }

func TestDefaultTemplateFromDB(t *testing.T) {
	db := newTestDB(t)
	created, err := store.SeedNotifyTemplate(db)
	if err != nil || !created {
		t.Fatalf("应写入默认模板: created=%v err=%v", created, err)
	}
	if again, _ := store.SeedNotifyTemplate(db); again {
		t.Fatal("默认模板已存在时不应重复写入")
	}

	templates := NewTemplateService(db)
	rows, err := templates.List()
	if err != nil || len(rows) != 1 || !rows[0].IsDefault {
		t.Fatalf("默认模板应在列表首位: %+v %v", rows, err)
	}
	def := rows[0]
	if _, err := templates.Update(def.ID, TemplateInput{Name: def.Name, Title: "T {{count}}", Body: "BODY {{count}} {{title}}"}); err != nil {
		t.Fatal(err)
	}

	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	channels := NewChannelService(db)
	row, err := channels.Create(adminActor(), ChannelInput{
		Name: "未绑定模板", Type: "webhook", Enabled: true,
		Config: map[string]any{"url": server.URL, "body_format": "text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, _, err := channels.Send(row, "raw", map[string]string{"count": "2", "title": "规则"}, true)
	if err != nil || status != http.StatusOK {
		t.Fatalf("发送失败: %d %v", status, err)
	}
	if body != "BODY 2 规则" {
		t.Fatalf("未绑定模板时应使用默认模板，实际 %q", body)
	}
	if err := templates.Delete(def.ID); !errors.Is(err, ErrTemplateDefault) {
		t.Fatalf("默认模板应禁止删除，实际 %v", err)
	}
}
