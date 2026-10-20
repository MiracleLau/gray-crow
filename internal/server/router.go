package server

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"graycrow/internal/api"
	"graycrow/internal/config"
	"graycrow/internal/middleware"
	"graycrow/internal/service"
	webui "graycrow/web"
)

var embeddedFS = func() fs.FS {
	root, err := webui.FS()
	if err != nil {
		return nil
	}
	return root
}()

func New(cfg *config.Config, db *gorm.DB, auth *service.Auth, deps *api.Deps, version string) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Recovery(), middleware.AccessLog())
	r.GET("/healthz", health(cfg, db, version))

	r.NoRoute(serveStatic)

	v1 := r.Group("/api/v1", middleware.WriteGuard())

	authHandler := api.NewAuthHandler(auth, deps.Rules, cfg.SessionTTLHours*3600, cfg.CookieSecure)
	v1.POST("/auth/login", authHandler.Login)
	v1.POST("/auth/totp", authHandler.TOTP)

	authed := v1.Group("", middleware.Session(auth, cfg.CookieSecure))
	authed.POST("/auth/logout", authHandler.Logout)
	authed.GET("/auth/me", authHandler.Me)
	authed.POST("/auth/password", authHandler.ChangePassword)
	authed.POST("/auth/totp/setup", authHandler.SetupTOTP)
	authed.POST("/auth/totp/enable", authHandler.EnableTOTP)
	authed.POST("/auth/totp/disable", authHandler.DisableTOTP)

	// 普通用户可读：规则列表/详情按 rule_viewers 过滤
	authed.GET("/rules", deps.ListRules)
	authed.GET("/rules/:id", deps.GetRule)
	authed.GET("/rules/:id/runs", deps.ListRuleRuns)
	authed.GET("/rules/:id/diagnose", deps.DiagnoseRule)

	// 接收人可直接处置自己可见范围内的告警
	authed.GET("/alerts", deps.ListAlerts)
	authed.GET("/alerts/:id", deps.GetAlert)
	authed.PATCH("/alerts/:id/status", deps.SetAlertStatus)
	authed.POST("/alerts/bulk-status", deps.BulkAlertStatus)
	authed.POST("/alerts/:id/allowlist", deps.AllowlistAlert)
	authed.POST("/alerts/:id/renotify", deps.RenotifyAlert)
	authed.GET("/notify-records", deps.ListNotifyRecords)

	admin := authed.Group("", middleware.RequireAdmin())

	admin.POST("/rules", deps.CreateRule)
	admin.PUT("/rules/:id", deps.UpdateRule)
	admin.DELETE("/rules/:id", deps.DeleteRule)
	admin.POST("/rules/:id/scan", deps.ScanRule)
	admin.GET("/schedule/preview", deps.PreviewSchedule)

	// 历史手动清理：告警与通知记录不再自动删除
	admin.DELETE("/alerts/:id", deps.DeleteAlert)
	admin.POST("/alerts/bulk-delete", deps.BulkDeleteAlerts)
	admin.DELETE("/notify-records/:id", deps.DeleteNotifyRecord)
	admin.POST("/notify-records/bulk-delete", deps.BulkDeleteNotifyRecords)

	admin.GET("/allowlist", deps.ListAllowlist)
	admin.POST("/allowlist", deps.CreateAllowlist)
	admin.DELETE("/allowlist/:id", deps.DeleteAllowlist)

	admin.GET("/api-keys", deps.ListKeys)
	admin.POST("/api-keys", deps.CreateKey)
	admin.PUT("/api-keys/:id", deps.UpdateKey)
	admin.DELETE("/api-keys/:id", deps.DeleteKey)
	admin.GET("/api-keys/quota", deps.KeyQuota)
	admin.POST("/api-keys/check-all", deps.CheckAllKeys)
	admin.POST("/api-keys/reorder", deps.ReorderKeys)
	admin.GET("/api-keys/usage", deps.KeyUsage)
	admin.POST("/api-keys/:id/check", deps.CheckKey)

	admin.GET("/channels", deps.ListChannels)
	admin.POST("/channels", deps.CreateChannel)
	admin.POST("/channels/test", deps.TestChannelDraft)
	admin.PUT("/channels/:id", deps.UpdateChannel)
	admin.DELETE("/channels/:id", deps.DeleteChannel)
	admin.POST("/channels/:id/test", deps.TestChannel)

	admin.GET("/templates", deps.ListTemplates)
	admin.POST("/templates", deps.CreateTemplate)
	admin.PUT("/templates/:id", deps.UpdateTemplate)
	admin.DELETE("/templates/:id", deps.DeleteTemplate)

	admin.GET("/users", deps.ListUsers)
	admin.POST("/users", deps.CreateUser)
	admin.PUT("/users/:id", deps.UpdateUser)
	admin.DELETE("/users/:id", deps.DeleteUser)
	admin.PATCH("/users/:id/password", deps.ResetUserPassword)
	admin.PATCH("/users/:id/status", deps.SetUserStatus)
	admin.DELETE("/users/:id/totp", deps.ResetUserTOTP)

	authed.GET("/stats/overview", deps.StatsOverview)
	authed.GET("/stats/trend", deps.StatsTrend)
	authed.GET("/stats/by-rule", deps.StatsByRule)
	authed.GET("/stats/by-status", deps.StatsByStatus)
	authed.GET("/stats/export", deps.StatsExport)
	authed.GET("/stats/quota", deps.StatsQuota)

	admin.GET("/audit-logs", deps.ListAuditLogs)
	admin.GET("/settings", deps.GetSettings)
	admin.PUT("/settings", deps.UpdateSettings)

	return r
}

func health(cfg *config.Config, db *gorm.DB, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbStatus := "ok"
		if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
			dbStatus = "down"
		}
		status := "ok"
		code := http.StatusOK
		if dbStatus != "ok" {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{
			"status":  status,
			"version": version,
			"db":      dbStatus,
			"driver":  cfg.DBDriver,
			"scanner": cfg.ScanEnabled,
		})
	}
}
