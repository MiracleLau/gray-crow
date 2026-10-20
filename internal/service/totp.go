package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"graycrow/internal/crypto"
	"graycrow/internal/model"
)

var (
	ErrTOTPRequired  = errors.New("需要动态验证码")
	ErrTOTPInvalid   = errors.New("验证码不正确")
	ErrTOTPNotSetup  = errors.New("尚未开启两步验证")
	ErrTicketInvalid = errors.New("登录票据无效或已过期")
)

const (
	totpStep       = 30 * time.Second
	totpDigits     = 6
	totpSkew       = 1
	ticketTTL      = 5 * time.Minute
	recoveryCount  = 10
	totpIssuerName = "GrayCrow"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// LoginResult 一次登录尝试的结果：命中 2FA 时只发票据，不发会话。
type LoginResult struct {
	User         *model.User
	Token        string
	Ticket       string
	TOTPRequired bool
}

// NewTOTPSecret 生成 20 字节随机密钥（base32 无填充，160 bit）。
func NewTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b32.EncodeToString(buf), nil
}

// TOTPCode 按 RFC 6238 计算 t 时刻的验证码。
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(t.Unix()/int64(totpStep/time.Second)))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, value%1000000), nil
}

// VerifyTOTP 允许前后各一个时间步，容忍客户端时钟漂移。
func VerifyTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	for i := -totpSkew; i <= totpSkew; i++ {
		want, err := TOTPCode(secret, t.Add(time.Duration(i)*totpStep))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// OTPAuthURL 生成 otpauth:// 链接，供认证器 App 手动录入或扫码。
func OTPAuthURL(account, secret string) string {
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", totpIssuerName)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(totpDigits))
	query.Set("period", strconv.Itoa(int(totpStep/time.Second)))
	label := url.PathEscape(totpIssuerName + ":" + account)
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// SetupTOTP 生成并暂存密钥（此时尚未启用，需要用户用验证码确认）。
func (a *Auth) SetupTOTP(userID uint) (secret, otpauth string, err error) {
	var user model.User
	if err := a.db.First(&user, userID).Error; err != nil {
		return "", "", ErrUserNotFound
	}
	secret, err = NewTOTPSecret()
	if err != nil {
		return "", "", err
	}
	enc, err := crypto.Encrypt(secret)
	if err != nil {
		return "", "", err
	}
	err = a.db.Model(&user).Updates(map[string]any{"totp_secret_enc": enc, "totp_enabled": false}).Error
	if err != nil {
		return "", "", err
	}
	return secret, OTPAuthURL(user.Username, secret), nil
}

// EnableTOTP 校验一次验证码后正式开启，并返回一次性恢复码（仅此一次可见）。
func (a *Auth) EnableTOTP(userID uint, code string) ([]string, error) {
	var user model.User
	if err := a.db.First(&user, userID).Error; err != nil {
		return nil, ErrUserNotFound
	}
	secret, err := a.totpSecret(&user)
	if err != nil {
		return nil, err
	}
	if !VerifyTOTP(secret, code, time.Now().UTC()) {
		return nil, ErrTOTPInvalid
	}
	if err := a.db.Model(&user).Update("totp_enabled", true).Error; err != nil {
		return nil, err
	}
	return a.issueRecoveryCodes(userID)
}

func (a *Auth) DisableTOTP(userID uint, password string) error {
	var user model.User
	if err := a.db.First(&user, userID).Error; err != nil {
		return ErrUserNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return ErrOldPassword
	}
	return a.ResetTOTP(userID)
}

// ResetTOTP 清空密钥与恢复码（管理员为忘记设备的用户重置也用这里）。
func (a *Auth) ResetTOTP(userID uint) error {
	err := a.db.Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{"totp_secret_enc": "", "totp_enabled": false}).Error
	if err != nil {
		return err
	}
	return a.db.Where("user_id = ?", userID).Delete(&model.RecoveryCode{}).Error
}

func (a *Auth) totpSecret(user *model.User) (string, error) {
	if user.TOTPSecretEnc == "" {
		return "", ErrTOTPNotSetup
	}
	return crypto.Decrypt(user.TOTPSecretEnc)
}

func (a *Auth) issueRecoveryCodes(userID uint) ([]string, error) {
	if err := a.db.Where("user_id = ?", userID).Delete(&model.RecoveryCode{}).Error; err != nil {
		return nil, err
	}
	codes := make([]string, 0, recoveryCount)
	rows := make([]model.RecoveryCode, 0, recoveryCount)
	for i := 0; i < recoveryCount; i++ {
		buf := make([]byte, 6)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		code := b32.EncodeToString(buf)[:8]
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
		rows = append(rows, model.RecoveryCode{UserID: userID, CodeHash: string(hash)})
	}
	if err := a.db.Create(&rows).Error; err != nil {
		return nil, err
	}
	return codes, nil
}

// consumeRecoveryCode 命中即作废（一次性）。
func (a *Auth) consumeRecoveryCode(userID uint, code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return false
	}
	rows := []model.RecoveryCode{}
	err := a.db.Where("user_id = ? AND used_at IS NULL", userID).Find(&rows).Error
	if err != nil {
		return false
	}
	for _, row := range rows {
		if bcrypt.CompareHashAndPassword([]byte(row.CodeHash), []byte(code)) != nil {
			continue
		}
		now := time.Now().UTC()
		if err := a.db.Model(&model.RecoveryCode{}).Where("id = ? AND used_at IS NULL", row.ID).
			Update("used_at", now).Error; err != nil {
			return false
		}
		return true
	}
	return false
}

func totpTicket(userID uint, now time.Time) string {
	body := fmt.Sprintf("%d.%d", userID, now.Add(ticketTTL).Unix())
	return body + "." + crypto.Sign(body)
}

func parseTicket(ticket string, now time.Time) (uint, error) {
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 {
		return 0, ErrTicketInvalid
	}
	body := parts[0] + "." + parts[1]
	if !crypto.VerifySign(body, parts[2]) {
		return 0, ErrTicketInvalid
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || now.Unix() > exp {
		return 0, ErrTicketInvalid
	}
	id, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, ErrTicketInvalid
	}
	return uint(id), nil
}

// VerifySecondFactor 用登录票据 + 动态码（或恢复码）换取正式会话。
func (a *Auth) VerifySecondFactor(ticket, code string, recovery bool, ip, ua string) (*LoginResult, error) {
	key := "totp|" + ip
	if a.blocked(key) {
		return nil, ErrTooManyAttempt
	}
	userID, err := parseTicket(ticket, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var user model.User
	if err := a.db.First(&user, userID).Error; err != nil {
		return nil, ErrTicketInvalid
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}
	if !user.TOTPEnabled {
		return nil, ErrTOTPNotSetup
	}
	ok := false
	if recovery {
		ok = a.consumeRecoveryCode(user.ID, code)
	} else if secret, err := a.totpSecret(&user); err == nil {
		ok = VerifyTOTP(secret, code, time.Now().UTC())
	}
	if !ok {
		a.recordFail(key)
		return nil, ErrTOTPInvalid
	}
	a.clearFail(key)
	token, err := a.createSession(&user, ip, ua)
	if err != nil {
		return nil, err
	}
	return &LoginResult{User: &user, Token: token}, nil
}
