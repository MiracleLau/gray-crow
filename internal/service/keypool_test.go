package service

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
	"graycrow/internal/scanner"
)

func TestKeyPoolRoundRobinAndSkip(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	keys := []model.APIKey{
		{Name: "a", TokenEnc: "x", Enabled: true, SortOrder: 1, Status: model.KeyStatusActive, QuotaLimit: 10, QuotaRemaining: 10},
		{Name: "b", TokenEnc: "x", Enabled: true, SortOrder: 2, Status: model.KeyStatusActive, QuotaLimit: 10, QuotaRemaining: 10},
		{Name: "c", TokenEnc: "x", Enabled: true, SortOrder: 3, Status: model.KeyStatusActive, QuotaLimit: 10, QuotaRemaining: 10},
	}
	for i := range keys {
		if err := db.Create(&keys[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	pool := NewKeyPool(db)

	order := []string{}
	for i := 0; i < 6; i++ {
		key, err := pool.Pick(now)
		if err != nil {
			t.Fatalf("第 %d 次取用失败: %v", i, err)
		}
		order = append(order, key.Name)
	}
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("轮替顺序错误: %v", order)
		}
	}

	// 禁用 / 冷却 / 停用 的密钥应被跳过
	db.Model(&model.APIKey{}).Where("name = ?", "b").Updates(map[string]any{"enabled": false})
	db.Model(&model.APIKey{}).Where("name = ?", "c").Updates(map[string]any{"status": model.KeyStatusCooling, "cooldown_until": future})
	for i := 0; i < 3; i++ {
		key, err := pool.Pick(now)
		if err != nil {
			t.Fatal(err)
		}
		if key.Name != "a" {
			t.Fatalf("应只剩 a 可用，实际 %s", key.Name)
		}
	}

	// 余量耗尽 → 池耗尽并给出重置时刻
	db.Model(&model.APIKey{}).Where("name = ?", "a").Updates(map[string]any{"quota_remaining": 1, "quota_reset_at": future})
	_, err := pool.Pick(now)
	exhausted, ok := err.(*PoolExhaustedError)
	if !ok {
		t.Fatalf("应返回池耗尽，实际 %v", err)
	}
	if exhausted.ResetAt.IsZero() {
		t.Fatal("池耗尽应给出最早恢复时刻")
	}

	// 窗口翻篇后余量按上限计算，密钥应重新可用
	rolled := future.Add(time.Minute)
	if _, err := pool.Pick(rolled); err != nil {
		t.Fatalf("额度重置后池不应再报耗尽: %v", err)
	}
	if a := loadKey(t, db, keys[0].ID); !usable(a, rolled) {
		t.Fatalf("窗口重置后 a 应可用: %+v", a)
	}
}

func TestKeyPoolObserveTransitions(t *testing.T) {
	db := newTestDB(t)
	key := &model.APIKey{Name: "a", TokenEnc: "x", Enabled: true, Status: model.KeyStatusActive, QuotaLimit: 10, QuotaRemaining: 10}
	if err := db.Create(key).Error; err != nil {
		t.Fatal(err)
	}
	pool := NewKeyPool(db)
	reset := time.Now().UTC().Add(30 * time.Minute)

	pool.Observe(key, scanner.Quota{Limit: 10, Remaining: 1, ResetAt: reset})
	reloaded := loadKey(t, db, key.ID)
	if reloaded.Status != model.KeyStatusLow {
		t.Fatalf("余量剩下 10%% 应进入 low，实际 %s", reloaded.Status)
	}
	if reloaded.QuotaSource != "header" || reloaded.TotalUsed != 1 || reloaded.QuotaRemaining != 1 {
		t.Fatalf("响应头未写回: %+v", reloaded)
	}

	pool.Observe(reloaded, scanner.Quota{Limit: 10, Remaining: 9, ResetAt: reset})
	reloaded = loadKey(t, db, key.ID)
	if reloaded.Status != model.KeyStatusActive || reloaded.TotalUsed != 2 {
		t.Fatalf("恢复后应回到 active: %+v", reloaded)
	}

	pool.Cooling(reloaded, time.Now().UTC().Add(time.Minute), "429")
	reloaded = loadKey(t, db, key.ID)
	if reloaded.Status != model.KeyStatusCooling || reloaded.CooldownUntil == nil {
		t.Fatalf("冷却状态未写入: %+v", reloaded)
	}
	if usable(reloaded, time.Now().UTC()) {
		t.Fatal("冷却中的密钥不应可用")
	}

	pool.MarkInvalid(reloaded, "401")
	reloaded = loadKey(t, db, key.ID)
	if reloaded.Status != model.KeyStatusInvalid || usable(reloaded, time.Now().UTC()) {
		t.Fatalf("失效密钥应不可用: %+v", reloaded)
	}

	fresh := &model.APIKey{Name: "b", TokenEnc: "x", Enabled: true, Status: model.KeyStatusActive, QuotaLimit: 10, QuotaRemaining: 10}
	if err := db.Create(fresh).Error; err != nil {
		t.Fatal(err)
	}
	pool.Failure(fresh, errString("upstream 500"))
	pool.Failure(fresh, errString("upstream 500"))
	if got := loadKey(t, db, fresh.ID); got.Status != model.KeyStatusActive || got.FailCount != 2 {
		t.Fatalf("两次失败不应进冷却: %+v", got)
	}
	pool.Failure(fresh, errString("upstream 500"))
	if got := loadKey(t, db, fresh.ID); got.Status != model.KeyStatusCooling {
		t.Fatalf("连续三次失败应进冷却: %+v", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func loadKey(t *testing.T, db *gorm.DB, id uint) *model.APIKey {
	t.Helper()
	var key model.APIKey
	if err := db.First(&key, id).Error; err != nil {
		t.Fatalf("读取密钥失败: %v", err)
	}
	return &key
}
