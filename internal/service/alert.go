package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
	"graycrow/internal/notify"
)

const (
	maxNotifyAttempts = 3
	maxBulkAlerts     = 200
)

var (
	ErrAlertNotFound = errors.New("告警不存在")
	ErrStatusInvalid = errors.New("告警状态不合法")
	ErrTooManyIDs    = errors.New("一次最多操作 200 条")
)

var alertStatuses = map[string]bool{
	model.AlertPending:       true,
	model.AlertResolved:      true,
	model.AlertFalsePositive: true,
	model.AlertIgnored:       true,
	model.AlertAllowlisted:   true,
}

type Skip struct {
	ID     uint   `json:"id"`
	Reason string `json:"reason"`
}

type AlertQuery struct {
	Status  string
	Repo    string
	Keyword string
	Q       string
	RuleID  *uint
	From    *time.Time
	To      *time.Time
	Page    int
	Size    int
}

type AlertService struct {
	db       *gorm.DB
	rules    *RuleService
	channels *ChannelService
}

func NewAlertService(db *gorm.DB, rules *RuleService, channels *ChannelService) *AlertService {
	return &AlertService{db: db, rules: rules, channels: channels}
}

func (s *AlertService) List(actor *model.User, q AlertQuery) ([]model.Alert, int64, error) {
	tx, err := s.scoped(actor)
	if err != nil {
		return nil, 0, err
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.Repo != "" {
		tx = tx.Where("repo_full_name LIKE ?", "%"+q.Repo+"%")
	}
	if q.Keyword != "" {
		tx = tx.Where("matched_keyword = ?", q.Keyword)
	}
	if q.RuleID != nil {
		tx = tx.Where("rule_id = ?", *q.RuleID)
	}
	if q.From != nil {
		tx = tx.Where("first_seen_at >= ?", *q.From)
	}
	if q.To != nil {
		tx = tx.Where("first_seen_at <= ?", *q.To)
	}
	if q.Q != "" {
		like := "%" + q.Q + "%"
		tx = tx.Where("repo_full_name LIKE ? OR path LIKE ? OR snippet LIKE ? OR detector_id LIKE ?", like, like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.Alert{}
	if err := tx.Order("id desc").Offset((q.Page - 1) * q.Size).Limit(q.Size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	s.fillRuleNames(rows)
	return rows, total, nil
}

func (s *AlertService) Get(actor *model.User, id uint) (*model.Alert, error) {
	alert, err := s.raw(actor, id)
	if err != nil {
		return nil, err
	}
	events := []model.AlertEvent{}
	if err := s.db.Where("alert_id = ?", id).Order("id asc").Find(&events).Error; err != nil {
		return nil, err
	}
	alert.Events = events
	alert.Allowed = s.isAllowed(alert, time.Now().UTC())
	if err := s.fillHandledBy(alert); err != nil {
		return nil, err
	}
	rows := []model.Alert{*alert}
	s.fillRuleNames(rows)
	alert.RuleName = rows[0].RuleName
	return alert, nil
}

func (s *AlertService) SetStatus(actor *model.User, id uint, status, note string) (*model.Alert, error) {
	if !alertStatuses[status] {
		return nil, ErrStatusInvalid
	}
	alert, err := s.raw(actor, id)
	if err != nil {
		return nil, err
	}
	if !canTransit(alert.Status, status, actor.IsAdmin()) {
		return nil, fmt.Errorf("%w：%s → %s 仅管理员可回退", ErrStatusInvalid, alert.Status, status)
	}
	if alert.Status != status {
		now := time.Now().UTC()
		updates := map[string]any{"status": status, "updated_at": now}
		if note != "" {
			updates["note"] = note
		}
		if status == model.AlertPending {
			updates["handled_by"] = nil
			updates["handled_at"] = nil
			updates["handle_seconds"] = 0
		} else {
			updates["handled_by"] = actor.ID
			updates["handled_at"] = now
			// 落库处置时长，避免统计时用各库不一致的时间差函数
			updates["handle_seconds"] = int(now.Sub(alert.FirstSeenAt).Seconds())
		}
		if err := s.db.Model(&model.Alert{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return nil, err
		}
		s.recordEvent(id, actor, "status_change", alert.Status, status, note)
	}
	return s.Get(actor, id)
}

func (s *AlertService) BulkStatus(actor *model.User, ids []uint, status, note string) ([]uint, []Skip, error) {
	if len(ids) > maxBulkAlerts {
		return nil, nil, ErrTooManyIDs
	}
	if !alertStatuses[status] {
		return nil, nil, ErrStatusInvalid
	}
	updated := []uint{}
	skipped := []Skip{}
	for _, id := range ids {
		if _, err := s.SetStatus(actor, id, status, note); err != nil {
			skipped = append(skipped, Skip{ID: id, Reason: err.Error()})
			continue
		}
		updated = append(updated, id)
	}
	return updated, skipped, nil
}

// DeleteAlerts 物理删除告警及其时间线（管理员手动整理历史用）。
func (s *AlertService) DeleteAlerts(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > maxBulkAlerts {
		return 0, ErrTooManyIDs
	}
	var deleted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("alert_id IN ?", ids).Delete(&model.AlertEvent{}).Error; err != nil {
			return err
		}
		res := tx.Where("id IN ?", ids).Delete(&model.Alert{})
		deleted = res.RowsAffected
		return res.Error
	})
	return deleted, err
}

// DeleteNotifyRecords 物理删除通知记录；告警删除不级联到这里，两类历史各管各的。
func (s *AlertService) DeleteNotifyRecords(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > maxBulkAlerts {
		return 0, ErrTooManyIDs
	}
	res := s.db.Where("id IN ?", ids).Delete(&model.NotifyRecord{})
	return res.RowsAffected, res.Error
}

// AllowlistAlert 一键加白：写白名单 + 把该告警置为 allowlisted。
func (s *AlertService) AllowlistAlert(actor *model.User, id uint, scope, reason string, expiresAt *time.Time) (*model.Alert, error) {
	alert, err := s.raw(actor, id)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "global"
	}
	if scope != "global" && scope != "rule" {
		return nil, ErrScopeInvalid
	}
	if reason == "" {
		reason = "人工加白"
	}
	var exists int64
	tx := s.db.Model(&model.Allowlist{}).Where("repo_full_name = ? AND scope = ?", alert.RepoFullName, scope)
	if scope == "rule" {
		tx = tx.Where("rule_id = ?", alert.RuleID)
	}
	if err := tx.Count(&exists).Error; err != nil {
		return nil, err
	}
	if exists == 0 {
		row := &model.Allowlist{
			Scope:         scope,
			RepoFullName:  alert.RepoFullName,
			Reason:        reason,
			ExpiresAt:     expiresAt,
			CreatedBy:     actor.ID,
			CreatedByName: actorDisplay(actor),
		}
		if scope == "rule" {
			ruleID := alert.RuleID
			row.RuleID = &ruleID
		}
		if err := s.db.Create(row).Error; err != nil {
			return nil, err
		}
	}
	if alert.Status != model.AlertAllowlisted {
		now := time.Now().UTC()
		err := s.db.Model(&model.Alert{}).Where("id = ?", id).
			Updates(map[string]any{
				"status": model.AlertAllowlisted, "handled_by": actor.ID, "handled_at": now,
				"handle_seconds": int(now.Sub(alert.FirstSeenAt).Seconds()), "updated_at": now,
			}).Error
		if err != nil {
			return nil, err
		}
		s.recordEvent(id, actor, "allowlist", alert.Status, model.AlertAllowlisted, reason)
	}
	return s.Get(actor, id)
}

type NotifyQuery struct {
	RuleID    *uint
	ChannelID *uint
	Status    string
	Page      int
	Size      int
}

// NotifyRecords 通知记录列表：普通用户只看得到自己可见规则的通知。
func (s *AlertService) NotifyRecords(actor *model.User, q NotifyQuery) ([]model.NotifyRecord, int64, error) {
	tx := s.db.Model(&model.NotifyRecord{})
	if !actor.IsAdmin() {
		ids, err := s.rules.VisibleRuleIDs(actor)
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			tx = tx.Where("1 = 0")
		} else {
			tx = tx.Where("rule_id IN ?", ids)
		}
	}
	if q.RuleID != nil {
		tx = tx.Where("rule_id = ?", *q.RuleID)
	}
	if q.ChannelID != nil {
		tx = tx.Where("channel_id = ?", *q.ChannelID)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.NotifyRecord{}
	if err := tx.Order("id desc").Offset((q.Page - 1) * q.Size).Limit(q.Size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	s.fillNotifyNames(rows)
	return rows, total, nil
}

// Renotify 手动重投：无视 dry_run 与告警状态，按规则绑定渠道重发单条详情并留下通知记录。
func (s *AlertService) Renotify(actor *model.User, id uint) (int, int, error) {
	alert, err := s.raw(actor, id)
	if err != nil {
		return 0, 0, err
	}
	rule := &model.Rule{ID: alert.RuleID}
	if err := s.db.First(rule, alert.RuleID).Error; err != nil {
		return 0, 0, ErrRuleNotFound
	}
	channels, err := s.channels.Enabled(s.ruleChannelIDs(rule.ID))
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, errors.New("没有可用的通知渠道")
	}
	payload := notify.Payload{
		Title:    rule.Name,
		Repo:     alert.RepoFullName,
		Path:     alert.Path,
		Detector: alert.DetectorID,
		Keyword:  alert.MatchedKeyword,
		URL:      alert.HTMLURL,
		Snippet:  alert.Snippet,
	}
	ok, failed := 0, 0
	for i := range channels {
		if s.deliver(&channels[i], notice{
			ruleID: rule.ID, alertID: &alert.ID, count: 1,
			text: payload.Text(), vars: payload.Vars(), alertIDs: []uint{alert.ID},
		}) {
			ok++
		} else {
			failed++
		}
	}
	return ok, failed, nil
}

// Dispatch 一条规则一次扫描结束后汇总投递：本次全部新命中合成一条消息发往每个绑定渠道。
// 干跑规则不通知；非 pending 告警（已加白等）不入汇总。
func (s *AlertService) Dispatch(rule *model.Rule, alerts []model.Alert) {
	if rule.DryRun {
		return
	}
	pending := make([]model.Alert, 0, len(alerts))
	for i := range alerts {
		if alerts[i].Status == model.AlertPending {
			pending = append(pending, alerts[i])
		}
	}
	if len(pending) == 0 {
		return
	}
	channels, err := s.channels.Enabled(s.ruleChannelIDs(rule.ID))
	if err != nil || len(channels) == 0 {
		return
	}
	batch := notify.Batch{Title: rule.Name}
	alertIDs := make([]uint, 0, len(pending))
	for i := range pending {
		batch.Items = append(batch.Items, notify.BatchItem{
			Repo:     pending[i].RepoFullName,
			Path:     pending[i].Path,
			Detector: pending[i].DetectorID,
			Keyword:  pending[i].MatchedKeyword,
			URL:      pending[i].HTMLURL,
		})
		alertIDs = append(alertIDs, pending[i].ID)
	}
	n := notice{ruleID: rule.ID, count: len(pending), text: batch.Text(), vars: batch.Vars(), alertIDs: alertIDs, digest: true}
	for i := range channels {
		s.deliver(&channels[i], n)
	}
}

// notice 一次待投递的通知：text 为消息正文，alertIDs 非空时投递成功后给这些告警打上 notified_at。
type notice struct {
	ruleID   uint
	alertID  *uint
	count    int
	text     string
	vars     map[string]string
	alertIDs []uint
	digest   bool
}

// deliver 单渠道投递：失败按 1s/2s/4s 退避重试（4xx 不重试），最终结果落一条通知记录。
func (s *AlertService) deliver(channel *model.Channel, n notice) bool {
	backoff := time.Second
	for attempt := 1; attempt <= maxNotifyAttempts; attempt++ {
		status, body, err := s.channels.Send(channel, n.text, n.vars, n.digest)
		ok := err == nil && status < 400
		if ok {
			s.saveNotifyRecord(channel, n, "success", status, attempt, "", true)
			return true
		}
		message := fmt.Sprintf("HTTP %d: %s", status, body)
		if err != nil {
			message = err.Error()
		}
		if attempt < maxNotifyAttempts && retryableNotify(err, status) {
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		s.saveNotifyRecord(channel, n, "failed", status, attempt, message, false)
		return false
	}
	return false
}

func (s *AlertService) saveNotifyRecord(channel *model.Channel, n notice, status string, httpStatus, attempt int, errText string, notified bool) {
	record := model.NotifyRecord{
		RuleID:     n.ruleID,
		AlertID:    n.alertID,
		ChannelID:  channel.ID,
		AlertCount: n.count,
		Status:     status,
		Attempt:    attempt,
		Error:      truncateText(errText, 500),
		Content:    truncateText(n.text, 4000),
		SentAt:     time.Now().UTC(),
	}
	if httpStatus > 0 {
		code := httpStatus
		record.HTTPStatus = &code
	}
	if err := s.db.Create(&record).Error; err != nil {
		return
	}
	if notified && len(n.alertIDs) > 0 {
		if err := s.db.Model(&model.Alert{}).Where("id IN ?", n.alertIDs).Update("notified_at", record.SentAt).Error; err != nil {
			return
		}
	}
}

func (s *AlertService) ruleChannelIDs(ruleID uint) []uint {
	ids := []uint{}
	if err := s.db.Model(&model.RuleChannel{}).Where("rule_id = ?", ruleID).Pluck("channel_id", &ids).Error; err != nil {
		return nil
	}
	return ids
}

func (s *AlertService) scoped(actor *model.User) (*gorm.DB, error) {
	tx := s.db.Model(&model.Alert{})
	if actor.IsAdmin() {
		return tx, nil
	}
	ids, err := s.rules.VisibleRuleIDs(actor)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return tx.Where("1 = 0"), nil
	}
	return tx.Where("rule_id IN ?", ids), nil
}

func (s *AlertService) raw(actor *model.User, id uint) (*model.Alert, error) {
	var alert model.Alert
	if err := s.db.First(&alert, id).Error; err != nil {
		return nil, ErrAlertNotFound
	}
	if !actor.IsAdmin() {
		ids, err := s.rules.VisibleRuleIDs(actor)
		if err != nil {
			return nil, err
		}
		inScope := false
		for _, ruleID := range ids {
			if ruleID == alert.RuleID {
				inScope = true
				break
			}
		}
		if !inScope {
			return nil, ErrOutOfScope
		}
	}
	return &alert, nil
}

func (s *AlertService) isAllowed(alert *model.Alert, now time.Time) bool {
	var count int64
	err := s.db.Model(&model.Allowlist{}).
		Where("repo_full_name = ? AND (scope = ? OR (scope = ? AND rule_id = ?)) AND (expires_at IS NULL OR expires_at > ?)",
			alert.RepoFullName, "global", "rule", alert.RuleID, now).Count(&count).Error
	return err == nil && count > 0
}

func (s *AlertService) fillRuleNames(rows []model.Alert) {
	ids := []uint{}
	for _, row := range rows {
		ids = append(ids, row.RuleID)
	}
	if len(ids) == 0 {
		return
	}
	names := map[uint]string{}
	var rules []model.Rule
	if err := s.db.Select("id", "name").Where("id IN ?", ids).Find(&rules).Error; err != nil {
		return
	}
	for _, rule := range rules {
		names[rule.ID] = rule.Name
	}
	for i := range rows {
		rows[i].RuleName = names[rows[i].RuleID]
	}
}

func (s *AlertService) fillNotifyNames(rows []model.NotifyRecord) {
	ruleIDs := []uint{}
	for _, row := range rows {
		ruleIDs = append(ruleIDs, row.RuleID)
	}
	if len(ruleIDs) == 0 {
		return
	}
	ruleNames := map[uint]string{}
	var rules []model.Rule
	if err := s.db.Select("id", "name").Where("id IN ?", ruleIDs).Find(&rules).Error; err == nil {
		for _, rule := range rules {
			ruleNames[rule.ID] = rule.Name
		}
	}
	channelNames := map[uint]string{}
	var channels []model.Channel
	if err := s.db.Select("id", "name").Find(&channels).Error; err == nil {
		for _, channel := range channels {
			channelNames[channel.ID] = channel.Name
		}
	}
	for i := range rows {
		rows[i].RuleName = ruleNames[rows[i].RuleID]
		rows[i].ChannelName = channelNames[rows[i].ChannelID]
	}
}

func (s *AlertService) fillHandledBy(alert *model.Alert) error {
	if alert.HandledBy == nil {
		return nil
	}
	var user model.User
	if err := s.db.Select("id", "username", "nickname").First(&user, *alert.HandledBy).Error; err != nil {
		return nil
	}
	alert.HandledByName = user.Display()
	return nil
}

func (s *AlertService) recordEvent(alertID uint, actor *model.User, action, from, to, note string) {
	event := model.AlertEvent{
		AlertID:    alertID,
		Action:     action,
		FromStatus: from,
		ToStatus:   to,
		Note:       truncateText(note, 500),
	}
	if actor != nil {
		event.ActorID = actor.ID
		event.ActorName = actorDisplay(actor)
	} else {
		event.ActorName = "system"
	}
	_ = s.db.Create(&event).Error
}

// canTransit 状态机：pending 可去任意终态；终态之间可人工纠正；回退到 pending 仅管理员。
func canTransit(from, to string, isAdmin bool) bool {
	if from == to {
		return true
	}
	if from == model.AlertPending {
		return true
	}
	if to == model.AlertPending {
		return isAdmin
	}
	return true
}

// retryableNotify 只重试临时故障：网络错误、5xx、429；4xx（如机器人被移除）重试无意义。
func retryableNotify(err error, status int) bool {
	if err != nil {
		return true
	}
	return status >= 500 || status == 429
}

func actorDisplay(actor *model.User) string {
	if actor == nil {
		return "system"
	}
	if actor.Nickname != "" {
		return actor.Nickname
	}
	return actor.Username
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n-1]) + "…"
}
