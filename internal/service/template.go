package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"graycrow/internal/model"
)

var (
	ErrTemplateNotFound = errors.New("模板不存在")
	ErrTemplateInUse    = errors.New("该模板仍被渠道引用，请先解绑")
	ErrTemplateDefault  = errors.New("默认模板不能删除")
	ErrTemplateName     = errors.New("请填写模板名称")
	ErrTemplateBody     = errors.New("请填写模板正文")
)

// TemplateService 汇总通知模板。变量：title / count / repos / items。
type TemplateService struct {
	db *gorm.DB
}

func NewTemplateService(db *gorm.DB) *TemplateService { return &TemplateService{db: db} }

func (s *TemplateService) List() ([]model.NotifyTemplate, error) {
	rows := []model.NotifyTemplate{}
	err := s.db.Order("is_default desc, id asc").Find(&rows).Error
	return rows, err
}

type TemplateInput struct {
	Name  string
	Title string
	Body  string
}

func (s *TemplateService) Create(actor *model.User, in TemplateInput) (*model.NotifyTemplate, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, ErrTemplateName
	}
	if strings.TrimSpace(in.Body) == "" {
		return nil, ErrTemplateBody
	}
	row := &model.NotifyTemplate{Name: in.Name, Title: strings.TrimSpace(in.Title), Body: in.Body, CreatedBy: actor.ID}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *TemplateService) Update(id uint, in TemplateInput) (*model.NotifyTemplate, error) {
	row := &model.NotifyTemplate{}
	if err := s.db.First(row, id).Error; err != nil {
		return nil, ErrTemplateNotFound
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, ErrTemplateName
	}
	if strings.TrimSpace(in.Body) == "" {
		return nil, ErrTemplateBody
	}
	if err := s.db.Model(row).Updates(map[string]any{
		"name": in.Name, "title": strings.TrimSpace(in.Title), "body": in.Body,
	}).Error; err != nil {
		return nil, err
	}
	return s.get(id)
}

func (s *TemplateService) Delete(id uint) error {
	row, err := s.get(id)
	if err != nil {
		return err
	}
	if row.IsDefault {
		return ErrTemplateDefault
	}
	var count int64
	s.db.Model(&model.Channel{}).Where("template_id = ?", id).Count(&count)
	if count > 0 {
		return ErrTemplateInUse
	}
	return s.db.Delete(&model.NotifyTemplate{}, id).Error
}

func (s *TemplateService) get(id uint) (*model.NotifyTemplate, error) {
	row := &model.NotifyTemplate{}
	if err := s.db.First(row, id).Error; err != nil {
		return nil, ErrTemplateNotFound
	}
	return row, nil
}
