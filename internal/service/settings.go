package service

import (
	"errors"
	"strconv"

	"gorm.io/gorm"

	"graycrow/internal/model"
)

var ErrSettingInvalid = errors.New("设置值不合法")

type SettingsService struct {
	db *gorm.DB
}

func NewSettingsService(db *gorm.DB) *SettingsService { return &SettingsService{db: db} }

func defaults() map[string]any {
	return map[string]any{
		"default_interval_sec": 3600,
		"dry_run_days":         3,
	}
}

func (s *SettingsService) Get() (map[string]any, error) {
	out := defaults()
	rows := []model.Setting{}
	if err := s.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		switch row.Key {
		case "default_interval_sec", "dry_run_days":
			if n, err := strconv.Atoi(row.Value); err == nil {
				out[row.Key] = n
			}
		}
	}
	return out, nil
}

// Update 只接受白名单键，并在写库前校验取值，避免把坏值写进调度参数。
func (s *SettingsService) Update(in map[string]any) error {
	allowed := map[string]bool{
		"default_interval_sec": true, "dry_run_days": true,
	}
	for key, value := range in {
		if !allowed[key] {
			continue
		}
		text, err := normalizeSetting(key, value)
		if err != nil {
			return err
		}
		// 结构体条件让 GORM 给列名加引号：key 是 MySQL 保留字，裸写 key = ? 会 1064
		row := model.Setting{Key: key}
		if err := s.db.Where(&model.Setting{Key: key}).Assign(model.Setting{Value: text}).FirstOrCreate(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeSetting(key string, value any) (string, error) {
	n, err := toInt(value)
	if err != nil {
		return "", ErrSettingInvalid
	}
	switch key {
	case "default_interval_sec":
		if n < 60 {
			return "", ErrSettingInvalid
		}
	case "dry_run_days":
		if n < 0 {
			return "", ErrSettingInvalid
		}
	}
	return strconv.Itoa(n), nil
}

func toInt(value any) (int, error) {
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		return strconv.Atoi(v)
	}
	return 0, ErrSettingInvalid
}
