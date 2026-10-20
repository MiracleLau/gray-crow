// Package schedule 把「固定间隔」和「cron」两种调度统一成 Next。
package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	TypeInterval = "interval"
	TypeCron     = "cron"
)

// Parse 校验并构造调度：kind 为空按 interval；cron 表达式按 loc 时区解释。
func Parse(kind, expr string, intervalSec int, loc *time.Location) (*Schedule, error) {
	if loc == nil {
		loc = time.UTC
	}
	switch strings.TrimSpace(kind) {
	case "", TypeInterval:
		if intervalSec <= 0 {
			return nil, errors.New("扫描间隔必须大于 0")
		}
		return &Schedule{interval: time.Duration(intervalSec) * time.Second, loc: loc}, nil
	case TypeCron:
		expr = strings.TrimSpace(expr)
		if expr == "" {
			return nil, errors.New("cron 表达式不能为空")
		}
		// 只接受 5 段（分 时 日 月 周），没有秒级字段，最小粒度就是 1 分钟
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		parsed, err := parser.Parse(expr)
		if err != nil {
			return nil, fmt.Errorf("cron 表达式无效（需要 5 段：分 时 日 月 周）：%v", err)
		}
		return &Schedule{cron: parsed, loc: loc}, nil
	default:
		return nil, fmt.Errorf("未知调度方式 %q", kind)
	}
}

type Schedule struct {
	cron     cron.Schedule
	interval time.Duration
	loc      *time.Location
}

func (s *Schedule) IsCron() bool { return s.cron != nil }

func (s *Schedule) Interval() time.Duration { return s.interval }

func (s *Schedule) Location() *time.Location { return s.loc }

// Next 返回严格晚于 from 的下一次运行时间（UTC，便于直接写库）。
func (s *Schedule) Next(from time.Time) time.Time {
	if s.cron != nil {
		return s.cron.Next(from.In(s.loc)).UTC()
	}
	return from.Add(s.interval).UTC()
}

// Validate 检查表达式真的会触发，且相邻两次的间隔不低于 minInterval。
func (s *Schedule) Validate(now time.Time, minInterval time.Duration) error {
	next := s.Next(now)
	if next.IsZero() {
		return errors.New("cron 表达式不会触发")
	}
	if minInterval > 0 {
		if after := s.Next(next); !after.IsZero() && after.Sub(next) < minInterval {
			return fmt.Errorf("两次运行间隔不能小于 %s", minInterval)
		}
	}
	return nil
}
