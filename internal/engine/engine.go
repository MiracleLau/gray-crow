package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
	"graycrow/internal/notify"
	"graycrow/internal/scanner"
	"graycrow/internal/schedule"
	"graycrow/internal/service"
)

const (
	tickInterval       = 5 * time.Second
	ruleTimeout        = 5 * time.Minute
	maxRequestsPerScan = 60
	maxWindowsPerQuery = 8
	tickBudget         = 50
	exhaustNoticeGap   = 30 * time.Minute
	maxExhaustSleep    = 300 * time.Second
)

type Engine struct {
	db       *gorm.DB
	pool     *service.KeyPool
	channels *service.ChannelService
	alerts   *service.AlertService
	audit    *service.Audit
	limiter  *bucket
	loc      *time.Location

	mu                sync.Mutex
	lastExhaustNotice time.Time
	lastInvalidNotice time.Time
}

func New(db *gorm.DB, pool *service.KeyPool, channels *service.ChannelService, alerts *service.AlertService, audit *service.Audit, perMin int, loc *time.Location) *Engine {
	if loc == nil {
		loc = time.UTC
	}
	return &Engine{db: db, pool: pool, channels: channels, alerts: alerts, audit: audit, limiter: newBucket(perMin), loc: loc}
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.dispatch(ctx)
		}
	}
}

func (e *Engine) dispatch(ctx context.Context) {
	rules := []model.Rule{}
	err := e.db.Where("enabled = ? AND next_scan_at <= ?", true, time.Now().UTC()).
		Order("next_scan_at asc").Limit(tickBudget).Find(&rules).Error
	if err != nil {
		log.Printf("调度器读取待扫描规则失败: %v", err)
		return
	}
	for i := range rules {
		if !e.limiter.Wait(ctx) {
			return
		}
		rule := &rules[i]
		scanErr := e.scan(ctx, rule)

		now := time.Now().UTC()
		next := nextScanAt(rule, now, e.pool.LowMode(), e.loc)
		if err := e.db.Model(&model.Rule{}).Where("id = ?", rule.ID).
			Updates(map[string]any{"last_scan_at": now, "next_scan_at": next}).Error; err != nil {
			log.Printf("更新规则 %d 扫描时间失败: %v", rule.ID, err)
		}

		var exhausted *service.PoolExhaustedError
		if errors.As(scanErr, &exhausted) {
			log.Printf("密钥池耗尽: %v", exhausted)
			// 先告警再睡：睡到额度重置可能长达 5 分钟，别让通知迟到
			e.notifyPoolExhausted(exhausted.ResetAt)
			e.sleepUntil(ctx, exhausted.ResetAt)
			return
		}
		if scanErr != nil {
			log.Printf("规则 %d 扫描失败: %v", rule.ID, scanErr)
		}
	}
}

// nextScanAt 按规则调度算下一次：cron 用配置时区，interval 在省额模式下翻倍。
func nextScanAt(rule *model.Rule, now time.Time, low bool, loc *time.Location) time.Time {
	spec, err := schedule.Parse(rule.ScheduleType, rule.CronExpr, rule.IntervalSec, loc)
	if err != nil {
		// 保存时已校验，这里只兜底脏数据，避免规则饿死
		log.Printf("规则 %d 调度表达式无效，按 1 小时兜底: %v", rule.ID, err)
		return now.Add(time.Hour)
	}
	if spec.IsCron() {
		return spec.Next(now)
	}
	interval := spec.Interval()
	if low {
		interval *= 2
	}
	return now.Add(interval)
}

func (e *Engine) scan(ctx context.Context, rule *model.Rule) error {
	ctx, cancel := context.WithTimeout(ctx, ruleTimeout)
	defer cancel()

	key, err := e.pool.Pick(time.Now().UTC())
	if err != nil {
		return err
	}
	token, err := e.pool.Token(key)
	if err != nil {
		return err
	}
	budget := maxRequestsPerScan
	fresh := []model.Alert{}
	for _, query := range scanner.BuildQueries(rule.Keywords, rule.QueryTemplate) {
		if budget <= 0 || ctx.Err() != nil {
			break
		}
		batch, err := e.runQuery(ctx, rule, key, token, query, &budget)
		fresh = append(fresh, batch...)
		if err != nil {
			// 已抓到的先投递，别让一次失败吞掉整批通知
			e.dispatchAlerts(rule, fresh)
			return err
		}
	}
	e.dispatchAlerts(rule, fresh)
	return nil
}

func (e *Engine) runQuery(ctx context.Context, rule *model.Rule, key *model.APIKey, token string, query scanner.Query, budget *int) ([]model.Alert, error) {
	got, err := e.collect(ctx, key, token, query.Text, budget)
	if err != nil {
		e.saveRun(rule, key, query.Text, model.RunFailed, 0, 0, got.cost, err.Error())
		return nil, err
	}
	fresh, err := e.persist(rule, query.Keyword, got.items)
	if err != nil {
		return nil, err
	}
	status := model.RunSuccess
	if got.total >= scanner.MaxResults {
		status = model.RunPartial
	}
	e.saveRun(rule, key, query.Text, status, got.itemCount, len(fresh), got.cost, "")
	if got.total < scanner.MaxResults {
		return fresh, nil
	}

	// ponytail: 结果触顶只做一层 30 天窗口回切（最多 8 个窗口）；真需要更细粒度再上二分
	to := time.Now().UTC()
	for i := 1; i <= maxWindowsPerQuery && *budget > 0; i++ {
		from := to.Add(-scanner.WindowSize)
		text := scanner.Window(query.Text, from, to)
		got, err := e.collect(ctx, key, token, text, budget)
		if err != nil {
			e.saveRun(rule, key, text, model.RunFailed, 0, 0, got.cost, err.Error())
			return fresh, err
		}
		windowFresh, err := e.persist(rule, query.Keyword, got.items)
		if err != nil {
			return fresh, err
		}
		fresh = append(fresh, windowFresh...)
		windowStatus := model.RunSuccess
		if got.total >= scanner.MaxResults {
			windowStatus = model.RunPartial
		}
		e.saveRun(rule, key, text, windowStatus, got.itemCount, len(windowFresh), got.cost, "")
		to = from
		if got.total < scanner.MaxResults {
			break
		}
	}
	return fresh, nil
}

// fetch 一次查询的抓取结果：items 是命中片段，itemCount 是上游条目数，total 是上游总数。
type fetch struct {
	items     []scanner.CodeItem
	itemCount int
	total     int
	cost      int
}

func (e *Engine) collect(ctx context.Context, key *model.APIKey, token, text string, budget *int) (fetch, error) {
	got := fetch{items: []scanner.CodeItem{}}
	for page := 1; page <= scanner.MaxPages; page++ {
		if *budget <= 0 {
			return got, nil
		}
		result, apiErr := scanner.SearchCode(ctx, token, text, page)
		*budget = *budget - 1
		got.cost++
		if apiErr != nil {
			e.handleAPIError(key, apiErr)
			return got, errors.New(apiErr.Message)
		}
		e.pool.Observe(key, result.Quota)
		got.total = result.Total
		got.itemCount += result.ItemCount
		got.items = append(got.items, result.Items...)
		if result.ItemCount == 0 || result.ItemCount < scanner.PageSize || page*scanner.PageSize >= result.Total {
			break
		}
	}
	return got, nil
}

func (e *Engine) handleAPIError(key *model.APIKey, apiErr *scanner.APIError) {
	switch apiErr.Kind {
	case scanner.ErrKindUnauthorized:
		e.pool.MarkInvalid(key, apiErr.Message)
		e.notifyKeyInvalid(key, apiErr.Message)
	case scanner.ErrKindRateLimit:
		e.pool.Cooling(key, time.Now().UTC().Add(apiErr.RetryAfter), apiErr.Message)
	case scanner.ErrKindQuery:
		// 查询语法被上游拒绝：记账到规则（scan_runs 已落库），不把健康密钥拖进冷却
	default:
		e.pool.Failure(key, apiErr)
	}
}

// persist 二次判定 + 指纹去重入库；同一指纹只刷 last_seen_at，不重复告警，返回本次新增的告警。
func (e *Engine) persist(rule *model.Rule, keyword string, items []scanner.CodeItem) ([]model.Alert, error) {
	if len(items) == 0 {
		return nil, nil
	}
	allow := e.allowSet(rule.ID)
	now := time.Now().UTC()
	candidates := []model.Alert{}
	seen := map[string]bool{}
	for _, item := range items {
		if item.Repo == "" || item.Fragment == "" {
			continue
		}
		verdict := scanner.Detect(item.Fragment)
		detector := verdict.DetectorID
		if detector == "" && !strings.Contains(item.Fragment, keyword) {
			detector = "context-only"
		}
		fingerprint := fingerprint(rule.ID, keyword, item, detector, verdict.Snippet)
		if seen[fingerprint] {
			continue
		}
		seen[fingerprint] = true
		status := model.AlertPending
		if allow[item.Repo] {
			status = model.AlertAllowlisted
		}
		candidates = append(candidates, model.Alert{
			RuleID:         rule.ID,
			RepoFullName:   item.Repo,
			RepoURL:        item.RepoURL,
			HTMLURL:        item.HTMLURL,
			Path:           item.Path,
			MatchedKeyword: keyword,
			DetectorID:     detector,
			Snippet:        verdict.Snippet,
			Status:         status,
			Fingerprint:    fingerprint,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		})
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	fingerprints := make([]string, 0, len(candidates))
	for _, alert := range candidates {
		fingerprints = append(fingerprints, alert.Fingerprint)
	}
	existing := []string{}
	if err := e.db.Model(&model.Alert{}).Where("fingerprint IN ?", fingerprints).Pluck("fingerprint", &existing).Error; err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		err := e.db.Model(&model.Alert{}).Where("fingerprint IN ?", existing).
			Update("last_seen_at", now).Error
		if err != nil {
			return nil, err
		}
	}

	known := map[string]bool{}
	for _, fp := range existing {
		known[fp] = true
	}
	fresh := []model.Alert{}
	for _, alert := range candidates {
		if !known[alert.Fingerprint] {
			fresh = append(fresh, alert)
		}
	}
	if len(fresh) == 0 {
		return nil, nil
	}
	if err := e.db.Create(&fresh).Error; err != nil {
		return nil, err
	}
	events := make([]model.AlertEvent, 0, len(fresh))
	for _, alert := range fresh {
		events = append(events, model.AlertEvent{
			AlertID:   alert.ID,
			ActorName: "system",
			Action:    "create",
			ToStatus:  alert.Status,
		})
	}
	if err := e.db.Create(&events).Error; err != nil {
		log.Printf("写入告警轨迹失败: %v", err)
	}
	return fresh, nil
}

// dispatchAlerts 异步投递汇总通知：单渠道最坏 3 次重试 30s，不能拖住扫描节奏。
func (e *Engine) dispatchAlerts(rule *model.Rule, alerts []model.Alert) {
	if len(alerts) == 0 || e.alerts == nil {
		return
	}
	go e.alerts.Dispatch(rule, alerts)
}

func (e *Engine) allowSet(ruleID uint) map[string]bool {
	rows := []model.Allowlist{}
	err := e.db.Where("(scope = ? OR (scope = ? AND rule_id = ?)) AND (expires_at IS NULL OR expires_at > ?)",
		"global", "rule", ruleID, time.Now().UTC()).Find(&rows).Error
	if err != nil {
		log.Printf("读取白名单失败: %v", err)
		return map[string]bool{}
	}
	set := make(map[string]bool, len(rows))
	for _, row := range rows {
		set[row.RepoFullName] = true
	}
	return set
}

func (e *Engine) saveRun(rule *model.Rule, key *model.APIKey, query, status string, results, newAlerts, cost int, errMsg string) {
	now := time.Now().UTC()
	run := model.ScanRun{
		RuleID:              rule.ID,
		APIKeyID:            key.ID,
		Query:               truncate(query, 500),
		Status:              status,
		ResultCount:         results,
		NewAlertCount:       newAlerts,
		APICost:             cost,
		QuotaRemainingAfter: key.QuotaRemaining,
		Error:               truncate(errMsg, 500),
		StartedAt:           now,
		FinishedAt:          &now,
	}
	if err := e.db.Create(&run).Error; err != nil {
		log.Printf("写入扫描日志失败: %v", err)
	}
}

func (e *Engine) Runs(ruleID uint, page, size int) ([]model.ScanRun, int64, error) {
	var total int64
	if err := e.db.Model(&model.ScanRun{}).Where("rule_id = ?", ruleID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	runs := []model.ScanRun{}
	err := e.db.Where("rule_id = ?", ruleID).Order("id desc").Offset((page - 1) * size).Limit(size).Find(&runs).Error
	if err != nil || len(runs) == 0 {
		return runs, total, err
	}
	keyIDs := make([]uint, 0, len(runs))
	for _, run := range runs {
		keyIDs = append(keyIDs, run.APIKeyID)
	}
	names := map[uint]string{}
	var keys []model.APIKey
	if err := e.db.Select("id", "name").Where("id IN ?", keyIDs).Find(&keys).Error; err == nil {
		for _, key := range keys {
			names[key.ID] = key.Name
		}
	}
	for i := range runs {
		runs[i].KeyName = names[runs[i].APIKeyID]
	}
	return runs, total, nil
}

// notifyKeyInvalid 密钥失效时立刻通知管理员；同进程内 30 分钟最多一次。
func (e *Engine) notifyKeyInvalid(key *model.APIKey, reason string) {
	e.mu.Lock()
	if time.Since(e.lastInvalidNotice) < exhaustNoticeGap {
		e.mu.Unlock()
		return
	}
	e.lastInvalidNotice = time.Now()
	e.mu.Unlock()

	detail := fmt.Sprintf("密钥「%s」已标记失效：%s", key.Name, reason)
	e.audit.Record(nil, "apikey.invalid", fmt.Sprintf("key#%d", key.ID), detail, "")
	e.channels.Broadcast(notify.Payload{
		Title:    "密钥失效",
		Repo:     "-",
		Detector: "apikey.invalid",
		Snippet:  detail,
	}, nil)
}

func (e *Engine) notifyPoolExhausted(resetAt time.Time) {
	e.mu.Lock()
	if time.Since(e.lastExhaustNotice) < exhaustNoticeGap {
		e.mu.Unlock()
		return
	}
	e.lastExhaustNotice = time.Now()
	e.mu.Unlock()

	detail := "没有可用密钥，请检查密钥池状态"
	if !resetAt.IsZero() {
		detail = fmt.Sprintf("额度窗口将于 %s 重置；期间暂停扫描", resetAt.Local().Format("15:04:05"))
	}
	e.audit.Record(nil, "pool.exhausted", "pool", detail, "")
	e.channels.Broadcast(notify.Payload{
		Title:    "密钥池耗尽",
		Repo:     "-",
		Detector: "pool.exhausted",
		Snippet:  detail,
	}, nil)
}

func (e *Engine) sleepUntil(ctx context.Context, until time.Time) {
	wait := time.Minute
	if !until.IsZero() {
		if d := time.Until(until); d > 0 {
			wait = d
		}
	}
	if wait > maxExhaustSleep {
		wait = maxExhaustSleep
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func fingerprint(ruleID uint, keyword string, item scanner.CodeItem, detector, snippet string) string {
	raw := fmt.Sprintf("%d|%s|%s|%s|%s|%s", ruleID, item.Repo, item.Path, keyword, detector, normalize(snippet))
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', '"', '\'', '`':
			return -1
		}
		return r
	}, strings.ToLower(s))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// bucket 全局令牌桶：GitHub code search 按账号 10 次/分钟，硬限就是 10。
type bucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	ratePerSec float64
	last       time.Time
}

func newBucket(perMin int) *bucket {
	if perMin < 1 {
		perMin = 1
	}
	if perMin > 10 {
		perMin = 10
	}
	return &bucket{tokens: 1, capacity: float64(perMin), ratePerSec: float64(perMin) / 60, last: time.Now()}
}

func (b *bucket) Wait(ctx context.Context) bool {
	for {
		b.mu.Lock()
		now := time.Now()
		b.tokens = math.Min(b.capacity, b.tokens+now.Sub(b.last).Seconds()*b.ratePerSec)
		b.last = now
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return true
		}
		wait := time.Duration((1 - b.tokens) / b.ratePerSec * float64(time.Second))
		b.mu.Unlock()
		timer := time.NewTimer(wait + 50*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}
