package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"graycrow/internal/model"
)

var (
	ErrBadCredential  = errors.New("用户名或密码错误")
	ErrUserDisabled   = errors.New("账号已停用")
	ErrTooManyAttempt = errors.New("登录失败次数过多，请稍后再试")
	ErrSessionInvalid = errors.New("会话无效或已过期")
	ErrOldPassword    = errors.New("原密码不正确")
	ErrUserNotFound   = errors.New("用户不存在")
)

const (
	maxLoginFails = 5
	failWindow    = 10 * time.Minute
)

type failEntry struct {
	count int
	until time.Time
}

type Auth struct {
	db    *gorm.DB
	ttl   time.Duration
	mu    sync.Mutex
	fails map[string]*failEntry
}

func NewAuth(db *gorm.DB, ttl time.Duration) *Auth {
	return &Auth{db: db, ttl: ttl, fails: map[string]*failEntry{}}
}

func (a *Auth) Login(username, password, ip, ua string) (*LoginResult, error) {
	key := ip + "|" + username
	if a.blocked(key) {
		return nil, ErrTooManyAttempt
	}

	var user model.User
	err := a.db.Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		a.recordFail(key)
		return nil, ErrBadCredential
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		a.recordFail(key)
		return nil, ErrBadCredential
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}
	a.clearFail(key)

	// 开了两步验证就先发短时票据，验证通过再发会话
	if user.TOTPEnabled && user.TOTPSecretEnc != "" {
		return &LoginResult{User: &user, Ticket: totpTicket(user.ID, time.Now().UTC()), TOTPRequired: true}, nil
	}
	token, err := a.createSession(&user, ip, ua)
	if err != nil {
		return nil, err
	}
	return &LoginResult{User: &user, Token: token}, nil
}

func (a *Auth) createSession(user *model.User, ip, ua string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	sess := &model.Session{
		UserID:    user.ID,
		TokenHash: hashToken(token),
		IP:        cut(ip, 64),
		UserAgent: cut(ua, 255),
		ExpiresAt: now.Add(a.ttl),
	}
	if err := a.db.Create(sess).Error; err != nil {
		return "", err
	}
	if err := a.db.Model(user).Update("last_login_at", now).Error; err != nil {
		return "", err
	}
	user.LastLoginAt = &now
	return token, nil
}

func (a *Auth) Validate(token string) (*model.User, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}
	var sess model.Session
	if err := a.db.Where("token_hash = ?", hashToken(token)).First(&sess).Error; err != nil {
		return nil, ErrSessionInvalid
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		a.db.Delete(&sess)
		return nil, ErrSessionInvalid
	}
	var user model.User
	if err := a.db.First(&user, sess.UserID).Error; err != nil {
		return nil, ErrSessionInvalid
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}
	return &user, nil
}

func (a *Auth) Logout(token string) error {
	if token == "" {
		return nil
	}
	return a.db.Where("token_hash = ?", hashToken(token)).Delete(&model.Session{}).Error
}

func (a *Auth) ChangePassword(userID uint, oldPwd, newPwd string) error {
	var user model.User
	if err := a.db.First(&user, userID).Error; err != nil {
		return ErrUserNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPwd)) != nil {
		return ErrOldPassword
	}
	return a.setPassword(&user, newPwd, true)
}

func (a *Auth) ResetPassword(userID uint, newPwd string) error {
	var user model.User
	if err := a.db.First(&user, userID).Error; err != nil {
		return ErrUserNotFound
	}
	return a.setPassword(&user, newPwd, false)
}

func (a *Auth) setPassword(user *model.User, newPwd string, revokeSessions bool) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := a.db.Model(user).Update("password_hash", string(hash)).Error; err != nil {
		return err
	}
	if revokeSessions {
		return a.db.Where("user_id = ?", user.ID).Delete(&model.Session{}).Error
	}
	return nil
}

func (a *Auth) CleanupExpired() (int64, error) {
	res := a.db.Where("expires_at < ?", time.Now().UTC()).Delete(&model.Session{})
	return res.RowsAffected, res.Error
}

func (a *Auth) blocked(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	e, ok := a.fails[key]
	if !ok {
		return false
	}
	if time.Now().After(e.until) {
		delete(a.fails, key)
		return false
	}
	return e.count >= maxLoginFails
}

func (a *Auth) recordFail(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	e := a.fails[key]
	if e == nil || time.Now().After(e.until) {
		e = &failEntry{}
	}
	e.count++
	if e.count == 1 {
		e.until = time.Now().Add(failWindow)
	}
	a.fails[key] = e
}

func (a *Auth) clearFail(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.fails, key)
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
