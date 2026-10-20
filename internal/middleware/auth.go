package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"graycrow/internal/httpx"
	"graycrow/internal/model"
	"graycrow/internal/service"
)

const CookieName = "gc_session"

const (
	contextUser  = "gc_user"
	contextToken = "gc_token"
)

func setSessionCookie(c *gin.Context, token string, maxAge int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func SetSession(c *gin.Context, token string, ttlSeconds int, secure bool) {
	setSessionCookie(c, token, ttlSeconds, secure)
}

func ClearSession(c *gin.Context, secure bool) {
	setSessionCookie(c, "", -1, secure)
}

func Session(auth *service.Auth, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(CookieName)
		if err != nil || raw == "" {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "未登录")
			return
		}
		user, err := auth.Validate(raw)
		if err != nil {
			ClearSession(c, secure)
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "会话已失效，请重新登录")
			return
		}
		c.Set(contextUser, user)
		c.Set(contextToken, raw)
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if u := CurrentUser(c); u == nil || !u.IsAdmin() {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "需要管理员权限")
			return
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) *model.User {
	if v, ok := c.Get(contextUser); ok {
		if u, ok := v.(*model.User); ok {
			return u
		}
	}
	return nil
}

func CurrentToken(c *gin.Context) string {
	if v, ok := c.Get(contextToken); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
