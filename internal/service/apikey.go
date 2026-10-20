package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/crypto"
	"graycrow/internal/model"
	"graycrow/internal/scanner"
)

var (
	ErrKeyNotFound   = errors.New("密钥不存在")
	ErrTokenRequired = errors.New("请粘贴 Token")
	ErrNameRequired  = errors.New("请填写名称")
)

type quotaPool struct {
	UpdatedAt time.Time      `json:"updated_at"`
	Pool      poolSummary    `json:"pool"`
	Keys      []model.APIKey `json:"keys"`
}

type poolSummary struct {
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
	Accounts  int `json:"accounts"`
	PerMinute int `json:"per_minute"`
}

type KeyUsage struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	AccountLogin string `json:"account_login"`
	TotalUsed    int    `json:"total_used"`
	Alerts       int    `json:"alerts"`
}

type APIKeyService struct {
	db *gorm.DB

	mu      sync.Mutex
	cache   *quotaPool
	cacheAt time.Time
}

func NewAPIKeyService(db *gorm.DB) *APIKeyService { return &APIKeyService{db: db} }

func (s *APIKeyService) List() ([]model.APIKey, error) {
	keys := []model.APIKey{}
	err := s.db.Order("sort_order asc, id asc").Find(&keys).Error
	return keys, err
}

func (s *APIKeyService) Create(actor *model.User, name, token string) (*model.APIKey, error) {
	if name == "" {
		return nil, ErrNameRequired
	}
	if token == "" {
		return nil, ErrTokenRequired
	}
	enc, err := crypto.Encrypt(token)
	if err != nil {
		return nil, err
	}
	var maxSort int
	s.db.Model(&model.APIKey{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSort)

	key := &model.APIKey{
		Name:      name,
		Provider:  "github",
		TokenEnc:  enc,
		TokenHint: crypto.Hint(enc),
		Enabled:   true,
		SortOrder: maxSort + 1,
		Status:    model.KeyStatusActive,
		CreatedBy: actor.ID,
	}
	if err := s.db.Create(key).Error; err != nil {
		return nil, err
	}
	s.probe(key)
	return key, nil
}

func (s *APIKeyService) Update(id uint, name *string, enabled *bool, token *string) (*model.APIKey, error) {
	key, err := s.get(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if name != nil && *name != "" {
		updates["name"] = *name
	}
	if enabled != nil {
		updates["enabled"] = *enabled
		if !*enabled {
			updates["status"] = model.KeyStatusDisabled
		}
	}
	if token != nil && *token != "" {
		enc, err := crypto.Encrypt(*token)
		if err != nil {
			return nil, err
		}
		updates["token_enc"] = enc
		updates["token_hint"] = crypto.Hint(enc)
	}
	if len(updates) > 0 {
		if err := s.db.Model(key).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if token != nil && *token != "" {
		s.probe(key)
	} else if enabled != nil && *enabled && key.Status == model.KeyStatusDisabled {
		s.probe(key)
	}
	return s.get(id)
}

func (s *APIKeyService) Delete(id uint) error {
	if _, err := s.get(id); err != nil {
		return err
	}
	return s.db.Delete(&model.APIKey{}, id).Error
}

func (s *APIKeyService) Check(id uint) (*model.APIKey, error) {
	key, err := s.get(id)
	if err != nil {
		return nil, err
	}
	s.probe(key)
	return s.get(id)
}

func (s *APIKeyService) CheckAll() (int, error) {
	keys, err := s.List()
	if err != nil {
		return 0, err
	}
	for i := range keys {
		s.probe(&keys[i])
	}
	return len(keys), nil
}

func (s *APIKeyService) Reorder(ids []uint) error {
	for i, id := range ids {
		if err := s.db.Model(&model.APIKey{}).Where("id = ?", id).Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

// Quota 返回池内实时余量：3 秒内存缓存 + 单把密钥 15 秒探活节流。
// 响应头写回的值（source=header）权重更高，探活只补空闲密钥。
func (s *APIKeyService) Quota() (*quotaPool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, err := s.List()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	stale := s.cache == nil || now.Sub(s.cacheAt) >= 3*time.Second
	if stale {
		for i := range keys {
			k := &keys[i]
			if !k.Enabled {
				continue
			}
			if k.QuotaUpdatedAt != nil && now.Sub(*k.QuotaUpdatedAt) < 15*time.Second {
				continue
			}
			s.probe(k)
		}
		s.cacheAt = now
	}

	pool := &quotaPool{UpdatedAt: now, Keys: keys}
	accounts := map[string]bool{}
	for _, k := range keys {
		if !k.Enabled {
			continue
		}
		pool.Pool.Limit += k.QuotaLimit
		pool.Pool.Remaining += k.QuotaRemaining
		if k.AccountLogin != "" {
			accounts[k.AccountLogin] = true
		}
	}
	pool.Pool.Accounts = len(accounts)
	pool.Pool.PerMinute = pool.Pool.Remaining
	s.cache = pool
	return pool, nil
}

func (s *APIKeyService) Usage() ([]KeyUsage, error) {
	keys, err := s.List()
	if err != nil {
		return nil, err
	}
	// 贡献告警 = 该密钥扫出的新增告警数（scan_runs 里已按密钥记账，不需要新表）
	contrib := map[uint]int{}
	var rows []struct {
		APIKeyID uint
		Alerts   int
	}
	if err := s.db.Model(&model.ScanRun{}).Select("api_key_id, SUM(new_alert_count) AS alerts").
		Group("api_key_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		contrib[row.APIKeyID] = row.Alerts
	}
	out := make([]KeyUsage, 0, len(keys))
	for _, k := range keys {
		out = append(out, KeyUsage{ID: k.ID, Name: k.Name, AccountLogin: k.AccountLogin, TotalUsed: k.TotalUsed, Alerts: contrib[k.ID]})
	}
	return out, nil
}

func (s *APIKeyService) get(id uint) (*model.APIKey, error) {
	var key model.APIKey
	if err := s.db.First(&key, id).Error; err != nil {
		return nil, ErrKeyNotFound
	}
	return &key, nil
}

func (s *APIKeyService) probe(key *model.APIKey) {
	now := time.Now().UTC()
	key.LastCheckedAt = &now
	key.QuotaUpdatedAt = &now
	key.QuotaSource = "endpoint"

	token, err := crypto.Decrypt(key.TokenEnc)
	if err != nil {
		key.Status = model.KeyStatusInvalid
		key.LastError = "密文解密失败，请重新录入 Token（MASTER_KEY 可能已变更）"
		s.saveProbe(key)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := scanner.Probe(ctx, token)
	if err != nil {
		key.FailCount++
		key.LastError = err.Error()
		if key.FailCount >= 3 || strings.Contains(err.Error(), "token 无效") {
			key.Status = model.KeyStatusInvalid
		}
		s.saveProbe(key)
		return
	}

	key.AccountLogin = result.Login
	key.AccountID = strconv.FormatInt(result.ID, 10)
	key.Scopes = result.Scopes
	key.FailCount = 0
	key.LastError = ""
	key.QuotaLimit = result.CodeSearch.Limit
	key.QuotaRemaining = result.CodeSearch.Remaining
	if !result.CodeSearch.ResetAt.IsZero() {
		reset := result.CodeSearch.ResetAt
		key.QuotaResetAt = &reset
	}
	if !key.Enabled {
		key.Status = model.KeyStatusDisabled
	} else if result.CodeSearch.Limit > 0 && result.CodeSearch.Remaining*10 <= result.CodeSearch.Limit {
		key.Status = model.KeyStatusLow
	} else {
		key.Status = model.KeyStatusActive
	}
	s.saveProbe(key)
}

func (s *APIKeyService) saveProbe(key *model.APIKey) {
	_ = s.db.Model(&model.APIKey{}).Where("id = ?", key.ID).Updates(map[string]any{
		"account_login":    key.AccountLogin,
		"account_id":       key.AccountID,
		"scopes":           key.Scopes,
		"status":           key.Status,
		"quota_limit":      key.QuotaLimit,
		"quota_remaining":  key.QuotaRemaining,
		"quota_reset_at":   key.QuotaResetAt,
		"quota_updated_at": key.QuotaUpdatedAt,
		"quota_source":     key.QuotaSource,
		"fail_count":       key.FailCount,
		"last_error":       key.LastError,
		"last_checked_at":  key.LastCheckedAt,
	}).Error
}
