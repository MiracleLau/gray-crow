package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
	"graycrow/internal/schedule"
)

var (
	ErrRuleNotFound    = errors.New("规则不存在")
	ErrRuleName        = errors.New("请填写规则名称")
	ErrKeywords        = errors.New("至少填写一个关键词")
	ErrQuerySyntax     = errors.New("关键词与查询模板不能包含括号 ( )，上游代码搜索会直接拒绝该查询")
	ErrIntervalTooLow  = errors.New("扫描间隔不得低于下限")
	ErrScheduleInvalid = errors.New("调度表达式不合法")
	ErrRuleDisabled    = errors.New("规则已禁用,请先启用再扫描")
	ErrOutOfScope      = errors.New("无权访问该规则")
)

type RuleDTO struct {
	model.Rule
	ViewerIDs  []uint `json:"viewer_ids"`
	ChannelIDs []uint `json:"channel_ids"`
	AlertCount int    `json:"alert_count"`
}

type RuleInput struct {
	Name          string
	Description   string
	Keywords      []string
	QueryTemplate string
	ScheduleType  string
	CronExpr      string
	IntervalSec   int
	Severity      string
	Enabled       bool
	DryRun        bool
	RunNow        *bool // 仅创建时生效：false 表示等到第一次调度再扫
	ViewerIDs     []uint
	ChannelIDs    []uint
}

type RuleService struct {
	db          *gorm.DB
	minInterval int
	loc         *time.Location
}

func NewRuleService(db *gorm.DB, minInterval int, loc *time.Location) *RuleService {
	if minInterval < 60 {
		minInterval = 60
	}
	if loc == nil {
		loc = time.UTC
	}
	return &RuleService{db: db, minInterval: minInterval, loc: loc}
}

// Location 供 API 预览 cron 时复用同一套时区解释。
func (s *RuleService) Location() *time.Location { return s.loc }

// MinInterval 供 API 预览时复用同一套最小间隔校验。
func (s *RuleService) MinInterval() int { return s.minInterval }

func (s *RuleService) List(actor *model.User) ([]RuleDTO, error) {
	rules := []model.Rule{}
	tx := s.db.Order("id desc")
	if !actor.IsAdmin() {
		tx = tx.Where("id IN (?)", s.db.Model(&model.RuleViewer{}).Select("rule_id").Where("user_id = ?", actor.ID))
	}
	if err := tx.Find(&rules).Error; err != nil {
		return nil, err
	}
	return s.decorate(rules)
}

func (s *RuleService) Get(id uint, actor *model.User) (*RuleDTO, error) {
	var rule model.Rule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, ErrRuleNotFound
	}
	if !actor.IsAdmin() {
		var count int64
		s.db.Model(&model.RuleViewer{}).Where("rule_id = ? AND user_id = ?", id, actor.ID).Count(&count)
		if count == 0 {
			return nil, ErrOutOfScope
		}
	}
	list, err := s.decorate([]model.Rule{rule})
	if err != nil {
		return nil, err
	}
	return &list[0], nil
}

func (s *RuleService) VisibleRuleIDs(actor *model.User) ([]uint, error) {
	if actor.IsAdmin() {
		var ids []uint
		err := s.db.Model(&model.Rule{}).Pluck("id", &ids).Error
		return ids, err
	}
	ids := []uint{}
	err := s.db.Model(&model.RuleViewer{}).Where("user_id = ?", actor.ID).Pluck("rule_id", &ids).Error
	return ids, err
}

func (s *RuleService) Create(actor *model.User, in RuleInput) (*RuleDTO, error) {
	if err := s.validate(&in); err != nil {
		return nil, err
	}
	rule := &model.Rule{
		Name:          strings.TrimSpace(in.Name),
		Description:   in.Description,
		Keywords:      normalizeKeywords(in.Keywords),
		QueryTemplate: in.QueryTemplate,
		ScheduleType:  in.ScheduleType,
		CronExpr:      in.CronExpr,
		IntervalSec:   in.IntervalSec,
		Severity:      normalizeSeverity(in.Severity),
		Enabled:       in.Enabled,
		DryRun:        in.DryRun,
		NextScanAt:    s.firstRun(&in),
		CreatedBy:     actor.ID,
	}
	if err := s.db.Create(rule).Error; err != nil {
		return nil, err
	}
	if err := s.SetViewers(rule.ID, in.ViewerIDs); err != nil {
		return nil, err
	}
	if err := s.SetChannels(rule.ID, in.ChannelIDs); err != nil {
		return nil, err
	}
	return s.Get(rule.ID, actor)
}

func (s *RuleService) Update(id uint, in RuleInput) (*RuleDTO, error) {
	old, err := s.raw(id)
	if err != nil {
		return nil, err
	}
	if err := s.validate(&in); err != nil {
		return nil, err
	}
	// 用结构体 + Select 更新：map 更新会绕过 serializer:json，把 []string 直接拼进 SQL
	err = s.db.Model(&model.Rule{}).Where("id = ?", id).
		Select("name", "description", "keywords", "query_template", "schedule_type", "cron_expr", "interval_sec", "severity", "enabled", "dry_run").
		Updates(model.Rule{
			Name:          strings.TrimSpace(in.Name),
			Description:   in.Description,
			Keywords:      normalizeKeywords(in.Keywords),
			QueryTemplate: in.QueryTemplate,
			ScheduleType:  in.ScheduleType,
			CronExpr:      in.CronExpr,
			IntervalSec:   in.IntervalSec,
			Severity:      normalizeSeverity(in.Severity),
			Enabled:       in.Enabled,
			DryRun:        in.DryRun,
		}).Error
	if err != nil {
		return nil, err
	}
	// 只有调度变了才重算下一跑，避免改个名字把「立即扫描」挤掉
	// 从禁用恢复启用也要重算,否则禁用期攒下的过期 next_scan_at 会被补跑一次
	oldKind, _ := scheduleKind(old.ScheduleType)
	if oldKind != in.ScheduleType || strings.TrimSpace(old.CronExpr) != in.CronExpr || old.IntervalSec != in.IntervalSec || (!old.Enabled && in.Enabled) {
		if err := s.db.Model(&model.Rule{}).Where("id = ?", id).
			Update("next_scan_at", s.nextAt(in.ScheduleType, in.CronExpr, in.IntervalSec)).Error; err != nil {
			return nil, err
		}
	}
	if err := s.SetViewers(id, in.ViewerIDs); err != nil {
		return nil, err
	}
	if err := s.SetChannels(id, in.ChannelIDs); err != nil {
		return nil, err
	}
	var admin model.User
	admin.Role = model.RoleAdmin
	return s.Get(id, &admin)
}

func (s *RuleService) Delete(id uint) error {
	if _, err := s.raw(id); err != nil {
		return err
	}
	if err := s.db.Where("rule_id = ?", id).Delete(&model.RuleViewer{}).Error; err != nil {
		return err
	}
	if err := s.db.Where("rule_id = ?", id).Delete(&model.RuleChannel{}).Error; err != nil {
		return err
	}
	return s.db.Delete(&model.Rule{}, id).Error
}

// TriggerScan 只把 next_scan_at 拉到现在，实际扫描由 P4 的调度器消费。
// 禁用规则不参与调度,写进去也没人消费,还会在下次启用时被补跑,所以直接拒绝。
func (s *RuleService) TriggerScan(id uint) error {
	rule, err := s.raw(id)
	if err != nil {
		return err
	}
	if !rule.Enabled {
		return ErrRuleDisabled
	}
	return s.db.Model(&model.Rule{}).Where("id = ?", id).Update("next_scan_at", time.Now().UTC()).Error
}

func (s *RuleService) SetViewers(ruleID uint, userIDs []uint) error {
	if err := s.db.Where("rule_id = ?", ruleID).Delete(&model.RuleViewer{}).Error; err != nil {
		return err
	}
	for _, uid := range unique(userIDs) {
		if uid == 0 {
			continue
		}
		if err := s.db.Create(&model.RuleViewer{RuleID: ruleID, UserID: uid}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *RuleService) SetChannels(ruleID uint, channelIDs []uint) error {
	if err := s.db.Where("rule_id = ?", ruleID).Delete(&model.RuleChannel{}).Error; err != nil {
		return err
	}
	for _, cid := range unique(channelIDs) {
		if cid == 0 {
			continue
		}
		if err := s.db.Create(&model.RuleChannel{RuleID: ruleID, ChannelID: cid}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *RuleService) validate(in *RuleInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return ErrRuleName
	}
	if len(normalizeKeywords(in.Keywords)) == 0 {
		return ErrKeywords
	}
	if err := validateQuerySyntax(in.Keywords, in.QueryTemplate); err != nil {
		return err
	}
	kind, err := scheduleKind(in.ScheduleType)
	if err != nil {
		return err
	}
	in.ScheduleType = kind
	in.CronExpr = strings.TrimSpace(in.CronExpr)
	if kind == schedule.TypeInterval {
		if in.IntervalSec < s.minInterval {
			return ErrIntervalTooLow
		}
		return nil
	}
	spec, err := schedule.Parse(kind, in.CronExpr, in.IntervalSec, s.loc)
	if err != nil {
		return fmt.Errorf("%w：%s", ErrScheduleInvalid, err)
	}
	if err := spec.Validate(time.Now().UTC(), time.Duration(s.minInterval)*time.Second); err != nil {
		return fmt.Errorf("%w：%s", ErrScheduleInvalid, err)
	}
	return nil
}

// validateQuerySyntax 上游 /search/code 对含括号的查询直接 422，保存时就拦住而不是等扫描失败。
func validateQuerySyntax(keywords []string, template string) error {
	for _, s := range append([]string{template}, keywords...) {
		if strings.ContainsAny(s, "()") {
			return ErrQuerySyntax
		}
	}
	return nil
}

func scheduleKind(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", schedule.TypeInterval:
		return schedule.TypeInterval, nil
	case schedule.TypeCron:
		return schedule.TypeCron, nil
	}
	return "", fmt.Errorf("%w：未知调度方式 %q", ErrScheduleInvalid, raw)
}

// firstRun 决定新建规则的首次扫描：默认立即扫一次，取消勾选则等第一次调度。
func (s *RuleService) firstRun(in *RuleInput) time.Time {
	if in.RunNow == nil || *in.RunNow {
		return time.Now().UTC()
	}
	return s.nextAt(in.ScheduleType, in.CronExpr, in.IntervalSec)
}

func (s *RuleService) nextAt(kind, expr string, intervalSec int) time.Time {
	now := time.Now().UTC()
	spec, err := schedule.Parse(kind, expr, intervalSec, s.loc)
	if err != nil {
		return now.Add(time.Hour) // validate 已拦住，这里只兜底
	}
	return spec.Next(now)
}

func (s *RuleService) raw(id uint) (*model.Rule, error) {
	var rule model.Rule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, ErrRuleNotFound
	}
	return &rule, nil
}

func (s *RuleService) decorate(rules []model.Rule) ([]RuleDTO, error) {
	if len(rules) == 0 {
		return []RuleDTO{}, nil
	}
	ids := make([]uint, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.ID)
	}

	viewers := map[uint][]uint{}
	channels := map[uint][]uint{}
	var viewerRows []model.RuleViewer
	if err := s.db.Where("rule_id IN ?", ids).Find(&viewerRows).Error; err != nil {
		return nil, err
	}
	for _, row := range viewerRows {
		viewers[row.RuleID] = append(viewers[row.RuleID], row.UserID)
	}
	var channelRows []model.RuleChannel
	if err := s.db.Where("rule_id IN ?", ids).Find(&channelRows).Error; err != nil {
		return nil, err
	}
	for _, row := range channelRows {
		channels[row.RuleID] = append(channels[row.RuleID], row.ChannelID)
	}

	counts := map[uint]int{}
	var countRows []struct {
		RuleID uint
		Total  int
	}
	if err := s.db.Model(&model.Alert{}).Select("rule_id, COUNT(*) as total").
		Where("rule_id IN ?", ids).Group("rule_id").Scan(&countRows).Error; err != nil {
		return nil, err
	}
	for _, row := range countRows {
		counts[row.RuleID] = row.Total
	}

	out := make([]RuleDTO, 0, len(rules))
	for _, r := range rules {
		out = append(out, RuleDTO{
			Rule:       r,
			ViewerIDs:  orEmpty(viewers[r.ID]),
			ChannelIDs: orEmpty(channels[r.ID]),
			AlertCount: counts[r.ID],
		})
	}
	return out, nil
}

func normalizeKeywords(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, kw := range in {
		kw = strings.TrimSpace(kw)
		if kw == "" || seen[kw] {
			continue
		}
		seen[kw] = true
		out = append(out, kw)
	}
	return out
}

func normalizeSeverity(s string) string {
	switch s {
	case "high", "medium", "low":
		return s
	}
	return "high"
}

func unique(in []uint) []uint {
	out := []uint{}
	seen := map[uint]bool{}
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func orEmpty(in []uint) []uint {
	if in == nil {
		return []uint{}
	}
	return in
}
