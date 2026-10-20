package service

import (
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
)

type Audit struct {
	db *gorm.DB
}

func NewAudit(db *gorm.DB) *Audit { return &Audit{db: db} }

func (s *Audit) Record(actor *model.User, action, target, detail, ip string) {
	log := &model.AuditLog{Action: action, Target: target, Detail: detail, IP: ip}
	if actor != nil {
		log.ActorID = actor.ID
		log.ActorName = actor.Nickname
		if log.ActorName == "" {
			log.ActorName = actor.Username
		}
	} else {
		log.ActorName = "system"
	}
	_ = s.db.Create(log).Error
}

type AuditQuery struct {
	Action string
	From   *time.Time
	To     *time.Time
	Page   int
	Size   int
}

func (s *Audit) List(q AuditQuery) ([]model.AuditLog, int64, error) {
	tx := s.db.Model(&model.AuditLog{})
	if q.Action != "" {
		tx = tx.Where("action = ?", q.Action)
	}
	if q.From != nil {
		tx = tx.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		tx = tx.Where("created_at <= ?", *q.To)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	logs := []model.AuditLog{}
	err := tx.Order("id desc").Offset((q.Page - 1) * q.Size).Limit(q.Size).Find(&logs).Error
	return logs, total, err
}
