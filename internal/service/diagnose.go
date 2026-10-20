package service

import (
	"time"

	"graycrow/internal/model"
)

type KeywordStat struct {
	Keyword       string `json:"keyword"`
	Total         int    `json:"total"`
	ContextOnly   int    `json:"context_only"`
	FalsePositive int    `json:"false_positive"`
}

type DetectorStat struct {
	Detector string `json:"detector"`
	Total    int    `json:"total"`
}

type Suggestion struct {
	Kind    string  `json:"kind"`
	Keyword string  `json:"keyword,omitempty"`
	Value   float64 `json:"value,omitempty"`
}

type RuleDiagnosis struct {
	WindowDays        int            `json:"window_days"`
	DryRun            bool           `json:"dry_run"`
	Total             int            `json:"total"`
	Judged            int            `json:"judged"`
	FalsePositive     int            `json:"false_positive"`
	FalsePositiveRate float64        `json:"false_positive_rate"`
	Detected          int            `json:"detected"`
	ContextOnly       int            `json:"context_only"`
	ByKeyword         []KeywordStat  `json:"by_keyword"`
	ByDetector        []DetectorStat `json:"by_detector"`
	Suggestions       []Suggestion   `json:"suggestions"`
}

const (
	diagnoseMaxDays   = 30
	diagnoseMinHits   = 20
	diagnoseMinJudged = 10
)

// Diagnose 依据规则自身的命中与处置情况给出干跑期结论，供管理员决定放量还是收紧关键词。
func (s *RuleService) Diagnose(id uint, actor *model.User) (*RuleDiagnosis, error) {
	dto, err := s.Get(id, actor)
	if err != nil {
		return nil, err
	}
	// 固定 30 天窗口：阈值在 Go 侧算好再交给 DB 比较，避免把读回来的时间再拿去比（各驱动时区往返不一致）
	start := time.Now().UTC().AddDate(0, 0, -diagnoseMaxDays)
	out := &RuleDiagnosis{
		WindowDays: diagnoseMaxDays,
		DryRun:     dto.DryRun,
		ByKeyword:  []KeywordStat{},
		ByDetector: []DetectorStat{},
	}

	var rows []model.Alert
	err = s.db.Select("matched_keyword", "detector_id", "status").
		Where("rule_id = ? AND first_seen_at >= ?", id, start).Find(&rows).Error
	if err != nil {
		return nil, err
	}

	keywords := map[string]*KeywordStat{}
	detectors := map[string]int{}
	for _, row := range rows {
		out.Total++
		// 二次判定命中（有 detector）才算识别出密钥形状，否则只算关键词上下文命中
		detected := row.DetectorID != "" && row.DetectorID != "context-only"
		if detected {
			out.Detected++
		} else {
			out.ContextOnly++
		}
		if row.Status == model.AlertFalsePositive {
			out.FalsePositive++
		}
		if row.Status != model.AlertPending {
			out.Judged++
		}
		kw := keywords[row.MatchedKeyword]
		if kw == nil {
			kw = &KeywordStat{Keyword: row.MatchedKeyword}
			keywords[row.MatchedKeyword] = kw
		}
		kw.Total++
		if !detected {
			kw.ContextOnly++
		}
		if row.Status == model.AlertFalsePositive {
			kw.FalsePositive++
		}
		detectors[row.DetectorID]++
	}
	if out.Total > 0 {
		out.FalsePositiveRate = round2(float64(out.FalsePositive) / float64(out.Total))
	}
	for _, kw := range keywords {
		out.ByKeyword = append(out.ByKeyword, *kw)
	}
	for name, total := range detectors {
		out.ByDetector = append(out.ByDetector, DetectorStat{Detector: name, Total: total})
	}
	sortKeywordStats(out.ByKeyword)
	sortDetectorStats(out.ByDetector)
	out.Suggestions = suggestions(out)
	return out, nil
}

func suggestions(d *RuleDiagnosis) []Suggestion {
	if d.Total == 0 {
		return []Suggestion{{Kind: "no_data"}}
	}
	out := []Suggestion{}
	for _, kw := range d.ByKeyword {
		if kw.Total < diagnoseMinHits || float64(kw.ContextOnly)/float64(kw.Total) < 0.8 {
			continue
		}
		out = append(out, Suggestion{Kind: "drop_keyword", Keyword: kw.Keyword, Value: round2(float64(kw.ContextOnly) / float64(kw.Total))})
	}
	switch {
	case d.FalsePositiveRate >= 0.4 && d.Judged >= diagnoseMinJudged:
		out = append(out, Suggestion{Kind: "keep_dry_run", Value: d.FalsePositiveRate})
	case d.DryRun && d.Total >= diagnoseMinJudged && d.FalsePositiveRate < 0.15 && d.Detected > d.ContextOnly:
		out = append(out, Suggestion{Kind: "ready", Value: d.FalsePositiveRate})
	case len(out) == 0:
		out = append(out, Suggestion{Kind: "ok", Value: d.FalsePositiveRate})
	}
	return out
}

func sortKeywordStats(items []KeywordStat) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Total > items[j-1].Total; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func sortDetectorStats(items []DetectorStat) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Total > items[j-1].Total; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
