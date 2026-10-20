package schedule

import (
	"strings"
	"testing"
	"time"
)

func shanghai(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	return loc
}

func TestIntervalNext(t *testing.T) {
	from := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	s, err := Parse("", "", 90, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.IsCron() {
		t.Fatal("空类型应按 interval 处理")
	}
	if got := s.Next(from); !got.Equal(from.Add(90 * time.Second)) {
		t.Fatalf("间隔下一次 %v", got)
	}
}

func TestCronNextUsesConfiguredTimezone(t *testing.T) {
	loc := shanghai(t)
	s, err := Parse(TypeCron, "0 9 * * *", 0, loc)
	if err != nil {
		t.Fatal(err)
	}
	// 2026-10-20 00:00 UTC = 上海 08:00，下一次应为上海 09:00 = 01:00 UTC
	from := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 10, 20, 9, 0, 0, 0, loc).UTC()
	got := s.Next(from)
	if !got.Equal(want) {
		t.Fatalf("cron 下一次应为 %v，实际 %v", want, got)
	}
	if got.Location() != time.UTC {
		t.Fatalf("写库时间应为 UTC，实际 %v", got.Location())
	}
}

func TestCronValidation(t *testing.T) {
	loc := shanghai(t)
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

	if _, err := Parse(TypeCron, "", 0, loc); err == nil {
		t.Fatal("空表达式应报错")
	}
	if _, err := Parse(TypeCron, "0 0 9 * * *", 0, loc); err == nil {
		t.Fatal("6 段（秒级）表达式应报错")
	}
	if _, err := Parse("weekly", "", 60, loc); err == nil || !strings.Contains(err.Error(), "未知调度方式") {
		t.Fatalf("未知类型应报错，实际 %v", err)
	}

	never, err := Parse(TypeCron, "0 0 31 2 *", 0, loc)
	if err != nil {
		t.Fatalf("语法应合法: %v", err)
	}
	if err := never.Validate(now, 0); err == nil {
		t.Fatal("永不触发的表达式应报错")
	}

	everyMinute, err := Parse(TypeCron, "* * * * *", 0, loc)
	if err != nil {
		t.Fatal(err)
	}
	if err := everyMinute.Validate(now, 60*time.Second); err != nil {
		t.Fatalf("1 分钟间隔应满足 60s 下限: %v", err)
	}
	if err := everyMinute.Validate(now, 2*time.Minute); err == nil {
		t.Fatal("1 分钟间隔不应满足 2 分钟下限")
	}
}
