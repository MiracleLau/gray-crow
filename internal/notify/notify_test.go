package notify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBatchTextSummary(t *testing.T) {
	items := []BatchItem{{Repo: "acme/app", Path: "a.go", Detector: "aws-key", URL: "u1"}}
	for i := 0; i < 24; i++ {
		items = append(items, BatchItem{Repo: "acme/lib", Path: "b.go", Detector: "generic", URL: "u2"})
	}
	text := Batch{Title: "acme 巡检", Items: items}.Text()
	if !strings.Contains(text, "本次扫描发现 25 条新问题") {
		t.Fatalf("缺少总条数: %s", text)
	}
	if !strings.Contains(text, "acme/lib (24) / acme/app (1)") {
		t.Fatalf("仓库分布应按条数降序: %s", text)
	}
	if !strings.Contains(text, "其余 5 条请在控制台查看") {
		t.Fatalf("超出上限应只给计数: %s", text)
	}
	if n := strings.Count(text, ") acme/"); n != batchListCap {
		t.Fatalf("明细应截断到 %d 条，实际 %d", batchListCap, n)
	}
}

func TestRenderDefaultIsValidJSON(t *testing.T) {
	vars := Batch{Title: `引号"注入`, Items: []BatchItem{{Repo: "acme/app", Path: "a.go", Detector: "aws-key", URL: "u"}}}.Vars()
	raw, _ := json.Marshal(vars)
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("默认正文应为合法 JSON: %v", err)
	}
	if out["count"] != "1" || out["title"] != `引号"注入` {
		t.Fatalf("默认正文字段异常: %+v", out)
	}

	tpl := `{"msg":"{{count}} 条","title":"{{title}}"}`
	rendered := Render(tpl, vars, true)
	if !json.Valid([]byte(rendered)) {
		t.Fatalf("转义后应是合法 JSON: %s", rendered)
	}
	if got := Render(tpl, vars, true); got != `{"msg":"1 条","title":"引号\"注入"}` {
		t.Fatalf("变量应按 JSON 规则转义: %s", got)
	}
}

func TestSignAlgorithms(t *testing.T) {
	// 官方示例参数：secret=demo，timestamp=1599360473
	if got := feishuSign("demo", 1599360473); got != "l1N0gAcBjdwBvGm1xMjOF0XSyaLRpR7tuO5dHfhAYc8=" {
		t.Fatalf("飞书签名不符: %s", got)
	}
	if got := dingtalkSign("demo", 1599360473); got != "DkEDUyVoOOJ539qBvUgMyzkzIdRWlf4e%2FV3Rq%2BONJnQ%3D" {
		t.Fatalf("钉钉签名不符: %s", got)
	}
	if got := withDingtalkSign("https://oapi.dingtalk.com/robot/send?access_token=x", "demo"); !strings.Contains(got, "&timestamp=") || !strings.Contains(got, "&sign=") {
		t.Fatalf("钉钉签名应追加到已有 query 后: %s", got)
	}
}

func TestBusinessError(t *testing.T) {
	if got := businessError(`{"errcode":0,"errmsg":"ok"}`); got != "" {
		t.Fatalf("成功不应报错: %s", got)
	}
	if got := businessError(`{"code":19021,"msg":"sign match fail"}`); !strings.Contains(got, "19021") {
		t.Fatalf("飞书业务错误应识别: %s", got)
	}
	if got := businessError(`{"errcode":93000,"errmsg":"invalid webhook url"}`); !strings.Contains(got, "invalid webhook url") {
		t.Fatalf("企微/钉钉业务错误应带原文: %s", got)
	}
}
