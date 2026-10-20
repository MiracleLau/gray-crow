package service

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
)

type Overview struct {
	TodayNew        int64   `json:"today_new"`
	Pending         int64   `json:"pending"`
	FalsePositive   float64 `json:"false_positive_rate"`
	ResolvedRate    float64 `json:"resolved_rate"`
	AvgHandleMinute float64 `json:"avg_handle_minutes"`
	PoolRemaining   int     `json:"pool_remaining"`
	PoolLimit       int     `json:"pool_limit"`
	PoolAccounts    int     `json:"pool_accounts"`
}

type TrendPoint struct {
	Date          string `json:"date"`
	New           int64  `json:"new"`
	Resolved      int64  `json:"resolved"`
	FalsePositive int64  `json:"false_positive"`
}

type RuleStat struct {
	RuleID   uint   `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Total    int64  `json:"total"`
	Pending  int64  `json:"pending"`
}

type StatusStat struct {
	Status string `json:"status"`
	Total  int64  `json:"total"`
}

type QuotaPoint struct {
	Date      string `json:"date"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"`
}

type StatsService struct {
	db    *gorm.DB
	rules *RuleService
}

func NewStatsService(db *gorm.DB, rules *RuleService) *StatsService {
	return &StatsService{db: db, rules: rules}
}

func (s *StatsService) Overview(actor *model.User) (*Overview, error) {
	tx, err := s.scoped(actor)
	if err != nil {
		return nil, err
	}
	out := &Overview{}
	dayStart := time.Now().UTC().Truncate(24 * time.Hour)
	if err := tx.Where("first_seen_at >= ?", dayStart).Count(&out.TodayNew).Error; err != nil {
		return nil, err
	}
	statusCounts, err := s.byStatusTx(tx)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, row := range statusCounts {
		total += row.Total
	}
	out.Pending = countOf(statusCounts, model.AlertPending)
	if total > 0 {
		out.FalsePositive = round2(float64(countOf(statusCounts, model.AlertFalsePositive)) / float64(total) * 100)
		out.ResolvedRate = round2(float64(countOf(statusCounts, model.AlertResolved)) / float64(total) * 100)
	}
	var avgSeconds float64
	err = tx.Model(&model.Alert{}).Where("handle_seconds > 0").Select("COALESCE(AVG(handle_seconds), 0)").Scan(&avgSeconds).Error
	if err != nil {
		return nil, err
	}
	out.AvgHandleMinute = round2(avgSeconds / 60)
	return out, nil
}

// Trend 按天聚合；缺口补零，保证折线连续。
func (s *StatsService) Trend(actor *model.User, rangeStr string) ([]TrendPoint, error) {
	tx, err := s.scoped(actor)
	if err != nil {
		return nil, err
	}
	since := sinceOf(rangeStr, 30)
	type row struct {
		Day   string
		Total int64
	}
	newRows := []row{}
	err = tx.Model(&model.Alert{}).Where("first_seen_at >= ?", since).
		Select(s.dateExpr("first_seen_at") + " AS day, COUNT(*) AS total").Group("day").Scan(&newRows).Error
	if err != nil {
		return nil, err
	}

	handledRows := []struct {
		Day    string
		Status string
		Total  int64
	}{}
	err = tx.Model(&model.Alert{}).Where("handled_at >= ? AND status IN ?", since, []string{model.AlertResolved, model.AlertFalsePositive}).
		Select(s.dateExpr("handled_at") + " AS day, status, COUNT(*) AS total").Group("day, status").Scan(&handledRows).Error
	if err != nil {
		return nil, err
	}

	points := map[string]*TrendPoint{}
	for day := since.Truncate(24 * time.Hour); !day.After(time.Now().UTC()); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		points[key] = &TrendPoint{Date: key}
	}
	for _, row := range newRows {
		if point, ok := points[normalizeDay(row.Day)]; ok {
			point.New = row.Total
		}
	}
	for _, row := range handledRows {
		point, ok := points[normalizeDay(row.Day)]
		if !ok {
			continue
		}
		if row.Status == model.AlertResolved {
			point.Resolved = row.Total
		} else {
			point.FalsePositive = row.Total
		}
	}
	out := make([]TrendPoint, 0, len(points))
	for day := since.Truncate(24 * time.Hour); !day.After(time.Now().UTC()); day = day.AddDate(0, 0, 1) {
		out = append(out, *points[day.Format("2006-01-02")])
	}
	return out, nil
}

func (s *StatsService) ByRule(actor *model.User) ([]RuleStat, error) {
	tx, err := s.scoped(actor)
	if err != nil {
		return nil, err
	}
	rows := []struct {
		RuleID  uint
		Total   int64
		Pending int64
	}{}
	err = tx.Model(&model.Alert{}).
		Select("rule_id, COUNT(*) AS total, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS pending", model.AlertPending).
		Group("rule_id").Order("total DESC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	names := map[uint]string{}
	var rules []model.Rule
	if err := s.db.Select("id", "name").Find(&rules).Error; err == nil {
		for _, rule := range rules {
			names[rule.ID] = rule.Name
		}
	}
	out := make([]RuleStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, RuleStat{RuleID: row.RuleID, RuleName: names[row.RuleID], Total: row.Total, Pending: row.Pending})
	}
	return out, nil
}

func (s *StatsService) ByStatus(actor *model.User) ([]StatusStat, error) {
	tx, err := s.scoped(actor)
	if err != nil {
		return nil, err
	}
	return s.byStatusTx(tx)
}

// QuotaTrend 从 scan_runs 反推额度曲线：used 为当日请求数，remaining 为当日平均余量。
func (s *StatsService) QuotaTrend(rangeStr string) ([]QuotaPoint, error) {
	since := sinceOf(rangeStr, 7)
	rows := []struct {
		Day       string
		Used      int64
		Remaining float64
	}{}
	err := s.db.Model(&model.ScanRun{}).Where("started_at >= ?", since).
		Select(s.dateExpr("started_at") + " AS day, SUM(api_cost) AS used, AVG(quota_remaining_after) AS remaining").
		Group("day").Order("day asc").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]QuotaPoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, QuotaPoint{Date: normalizeDay(row.Day), Used: row.Used, Remaining: int64(row.Remaining + 0.5)})
	}
	return out, nil
}

// ExportAlerts 流式导出 CSV；viewer 只导出自己可见范围的告警。
func (s *StatsService) ExportAlerts(actor *model.User, q AlertQuery, w io.Writer) error {
	tx, err := s.scoped(actor)
	if err != nil {
		return err
	}
	tx = applyAlertFilters(tx, q)
	rows := []model.Alert{}
	if err := tx.Order("id desc").Limit(50000).Find(&rows).Error; err != nil {
		return err
	}
	names := map[uint]string{}
	var rules []model.Rule
	if err := s.db.Select("id", "name").Find(&rules).Error; err == nil {
		for _, rule := range rules {
			names[rule.ID] = rule.Name
		}
	}

	writer := csv.NewWriter(w)
	// BOM：Excel 直接打开不乱码
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return err
	}
	header := []string{"ID", "状态", "仓库", "文件", "命中关键词", "二次判定", "规则", "首次发现", "最近命中", "处置人", "处置时间", "备注", "片段", "文件链接"}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, alert := range rows {
		record := []string{
			strconv.Itoa(int(alert.ID)),
			alert.Status,
			alert.RepoFullName,
			alert.Path,
			alert.MatchedKeyword,
			alert.DetectorID,
			names[alert.RuleID],
			alert.FirstSeenAt.Format(time.RFC3339),
			alert.LastSeenAt.Format(time.RFC3339),
			alert.HandledByName,
			formatTime(alert.HandledAt),
			alert.Note,
			strings.ReplaceAll(alert.Snippet, "\n", " "),
			alert.HTMLURL,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func (s *StatsService) scoped(actor *model.User) (*gorm.DB, error) {
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

func (s *StatsService) byStatusTx(tx *gorm.DB) ([]StatusStat, error) {
	rows := []StatusStat{}
	err := tx.Select("status, COUNT(*) AS total").Group("status").Scan(&rows).Error
	return rows, err
}

// dateExpr 三库统一取 YYYY-MM-DD：MySQL 的 CAST 只认 CHAR，sqlite/pg 用 TEXT。
func (s *StatsService) dateExpr(column string) string {
	if s.db.Dialector.Name() == "mysql" {
		return fmt.Sprintf("CAST(DATE(%s) AS CHAR)", column)
	}
	return fmt.Sprintf("CAST(DATE(%s) AS TEXT)", column)
}

func applyAlertFilters(tx *gorm.DB, q AlertQuery) *gorm.DB {
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
	return tx
}

func countOf(rows []StatusStat, status string) int64 {
	for _, row := range rows {
		if row.Status == status {
			return row.Total
		}
	}
	return 0
}

func sinceOf(rangeStr string, defaultDays int) time.Time {
	days := defaultDays
	if strings.HasSuffix(rangeStr, "d") {
		if parsed, err := strconv.Atoi(strings.TrimSuffix(rangeStr, "d")); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
}

// normalizeDay 兼容 sqlite 的 "2026-10-19" 与驱动返回的 "2026-10-19T00:00:00Z"。
func normalizeDay(raw string) string {
	if len(raw) >= 10 {
		return raw[:10]
	}
	return raw
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
