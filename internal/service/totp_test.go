package service

import (
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"graycrow/internal/crypto"
	"graycrow/internal/model"
)

// RFC 6238 附录 B 的 SHA1 测试向量（密钥为 ASCII "12345678901234567890" 的 base32）。
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTOTPRFC6238Vectors(t *testing.T) {
	cases := []struct {
		unix int64
		code string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, c := range cases {
		got, err := TOTPCode(rfcSecret, time.Unix(c.unix, 0).UTC())
		if err != nil {
			t.Fatal(err)
		}
		// 本实现固定 6 位，RFC 向量为 8 位，取末 6 位比对
		if got != c.code[len(c.code)-6:] {
			t.Fatalf("t=%d 期望 %s 实际 %s", c.unix, c.code[len(c.code)-6:], got)
		}
	}
}

func TestTOTPFlow(t *testing.T) {
	if err := crypto.Init("totp-test-master-key-0123456789-abcdef"); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open("file:totp?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.User{}, &model.Session{}, &model.RecoveryCode{}); err != nil {
		t.Fatal(err)
	}
	user, err := seedTestAdmin(db)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuth(db, time.Hour)

	// 未开启时直接发会话
	res, err := a.Login("admin", "admin123", "127.0.0.1", "test")
	if err != nil || res.TOTPRequired || res.Token == "" {
		t.Fatalf("未开启 2FA 应直接登录: %+v %v", res, err)
	}

	secret, otpauth, err := a.SetupTOTP(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || otpauth == "" {
		t.Fatalf("密钥/链接为空: %q %q", secret, otpauth)
	}
	if _, err := a.EnableTOTP(user.ID, "000000"); !errors.Is(err, ErrTOTPInvalid) {
		t.Fatalf("错误验证码不应开启，实际 %v", err)
	}

	code, err := TOTPCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	codes, err := a.EnableTOTP(user.ID, code)
	if err != nil || len(codes) != recoveryCount {
		t.Fatalf("开启失败: %v codes=%d", err, len(codes))
	}

	// 开启后登录只发票据
	res, err = a.Login("admin", "admin123", "127.0.0.1", "test")
	if err != nil || !res.TOTPRequired || res.Token != "" || res.Ticket == "" {
		t.Fatalf("应要求二次验证: %+v %v", res, err)
	}
	ticket := res.Ticket

	if _, err := a.VerifySecondFactor(ticket, "123456", false, "127.0.0.1", "test"); !errors.Is(err, ErrTOTPInvalid) {
		t.Fatalf("错误验证码不应换到会话，实际 %v", err)
	}
	good, err := a.VerifySecondFactor(ticket, code, false, "127.0.0.1", "test")
	if err != nil || good.Token == "" {
		t.Fatalf("正确验证码应换到会话: %+v %v", good, err)
	}
	if _, err := a.Validate(good.Token); err != nil {
		t.Fatalf("会话应有效: %v", err)
	}

	// 恢复码一次性
	rec := codes[0]
	if _, err := a.VerifySecondFactor(ticket, rec, true, "127.0.0.1", "test"); err != nil {
		t.Fatalf("恢复码应可用: %v", err)
	}
	if _, err := a.VerifySecondFactor(ticket, rec, true, "127.0.0.1", "test"); !errors.Is(err, ErrTOTPInvalid) {
		t.Fatalf("恢复码不应复用，实际 %v", err)
	}

	// 票据过期
	old := totpTicket(user.ID, time.Now().UTC().Add(-ticketTTL-time.Minute))
	if _, err := a.VerifySecondFactor(old, code, false, "127.0.0.1", "test"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("过期票据应被拒绝，实际 %v", err)
	}
	// 篡改票据
	if _, err := a.VerifySecondFactor(ticket+"x", code, false, "127.0.0.1", "test"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("篡改票据应被拒绝，实际 %v", err)
	}

	if err := a.DisableTOTP(user.ID, "wrong"); !errors.Is(err, ErrOldPassword) {
		t.Fatalf("密码错误不应关闭 2FA，实际 %v", err)
	}
	if err := a.DisableTOTP(user.ID, "admin123"); err != nil {
		t.Fatalf("关闭 2FA 失败: %v", err)
	}
	res, err = a.Login("admin", "admin123", "127.0.0.1", "test")
	if err != nil || res.TOTPRequired {
		t.Fatalf("关闭后应直接登录: %+v %v", res, err)
	}
	var n int64
	db.Model(&model.RecoveryCode{}).Where("user_id = ?", user.ID).Count(&n)
	if n != 0 {
		t.Fatalf("关闭后恢复码应清空，实际 %d", n)
	}
}
