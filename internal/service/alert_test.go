package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"graycrow/internal/model"
)

func newAlertFixture(t *testing.T) (*AlertService, *RuleService, *ChannelService, *model.User, *model.User, *model.Rule) {
	t.Helper()
	db := newTestDB(t)
	channels := NewChannelService(db)
	rules := NewRuleService(db, 60, time.UTC)
	alerts := NewAlertService(db, rules, channels)

	admin := adminActor()
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{Username: "alice", Nickname: "Alice", Role: model.RoleViewer, Status: 1}
	if err := db.Create(viewer).Error; err != nil {
		t.Fatal(err)
	}
	rule, err := rules.Create(admin, RuleInput{Name: "acme", Keywords: []string{"acme.com"}, IntervalSec: 60, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := rules.SetViewers(rule.ID, []uint{viewer.ID}); err != nil {
		t.Fatal(err)
	}
	return alerts, rules, channels, admin, viewer, &rule.Rule
}

func seedAlert(t *testing.T, alerts *AlertService, ruleID uint, status string) *model.Alert {
	t.Helper()
	now := time.Now().UTC()
	alert := &model.Alert{
		RuleID:         ruleID,
		RepoFullName:   "acme/backend",
		Path:           "config/app.yaml",
		MatchedKeyword: "acme.com",
		DetectorID:     "generic-credential",
		Snippet:        `password: "S3cr3t"`,
		Status:         status,
		Fingerprint:    fmt.Sprintf("fp-%d-%s", ruleID, status),
		FirstSeenAt:    now,
		LastSeenAt:     now,
	}
	alert.Fingerprint = fmt.Sprintf("fp-%d-%s-%d", ruleID, status, time.Now().UnixNano())
	if err := alerts.db.Create(alert).Error; err != nil {
		t.Fatal(err)
	}
	return alert
}

func TestAlertScopeAndStatusMachine(t *testing.T) {
	alerts, rules, _, admin, viewer, rule := newAlertFixture(t)
	inScope := seedAlert(t, alerts, rule.ID, model.AlertPending)

	otherRule, err := rules.Create(admin, RuleInput{Name: "other", Keywords: []string{"other.com"}, IntervalSec: 60, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	outOfScope := seedAlert(t, alerts, otherRule.ID, model.AlertPending)

	if _, err := alerts.Get(viewer, outOfScope.ID); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("越权读取应被拒绝，实际 %v", err)
	}
	if _, err := alerts.SetStatus(viewer, outOfScope.ID, model.AlertResolved, ""); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("越权处置应被拒绝，实际 %v", err)
	}

	updated, err := alerts.SetStatus(viewer, inScope.ID, model.AlertResolved, "已通知负责人")
	if err != nil {
		t.Fatalf("范围内处置失败: %v", err)
	}
	if updated.Status != model.AlertResolved || updated.HandledAt == nil || updated.Note != "已通知负责人" {
		t.Fatalf("处置结果异常: %+v", updated)
	}
	if updated.HandledByName != "Alice" {
		t.Fatalf("处置人应回显昵称: %q", updated.HandledByName)
	}
	if len(updated.Events) != 1 || updated.Events[0].FromStatus != model.AlertPending || updated.Events[0].ToStatus != model.AlertResolved {
		t.Fatalf("处置轨迹异常: %+v", updated.Events)
	}

	if _, err := alerts.SetStatus(viewer, inScope.ID, model.AlertPending, ""); !errors.Is(err, ErrStatusInvalid) {
		t.Fatalf("普通用户回退到 pending 应被拒绝，实际 %v", err)
	}
	back, err := alerts.SetStatus(admin, inScope.ID, model.AlertPending, "误操作回退")
	if err != nil {
		t.Fatalf("管理员回退失败: %v", err)
	}
	if back.Status != model.AlertPending || back.HandledAt != nil {
		t.Fatalf("回退未清理处置信息: %+v", back)
	}
	if _, err := alerts.SetStatus(admin, inScope.ID, "nonsense", ""); !errors.Is(err, ErrStatusInvalid) {
		t.Fatalf("非法状态应被拒绝，实际 %v", err)
	}

	updatedIDs, skipped, err := alerts.BulkStatus(admin, []uint{inScope.ID, 99999}, model.AlertIgnored, "批量忽略")
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedIDs) != 1 || updatedIDs[0] != inScope.ID || len(skipped) != 1 || skipped[0].ID != 99999 {
		t.Fatalf("批量结果异常: updated=%v skipped=%v", updatedIDs, skipped)
	}

	tooMany := make([]uint, 0, maxBulkAlerts+1)
	for i := 0; i <= maxBulkAlerts; i++ {
		tooMany = append(tooMany, uint(i+1))
	}
	if _, _, err := alerts.BulkStatus(admin, tooMany, model.AlertIgnored, ""); !errors.Is(err, ErrTooManyIDs) {
		t.Fatalf("超量批量应被拒绝，实际 %v", err)
	}
}

func TestAlertAllowlistFromAlert(t *testing.T) {
	alerts, _, _, admin, _, rule := newAlertFixture(t)
	alert := seedAlert(t, alerts, rule.ID, model.AlertPending)

	updated, err := alerts.AllowlistAlert(admin, alert.ID, "rule", "示例仓库", nil)
	if err != nil {
		t.Fatalf("加白失败: %v", err)
	}
	if updated.Status != model.AlertAllowlisted || !updated.Allowed {
		t.Fatalf("加白后状态异常: %+v", updated)
	}
	if len(updated.Events) != 1 || updated.Events[0].Action != "allowlist" {
		t.Fatalf("加白轨迹异常: %+v", updated.Events)
	}
	var rows []model.Allowlist
	if err := alerts.db.Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatalf("白名单应写入 1 条: %v %+v", err, rows)
	}
	if rows[0].Scope != "rule" || rows[0].RuleID == nil || *rows[0].RuleID != rule.ID || rows[0].CreatedByName != "管理员" {
		t.Fatalf("白名单字段异常: %+v", rows[0])
	}

	if _, err := alerts.AllowlistAlert(admin, alert.ID, "rule", "重复加白", nil); err != nil {
		t.Fatalf("重复加白不应报错: %v", err)
	}
	var count int64
	alerts.db.Model(&model.Allowlist{}).Count(&count)
	if count != 1 {
		t.Fatalf("重复加白不应产生重复记录，实际 %d", count)
	}
	if _, err := alerts.AllowlistAlert(admin, alert.ID, "bogus", "", nil); !errors.Is(err, ErrScopeInvalid) {
		t.Fatalf("非法范围应被拒绝，实际 %v", err)
	}
}

func TestDispatchDryRunAndStatusFilter(t *testing.T) {
	alerts, _, _, admin, _, rule := newAlertFixture(t)
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	secret := `{"X-Token":"abc"}`
	if _, err := alerts.channels.Create(admin, ChannelInput{
		Name: "网关", Type: "webhook", Enabled: true,
		Config: map[string]any{"url": server.URL}, Secret: &secret,
	}); err != nil {
		t.Fatal(err)
	}
	pending := seedAlert(t, alerts, rule.ID, model.AlertPending)
	resolved := seedAlert(t, alerts, rule.ID, model.AlertResolved)
	allowlisted := seedAlert(t, alerts, rule.ID, model.AlertAllowlisted)

	dryRun := *rule
	dryRun.DryRun = true
	alerts.Dispatch(&dryRun, []model.Alert{*pending})
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("干跑规则不应投递")
	}

	alerts.Dispatch(rule, []model.Alert{*pending, *resolved, *allowlisted})
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("一批只应发一条汇总消息，实际请求 %d 次", got)
	}
	records, total, err := alerts.NotifyRecords(admin, NotifyQuery{Page: 1, Size: 10})
	if err != nil || total != 1 {
		t.Fatalf("通知记录异常: %v total=%d %+v", err, total, records)
	}
	if records[0].Status != "success" || records[0].ChannelName != "网关" || records[0].RuleName != "acme" ||
		records[0].AlertCount != 1 || records[0].Attempt != 1 || records[0].HTTPStatus == nil || *records[0].HTTPStatus != 200 {
		t.Fatalf("通知记录字段异常: %+v", records[0])
	}
	if !strings.Contains(records[0].Content, "本次扫描发现 1 条新问题") {
		t.Fatalf("通知记录应保存消息正文: %q", records[0].Content)
	}
	for _, id := range []uint{pending.ID} {
		detail, err := alerts.Get(admin, id)
		if err != nil {
			t.Fatal(err)
		}
		if detail.NotifiedAt == nil {
			t.Fatalf("已投递告警应记 notified_at: %+v", detail)
		}
	}
	detail, err := alerts.Get(admin, resolved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.NotifiedAt != nil {
		t.Fatal("非 pending 告警不应被通知")
	}
}

func TestDispatchBatchesAlertsIntoOneMessage(t *testing.T) {
	alerts, _, _, admin, _, rule := newAlertFixture(t)
	var hits int32
	body := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if _, err := alerts.channels.Create(admin, ChannelInput{
		Name: "批次", Type: "webhook", Enabled: true, Config: map[string]any{"url": server.URL},
	}); err != nil {
		t.Fatal(err)
	}

	pending := []model.Alert{}
	for i := 0; i < 3; i++ {
		pending = append(pending, *seedAlert(t, alerts, rule.ID, model.AlertPending))
	}
	alerts.Dispatch(rule, pending)
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("3 条命中应合成一条消息，实际请求 %d 次", got)
	}
	if !strings.Contains(body, `"count":"3"`) {
		t.Fatalf("消息体应带本批条数: %s", body)
	}

	records, total, err := alerts.NotifyRecords(admin, NotifyQuery{Page: 1, Size: 10})
	if err != nil || total != 1 {
		t.Fatalf("通知记录异常: %v total=%d", err, total)
	}
	if records[0].AlertCount != 3 || !strings.Contains(records[0].Content, "本次扫描发现 3 条新问题") {
		t.Fatalf("通知记录应记本批条数与正文: %+v", records[0])
	}
	var notified int64
	if err := alerts.db.Model(&model.Alert{}).Where("notified_at IS NOT NULL").Count(&notified).Error; err != nil {
		t.Fatal(err)
	}
	if notified != 3 {
		t.Fatalf("批次内每条告警都应记 notified_at，实际 %d", notified)
	}

	// 已可见规则之外的用户看不到通知记录
	viewer := &model.User{Username: "bob", Role: model.RoleViewer, Status: 1}
	if err := alerts.db.Create(viewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, total, err := alerts.NotifyRecords(viewer, NotifyQuery{Page: 1, Size: 10}); err != nil || total != 0 {
		t.Fatalf("范围外用户不应看到通知记录: %v total=%d", err, total)
	}
}

func TestDispatchRetriesAndLogsFailures(t *testing.T) {
	alerts, _, _, admin, _, rule := newAlertFixture(t)
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if _, err := alerts.channels.Create(admin, ChannelInput{
		Name: "会抖动", Type: "webhook", Enabled: true, Config: map[string]any{"url": server.URL},
	}); err != nil {
		t.Fatal(err)
	}

	alert := seedAlert(t, alerts, rule.ID, model.AlertPending)
	alerts.Dispatch(rule, []model.Alert{*alert})

	records, total, err := alerts.NotifyRecords(admin, NotifyQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || records[0].Status != "success" || records[0].Attempt != 2 {
		t.Fatalf("重试后应只留一条成功记录: %+v", records)
	}

	// 4xx（机器人被移除等）不该重试
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		fmt.Fprint(w, `{"errcode":19024}`)
	}))
	defer dead.Close()
	if _, err := alerts.channels.Create(admin, ChannelInput{
		Name: "坏机器人", Type: "webhook", Enabled: true, Config: map[string]any{"url": dead.URL},
	}); err != nil {
		t.Fatal(err)
	}
	failing := seedAlert(t, alerts, rule.ID, model.AlertPending)
	alerts.Dispatch(rule, []model.Alert{*failing})
	records, total, err = alerts.NotifyRecords(admin, NotifyQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("两个渠道应各留一条记录，实际 %+v", records)
	}
	for _, entry := range records {
		if entry.ChannelName == "坏机器人" && (entry.Attempt != 1 || entry.Status != "failed" || !strings.Contains(entry.Error, "410")) {
			t.Fatalf("410 不应重试且需记录错误体: %+v", entry)
		}
	}

	// 重新投递：无视状态与干跑
	ok, failed, err := alerts.Renotify(admin, failing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok != 1 || failed != 1 {
		t.Fatalf("重投结果异常: ok=%d failed=%d", ok, failed)
	}
}

func TestDeleteAlertsAndNotifyRecords(t *testing.T) {
	alerts, _, _, _, _, rule := newAlertFixture(t)
	keep := seedAlert(t, alerts, rule.ID, model.AlertPending)
	dropA := seedAlert(t, alerts, rule.ID, model.AlertResolved)
	dropB := seedAlert(t, alerts, rule.ID, model.AlertIgnored)

	events := []model.AlertEvent{
		{AlertID: dropA.ID, Action: "create"},
		{AlertID: keep.ID, Action: "create"},
	}
	if err := alerts.db.Create(&events).Error; err != nil {
		t.Fatal(err)
	}
	rec := model.NotifyRecord{
		RuleID: rule.ID, AlertID: &dropA.ID, ChannelID: 1,
		AlertCount: 1, Status: "success", SentAt: time.Now().UTC(),
	}
	if err := alerts.db.Create(&rec).Error; err != nil {
		t.Fatal(err)
	}

	n, err := alerts.DeleteAlerts([]uint{dropA.ID, dropB.ID})
	if err != nil || n != 2 {
		t.Fatalf("删除告警 n=%d err=%v", n, err)
	}
	var count int64
	alerts.db.Model(&model.Alert{}).Where("id IN ?", []uint{dropA.ID, dropB.ID}).Count(&count)
	if count != 0 {
		t.Fatalf("告警未删除: %d", count)
	}
	alerts.db.Model(&model.AlertEvent{}).Where("alert_id IN ?", []uint{dropA.ID, dropB.ID}).Count(&count)
	if count != 0 {
		t.Fatalf("时间线应级联删除: %d", count)
	}
	alerts.db.Model(&model.AlertEvent{}).Where("alert_id = ?", keep.ID).Count(&count)
	if count != 1 {
		t.Fatalf("其他告警的时间线被误删")
	}
	// 通知记录独立保留，不随告警删除
	alerts.db.Model(&model.NotifyRecord{}).Where("id = ?", rec.ID).Count(&count)
	if count != 1 {
		t.Fatalf("通知记录不应随告警删除")
	}

	m, err := alerts.DeleteNotifyRecords([]uint{rec.ID, 999999})
	if err != nil || m != 1 {
		t.Fatalf("删除通知记录 n=%d err=%v", m, err)
	}
	if _, err := alerts.DeleteAlerts(make([]uint, 201)); !errors.Is(err, ErrTooManyIDs) {
		t.Fatalf("超量删除应被拒绝，实际 %v", err)
	}
}
