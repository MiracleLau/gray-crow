package service

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/crypto"
	"graycrow/internal/model"
	"graycrow/internal/scanner"
)

// reserve 每把密钥留存的余量，避免踩到 0 触发上游 403。
const reserve = 1

type PoolExhaustedError struct {
	ResetAt time.Time
}

func (e *PoolExhaustedError) Error() string {
	if e.ResetAt.IsZero() {
		return "密钥池耗尽：没有可用密钥（请检查池内密钥状态）"
	}
	return "密钥池耗尽：等待额度窗口重置"
}

// KeyPool 扫描侧的密钥轮替：严格 round-robin，跳过不可用，池耗尽时给出最早恢复时刻。
type KeyPool struct {
	db *gorm.DB

	mu     sync.Mutex
	cursor int
}

func NewKeyPool(db *gorm.DB) *KeyPool { return &KeyPool{db: db} }

func (p *KeyPool) Pick(now time.Time) (*model.APIKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	keys, err := p.list()
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, &PoolExhaustedError{}
	}
	for offset := 0; offset < len(keys); offset++ {
		idx := (p.cursor + offset) % len(keys)
		key := &keys[idx]
		if !usable(key, now) {
			continue
		}
		p.cursor = (idx + 1) % len(keys)
		return key, nil
	}
	return nil, &PoolExhaustedError{ResetAt: poolResetAt(keys, now)}
}

func (p *KeyPool) Token(key *model.APIKey) (string, error) {
	token, err := crypto.Decrypt(key.TokenEnc)
	if err != nil {
		p.MarkInvalid(key, "密文解密失败，请重新录入 Token（MASTER_KEY 可能已变更）")
		return "", err
	}
	return token, nil
}

// Observe 用响应头写回权威余量（header 来源优先于探活）。
func (p *KeyPool) Observe(key *model.APIKey, q scanner.Quota) {
	now := time.Now().UTC()
	if q.Limit > 0 {
		key.QuotaLimit = q.Limit
	}
	key.QuotaRemaining = q.Remaining
	if !q.ResetAt.IsZero() {
		reset := q.ResetAt
		key.QuotaResetAt = &reset
	}
	key.QuotaSource = "header"
	key.QuotaUpdatedAt = &now
	key.TotalUsed++
	key.LastUsedAt = &now
	key.FailCount = 0
	key.LastError = ""
	key.CooldownUntil = nil
	switch {
	case !key.Enabled:
		key.Status = model.KeyStatusDisabled
	case key.QuotaLimit > 0 && key.QuotaRemaining*10 <= key.QuotaLimit:
		key.Status = model.KeyStatusLow
	default:
		key.Status = model.KeyStatusActive
	}
	p.save(key)
}

func (p *KeyPool) Cooling(key *model.APIKey, until time.Time, reason string) {
	key.Status = model.KeyStatusCooling
	key.CooldownUntil = &until
	key.LastError = reason
	p.save(key)
}

func (p *KeyPool) MarkInvalid(key *model.APIKey, reason string) {
	key.Status = model.KeyStatusInvalid
	key.LastError = reason
	p.save(key)
}

// Failure 记录 5xx / 网络错误；连续 3 次进冷却 60s。
func (p *KeyPool) Failure(key *model.APIKey, err error) {
	key.FailCount++
	key.LastError = truncateErr(err.Error())
	if key.FailCount >= 3 {
		until := time.Now().UTC().Add(time.Minute)
		key.Status = model.KeyStatusCooling
		key.CooldownUntil = &until
	}
	p.save(key)
}

// LowMode 池内没有余量充足的密钥时降频（由调用方把间隔翻倍）。
func (p *KeyPool) LowMode() bool {
	keys, err := p.list()
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	for i := range keys {
		key := &keys[i]
		if !usable(key, now) {
			continue
		}
		if key.QuotaLimit == 0 || remaining(key, now)*2 > key.QuotaLimit {
			return false
		}
	}
	return len(keys) > 0
}

func (p *KeyPool) list() ([]model.APIKey, error) {
	keys := []model.APIKey{}
	err := p.db.Order("sort_order asc, id asc").Find(&keys).Error
	return keys, err
}

func (p *KeyPool) save(key *model.APIKey, extraFields ...string) {
	fields := []string{"status", "cooldown_until", "last_error", "fail_count", "quota_limit", "quota_remaining",
		"quota_reset_at", "quota_updated_at", "quota_source", "total_used", "last_used_at"}
	fields = append(fields, extraFields...)
	_ = p.db.Model(&model.APIKey{}).Where("id = ?", key.ID).Select(fields).Updates(key).Error
}

func usable(key *model.APIKey, now time.Time) bool {
	if !key.Enabled || key.Status == model.KeyStatusDisabled || key.Status == model.KeyStatusInvalid {
		return false
	}
	if key.CooldownUntil != nil && key.CooldownUntil.After(now) {
		return false
	}
	return remaining(key, now) > reserve
}

// remaining 额度窗口已翻篇时按满额度计算，否则一把用尽的密钥永远回不来。
func remaining(key *model.APIKey, now time.Time) int {
	if key.QuotaResetAt != nil && now.After(*key.QuotaResetAt) {
		return key.QuotaLimit
	}
	return key.QuotaRemaining
}

// poolResetAt 池耗尽后的最早恢复时刻：冷却结束与额度重置取较早者，都没有则零值。
func poolResetAt(keys []model.APIKey, now time.Time) time.Time {
	var earliest time.Time
	for i := range keys {
		key := &keys[i]
		if !key.Enabled {
			continue
		}
		var candidate time.Time
		switch {
		case key.CooldownUntil != nil && key.CooldownUntil.After(now):
			candidate = *key.CooldownUntil
		case remaining(key, now) <= reserve && key.QuotaResetAt != nil && key.QuotaResetAt.After(now):
			candidate = *key.QuotaResetAt
		}
		if candidate.IsZero() {
			continue
		}
		if earliest.IsZero() || candidate.Before(earliest) {
			earliest = candidate
		}
	}
	return earliest
}

func truncateErr(msg string) string {
	if len(msg) <= 500 {
		return msg
	}
	return fmt.Sprintf("%s…", msg[:499])
}
