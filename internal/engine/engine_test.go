package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"graycrow/internal/model"
	"graycrow/internal/scanner"
)

var engineDBCounter int64

func newEngineDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:engine%d?mode=memory&cache=shared", atomic.AddInt64(&engineDBCounter, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Rule{}, &model.Alert{}, &model.AlertEvent{}, &model.Allowlist{}, &model.ScanRun{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

func TestPersistDedupeAndAllowlist(t *testing.T) {
	db := newEngineDB(t)
	e := &Engine{db: db}
	rule := &model.Rule{Name: "acme", Keywords: []string{"acme.com"}, IntervalSec: 60, Severity: "high"}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Allowlist{Scope: "global", RepoFullName: "acme/internal"}).Error; err != nil {
		t.Fatal(err)
	}

	items := []scanner.CodeItem{
		{Repo: "acme/backend", Path: "config/app.yaml", HTMLURL: "https://github.com/acme/backend", Fragment: `password: "S3cr3t-Pa55w0rd"`},
		{Repo: "acme/backend", Path: "config/app.yaml", HTMLURL: "https://github.com/acme/backend", Fragment: `password: "S3cr3t-Pa55w0rd"`},
		{Repo: "acme/internal", Path: "deploy.js", Fragment: `const k = "AKIAJ7QK3M9PLZ2XV8NR"`},
		{Repo: "", Path: "skip", Fragment: "no repo"},
		{Repo: "acme/docs", Path: "readme.md", Fragment: "our domain is acme.com and nothing else"},
	}
	created, err := e.persist(rule, "acme.com", items)
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	if len(created) != 3 {
		t.Fatalf("同指纹应合并、无仓库应跳过，期望新增 3，实际 %d", len(created))
	}
	for i, alert := range created {
		if alert.ID == 0 || alert.RuleID != rule.ID {
			t.Fatalf("第 %d 条新告警字段异常: %+v", i, alert)
		}
	}

	alerts := []model.Alert{}
	if err := db.Order("id asc").Find(&alerts).Error; err != nil {
		t.Fatal(err)
	}
	if alerts[0].DetectorID != "generic-credential" {
		t.Fatalf("二次判定未生效: %+v", alerts[0])
	}
	if alerts[1].Status != model.AlertAllowlisted {
		t.Fatalf("白名单仓库应静默入库: %+v", alerts[1])
	}
	if alerts[1].DetectorID != "aws-access-key-id" {
		t.Fatalf("正则命中级别错误: %+v", alerts[1])
	}
	if alerts[2].DetectorID != "" {
		t.Fatalf("无二次判定应降级: %+v", alerts[2])
	}

	again, err := e.persist(rule, "acme.com", items)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("重复命中不应新建告警，实际新增 %d", len(again))
	}
	var count int64
	db.Model(&model.Alert{}).Count(&count)
	if count != 3 {
		t.Fatalf("告警总数应仍为 3，实际 %d", count)
	}
	reloaded := model.Alert{}
	db.Where("repo_full_name = ?", "acme/backend").First(&reloaded)
	if reloaded.LastSeenAt.Before(reloaded.FirstSeenAt) {
		t.Fatalf("重复命中应刷新 last_seen_at: %+v", reloaded)
	}
	var events int64
	db.Model(&model.AlertEvent{}).Count(&events)
	if events != 3 {
		t.Fatalf("每条新告警应有一条轨迹，实际 %d", events)
	}
}

func TestPersistContextOnlyWhenKeywordMissing(t *testing.T) {
	db := newEngineDB(t)
	e := &Engine{db: db}
	rule := &model.Rule{Name: "r", Keywords: []string{"acme.com"}, IntervalSec: 60}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
	items := []scanner.CodeItem{
		{Repo: "acme/x", Path: "env", Fragment: "FOO=Zq7Yb2NvLm4Xk9Rt6Wp1Hs8Ed3Cf5Jg0"},
	}
	if _, err := e.persist(rule, "acme.com", items); err != nil {
		t.Fatal(err)
	}
	alert := model.Alert{}
	if err := db.First(&alert).Error; err != nil {
		t.Fatal(err)
	}
	if alert.DetectorID != "high-entropy" {
		t.Fatalf("高熵串应先于 context-only 命中: %+v", alert)
	}

	db.Model(&model.Alert{}).Where("id = ?", alert.ID).Update("detector_id", "")
	if _, err := e.persist(rule, "acme.com", []scanner.CodeItem{
		{Repo: "acme/y", Path: "env", Fragment: "some unrelated line without the domain"},
	}); err != nil {
		t.Fatal(err)
	}
	contextOnly := model.Alert{}
	db.Where("repo_full_name = ?", "acme/y").First(&contextOnly)
	if contextOnly.DetectorID != "context-only" {
		t.Fatalf("关键词不在片段内应降级为 context-only: %+v", contextOnly)
	}
}

func TestNextScanAt(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) // 上海 08:00

	interval := &model.Rule{ScheduleType: "interval", IntervalSec: 60}
	if got := nextScanAt(interval, now, false, loc); !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("间隔调度下一次 %v", got)
	}
	if got := nextScanAt(interval, now, true, loc); !got.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("省额模式应翻倍，实际 %v", got)
	}

	cronRule := &model.Rule{ScheduleType: "cron", CronExpr: "0 9 * * *"}
	want := time.Date(2026, 10, 20, 9, 0, 0, 0, loc).UTC()
	if got := nextScanAt(cronRule, now, true, loc); !got.Equal(want) {
		t.Fatalf("cron 下一次应为 %v，实际 %v", want, got)
	}

	broken := &model.Rule{ScheduleType: "cron", CronExpr: "nope"}
	if got := nextScanAt(broken, now, false, loc); !got.Equal(now.Add(time.Hour)) {
		t.Fatalf("坏表达式应兜底 1 小时，实际 %v", got)
	}
}

func TestBucketClampsAndAccrues(t *testing.T) {
	b := newBucket(60)
	if b.capacity != 10 {
		t.Fatalf("GitHub code search 硬限 10/分钟，容量应被夹到 10，实际 %v", b.capacity)
	}
	b.tokens = 0
	b.last = time.Now().Add(-7 * time.Second)
	if !b.Wait(context.Background()) {
		t.Fatal("经过 7 秒应积累出 1 个令牌")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b.Wait(ctx) {
		t.Fatal("ctx 已结束时不应阻塞等待")
	}
}
