package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"graycrow/internal/model"
)

var testDBCounter int64

func newTestAuth(t *testing.T, ttl time.Duration) *Auth {
	t.Helper()
	dsn := fmt.Sprintf("file:test%d?mode=memory&cache=shared", atomic.AddInt64(&testDBCounter, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.User{}, &model.Session{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if _, err := seedTestAdmin(db); err != nil {
		t.Fatalf("seed 失败: %v", err)
	}
	return NewAuth(db, ttl)
}

func seedTestAdmin(db *gorm.DB) (*model.User, error) {
	u := &model.User{Username: "admin", PasswordHash: hashOf("admin123"), Nickname: "管理员", Role: model.RoleAdmin, Status: 1}
	return u, db.Create(u).Error
}

func TestLoginAndSession(t *testing.T) {
	a := newTestAuth(t, time.Hour)

	if _, err := a.Login("admin", "wrong", "127.0.0.1", "test"); !errors.Is(err, ErrBadCredential) {
		t.Fatalf("错误密码应返回 ErrBadCredential，实际 %v", err)
	}

	res, err := a.Login("admin", "admin123", "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("正确密码登录失败: %v", err)
	}
	if res.Token == "" || res.User.Role != model.RoleAdmin {
		t.Fatalf("登录结果异常: token=%q role=%q", res.Token, res.User.Role)
	}
	if res.User.LastLoginAt == nil {
		t.Fatal("登录后应更新 last_login_at")
	}

	got, err := a.Validate(res.Token)
	if err != nil || got.ID != res.User.ID {
		t.Fatalf("会话校验失败: user=%v err=%v", got, err)
	}

	if err := a.Logout(res.Token); err != nil {
		t.Fatalf("登出失败: %v", err)
	}
	if _, err := a.Validate(res.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("登出后会话应失效，实际 %v", err)
	}
}

func TestExpiredAndDisabled(t *testing.T) {
	a := newTestAuth(t, -time.Minute)
	res, err := a.Login("admin", "admin123", "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if _, err := a.Validate(res.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("过期会话应失效，实际 %v", err)
	}

	fresh := newTestAuth(t, time.Hour)
	if err := fresh.db.Model(&model.User{}).Where("username = ?", "admin").Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Login("admin", "admin123", "127.0.0.1", "test"); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("停用账号应拒绝登录，实际 %v", err)
	}
}

func TestLoginThrottle(t *testing.T) {
	a := newTestAuth(t, time.Hour)
	for i := 0; i < maxLoginFails; i++ {
		if _, err := a.Login("admin", "wrong", "10.0.0.1", "test"); !errors.Is(err, ErrBadCredential) {
			t.Fatalf("第 %d 次失败应返回凭证错误，实际 %v", i+1, err)
		}
	}
	if _, err := a.Login("admin", "admin123", "10.0.0.1", "test"); !errors.Is(err, ErrTooManyAttempt) {
		t.Fatalf("超过阈值应限流，实际 %v", err)
	}
	if _, err := a.Login("admin", "admin123", "10.0.0.2", "test"); err != nil {
		t.Fatalf("其他 IP 不应被限流，实际 %v", err)
	}
}

func TestChangePasswordRevokesSessions(t *testing.T) {
	a := newTestAuth(t, time.Hour)
	res, _ := a.Login("admin", "admin123", "127.0.0.1", "test")
	user, _ := a.Validate(res.Token)

	if err := a.ChangePassword(user.ID, "nope", "newpass123"); !errors.Is(err, ErrOldPassword) {
		t.Fatalf("原密码错误应被拒绝，实际 %v", err)
	}
	if err := a.ChangePassword(user.ID, "admin123", "newpass123"); err != nil {
		t.Fatalf("改密失败: %v", err)
	}
	if _, err := a.Validate(res.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("改密后旧会话应被吊销，实际 %v", err)
	}
	if _, err := a.Login("admin", "newpass123", "127.0.0.1", "test"); err != nil {
		t.Fatalf("新密码应可登录: %v", err)
	}
}

func TestCleanupExpired(t *testing.T) {
	a := newTestAuth(t, -time.Minute)
	if _, err := a.Login("admin", "admin123", "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	n, err := a.CleanupExpired()
	if err != nil || n != 1 {
		t.Fatalf("应清理 1 条过期会话，实际 n=%d err=%v", n, err)
	}
}

func hashOf(pwd string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.MinCost)
	return string(hash)
}
