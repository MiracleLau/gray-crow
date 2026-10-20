package model

import "time"

type User struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Username      string `gorm:"size:191;uniqueIndex;not null" json:"username"`
	PasswordHash  string `gorm:"size:191;not null" json:"-"`
	Nickname      string `gorm:"size:191" json:"nickname"`
	Role          string `gorm:"size:16;not null;default:viewer" json:"role"`
	Status        int8   `gorm:"not null;default:1" json:"status"`
	TOTPSecretEnc string `gorm:"size:255" json:"-"`
	// default:false 不只是零值语义：AutoMigrate 给已有表加 NOT NULL 列必须有默认值
	TOTPEnabled bool       `gorm:"not null;default:false" json:"totp_enabled"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (User) TableName() string { return "users" }

func (u *User) Display() string {
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}

func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

type RecoveryCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"index;not null" json:"user_id"`
	CodeHash  string     `gorm:"size:191;not null" json:"-"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (RecoveryCode) TableName() string { return "recovery_codes" }

type Session struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"index;not null"`
	TokenHash string    `gorm:"size:191;uniqueIndex;not null"`
	IP        string    `gorm:"size:64"`
	UserAgent string    `gorm:"size:255"`
	ExpiresAt time.Time `gorm:"index;not null"`
	CreatedAt time.Time
}

func (Session) TableName() string { return "sessions" }
