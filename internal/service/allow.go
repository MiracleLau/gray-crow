package service

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
)

var (
	ErrAllowNotFound = errors.New("白名单记录不存在")
	ErrRepoFormat    = errors.New("仓库格式应为 owner/repo")
	ErrScopeInvalid  = errors.New("范围只能是 global 或 rule")
)

type AllowService struct {
	db *gorm.DB
}

func NewAllowService(db *gorm.DB) *AllowService { return &AllowService{db: db} }

func (s *AllowService) List(scope string, repo string) ([]model.Allowlist, error) {
	rows := []model.Allowlist{}
	tx := s.db.Order("id desc")
	if scope != "" {
		tx = tx.Where("scope = ?", scope)
	}
	if repo != "" {
		tx = tx.Where("repo_full_name LIKE ?", "%"+repo+"%")
	}
	if err := tx.Find(&rows).Error; err != nil {
		return nil, err
	}
	s.fillRuleNames(rows)
	return rows, nil
}

// fillRuleNames 补 rule_name 供列表页展示，避免前端再发一次请求。
func (s *AllowService) fillRuleNames(rows []model.Allowlist) {
	ids := []uint{}
	for _, row := range rows {
		if row.RuleID != nil {
			ids = append(ids, *row.RuleID)
		}
	}
	if len(ids) == 0 {
		return
	}
	names := map[uint]string{}
	var rules []model.Rule
	if err := s.db.Select("id", "name").Where("id IN ?", ids).Find(&rules).Error; err != nil {
		return
	}
	for _, r := range rules {
		names[r.ID] = r.Name
	}
	for i := range rows {
		if rows[i].RuleID != nil {
			rows[i].RuleName = names[*rows[i].RuleID]
		}
	}
}

func (s *AllowService) Create(actor *model.User, scope string, ruleID *uint, repo, reason string, expiresAt *time.Time) (*model.Allowlist, error) {
	if !strings.Contains(repo, "/") {
		return nil, ErrRepoFormat
	}
	if scope != "global" && scope != "rule" {
		return nil, ErrScopeInvalid
	}
	if scope == "rule" && (ruleID == nil || *ruleID == 0) {
		return nil, ErrScopeInvalid
	}
	if scope == "global" {
		ruleID = nil
	}
	row := &model.Allowlist{
		Scope:         scope,
		RuleID:        ruleID,
		RepoFullName:  strings.TrimSpace(repo),
		Reason:        reason,
		ExpiresAt:     expiresAt,
		CreatedBy:     actor.ID,
		CreatedByName: displayName(actor),
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *AllowService) Delete(id uint) error {
	var row model.Allowlist
	if err := s.db.First(&row, id).Error; err != nil {
		return ErrAllowNotFound
	}
	return s.db.Delete(&row).Error
}

func displayName(u *model.User) string {
	if u == nil {
		return "system"
	}
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}
