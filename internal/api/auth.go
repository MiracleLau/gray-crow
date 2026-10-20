package api

import (
	"graycrow/internal/httpx"

	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"graycrow/internal/middleware"
	"graycrow/internal/model"
	"graycrow/internal/service"
)

type AuthHandler struct {
	auth         *service.Auth
	rules        *service.RuleService
	ttlSeconds   int
	secureCookie bool
}

func NewAuthHandler(auth *service.Auth, rules *service.RuleService, ttlSeconds int, secureCookie bool) *AuthHandler {
	return &AuthHandler{auth: auth, rules: rules, ttlSeconds: ttlSeconds, secureCookie: secureCookie}
}

func (h *AuthHandler) me(user *model.User) meResponse {
	ruleIDs, err := h.rules.VisibleRuleIDs(user)
	if err != nil || ruleIDs == nil {
		ruleIDs = []uint{}
	}
	return meResponse{User: *user, RuleIDs: ruleIDs}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type meResponse struct {
	model.User
	RuleIDs []uint `json:"rule_ids"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "请输入用户名与密码")
		return
	}
	res, err := h.auth.Login(strings.TrimSpace(req.Username), req.Password, c.ClientIP(), c.Request.UserAgent())
	switch {
	case errors.Is(err, service.ErrBadCredential):
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeBadCredential, err.Error())
		return
	case errors.Is(err, service.ErrUserDisabled):
		httpx.Fail(c, http.StatusForbidden, httpx.CodeUserDisabled, err.Error())
		return
	case errors.Is(err, service.ErrTooManyAttempt):
		httpx.Fail(c, http.StatusTooManyRequests, httpx.CodeTooManyAttempt, err.Error())
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "登录失败")
		return
	}
	if res.TOTPRequired {
		httpx.OK(c, gin.H{"totp_required": true, "ticket": res.Ticket})
		return
	}
	middleware.SetSession(c, res.Token, h.ttlSeconds, h.secureCookie)
	httpx.OK(c, h.me(res.User))
}

type totpRequest struct {
	Ticket   string `json:"ticket" binding:"required"`
	Code     string `json:"code" binding:"required"`
	Recovery bool   `json:"recovery"`
}

// TOTP 第二步登录：票据 + 动态码（或恢复码）换会话。
func (h *AuthHandler) TOTP(c *gin.Context) {
	var req totpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "请输入动态验证码")
		return
	}
	res, err := h.auth.VerifySecondFactor(req.Ticket, req.Code, req.Recovery, c.ClientIP(), c.Request.UserAgent())
	switch {
	case errors.Is(err, service.ErrTOTPInvalid), errors.Is(err, service.ErrTicketInvalid):
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeBadCredential, err.Error())
		return
	case errors.Is(err, service.ErrTooManyAttempt):
		httpx.Fail(c, http.StatusTooManyRequests, httpx.CodeTooManyAttempt, err.Error())
		return
	case errors.Is(err, service.ErrUserDisabled):
		httpx.Fail(c, http.StatusForbidden, httpx.CodeUserDisabled, err.Error())
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "验证失败")
		return
	}
	middleware.SetSession(c, res.Token, h.ttlSeconds, h.secureCookie)
	httpx.OK(c, h.me(res.User))
}

// SetupTOTP 生成密钥（未启用），返回密钥与 otpauth 链接供认证器录入。
func (h *AuthHandler) SetupTOTP(c *gin.Context) {
	secret, otpauth, err := h.auth.SetupTOTP(middleware.CurrentUser(c).ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "生成密钥失败")
		return
	}
	httpx.OK(c, gin.H{"secret": secret, "otpauth_url": otpauth})
}

func (h *AuthHandler) EnableTOTP(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "请输入动态验证码")
		return
	}
	codes, err := h.auth.EnableTOTP(middleware.CurrentUser(c).ID, req.Code)
	switch {
	case errors.Is(err, service.ErrTOTPInvalid):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, err.Error())
		return
	case errors.Is(err, service.ErrTOTPNotSetup):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, err.Error())
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "开启失败")
		return
	}
	httpx.OK(c, gin.H{"recovery_codes": codes})
}

func (h *AuthHandler) DisableTOTP(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "请输入当前密码")
		return
	}
	err := h.auth.DisableTOTP(middleware.CurrentUser(c).ID, req.Password)
	switch {
	case errors.Is(err, service.ErrOldPassword):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, err.Error())
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "关闭失败")
		return
	}
	httpx.OK(c, gin.H{})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if err := h.auth.Logout(middleware.CurrentToken(c)); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "退出失败")
		return
	}
	middleware.ClearSession(c, h.secureCookie)
	httpx.OK(c, gin.H{})
}

func (h *AuthHandler) Me(c *gin.Context) {
	httpx.OK(c, h.me(middleware.CurrentUser(c)))
}

type passwordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req passwordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "新密码至少 6 位")
		return
	}
	err := h.auth.ChangePassword(middleware.CurrentUser(c).ID, req.OldPassword, req.NewPassword)
	switch {
	case errors.Is(err, service.ErrOldPassword):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, err.Error())
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "修改失败")
		return
	}
	middleware.ClearSession(c, h.secureCookie)
	httpx.OK(c, gin.H{"relogin": true})
}
