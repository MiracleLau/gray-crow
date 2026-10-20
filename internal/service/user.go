package service

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"graycrow/internal/model"
)

var (
	ErrUserDuplicate = errors.New("用户名已存在")
	ErrSelfOperation = errors.New("不能对自己执行该操作")
	ErrLastAdmin     = errors.New("至少保留一个启用状态的管理员")
	ErrPasswordShort = errors.New("密码至少 6 位")
)

type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) *UserService { return &UserService{db: db} }

func (s *UserService) List(page, size int) ([]model.User, int64, error) {
	var total int64
	if err := s.db.Model(&model.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	users := []model.User{}
	err := s.db.Order("id asc").Offset((page - 1) * size).Limit(size).Find(&users).Error
	return users, total, err
}

type UserInput struct {
	Username string
	Nickname string
	Role     string
	Password string
	Status   *int8
}

func (s *UserService) Create(in UserInput) (*model.User, error) {
	username := strings.TrimSpace(in.Username)
	if username == "" {
		return nil, ErrNameRequired
	}
	if len(in.Password) < 6 {
		return nil, ErrPasswordShort
	}
	var count int64
	s.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil, ErrUserDuplicate
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		Nickname:     in.Nickname,
		Role:         normalizeRole(in.Role),
		Status:       1,
	}
	if err := s.db.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Update(id uint, in UserInput) (*model.User, error) {
	user, err := s.get(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if in.Nickname != "" {
		updates["nickname"] = in.Nickname
	}
	if in.Role != "" {
		if user.IsAdmin() && normalizeRole(in.Role) != model.RoleAdmin {
			if ok, err := s.hasOtherAdmin(user.ID); err != nil || !ok {
				return nil, ErrLastAdmin
			}
		}
		updates["role"] = normalizeRole(in.Role)
	}
	if in.Status != nil {
		if *in.Status != 1 && user.IsAdmin() {
			if ok, err := s.hasOtherAdmin(user.ID); err != nil || !ok {
				return nil, ErrLastAdmin
			}
		}
		updates["status"] = *in.Status
	}
	if len(updates) > 0 {
		if err := s.db.Model(user).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return s.get(id)
}

func (s *UserService) Delete(actor *model.User, id uint) error {
	if actor.ID == id {
		return ErrSelfOperation
	}
	user, err := s.get(id)
	if err != nil {
		return err
	}
	if user.IsAdmin() {
		if ok, err := s.hasOtherAdmin(id); err != nil || !ok {
			return ErrLastAdmin
		}
	}
	if err := s.db.Where("user_id = ?", id).Delete(&model.Session{}).Error; err != nil {
		return err
	}
	if err := s.db.Where("user_id = ?", id).Delete(&model.RuleViewer{}).Error; err != nil {
		return err
	}
	return s.db.Delete(&model.User{}, id).Error
}

func (s *UserService) ResetPassword(id uint, password string) error {
	if len(password) < 6 {
		return ErrPasswordShort
	}
	user, err := s.get(id)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.db.Model(user).Update("password_hash", string(hash)).Error; err != nil {
		return err
	}
	return s.db.Where("user_id = ?", id).Delete(&model.Session{}).Error
}

func (s *UserService) get(id uint) (*model.User, error) {
	var user model.User
	if err := s.db.First(&user, id).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

func (s *UserService) hasOtherAdmin(excludeID uint) (bool, error) {
	var count int64
	err := s.db.Model(&model.User{}).
		Where("role = ? AND status = 1 AND id <> ?", model.RoleAdmin, excludeID).
		Count(&count).Error
	return count > 0, err
}

func normalizeRole(role string) string {
	if role == model.RoleAdmin {
		return model.RoleAdmin
	}
	return model.RoleViewer
}
