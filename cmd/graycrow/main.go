package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"graycrow/internal/api"
	"graycrow/internal/config"
	"graycrow/internal/crypto"
	"graycrow/internal/engine"
	"graycrow/internal/scanner"
	"graycrow/internal/server"
	"graycrow/internal/service"
	"graycrow/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}

	scanner.SetBase(cfg.GitHubAPIBase)

	schedLoc, err := time.LoadLocation(cfg.ScanTimezone)
	if err != nil {
		log.Fatalf("SCAN_TIMEZONE 无效: %v", err)
	}

	if err := crypto.Init(cfg.MasterKey, cfg.MasterKeyOld...); err != nil {
		log.Fatalf("初始化加密模块失败: %v", err)
	}

	db, err := store.Open(cfg)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		log.Fatalf("建表失败: %v", err)
	}
	if len(cfg.MasterKeyOld) > 0 {
		if _, err := service.Reencrypt(db); err != nil {
			log.Printf("密钥轮换未完成（下次启动会重试）: %v", err)
		}
	}
	if created, err := store.SeedAdmin(db, cfg.AdminUser, cfg.AdminPassword); err != nil {
		log.Fatalf("初始化管理员失败: %v", err)
	} else if created {
		log.Printf("已创建初始管理员 %s / %s，请登录后立即修改密码", cfg.AdminUser, cfg.AdminPassword)
	}
	if created, err := store.SeedNotifyTemplate(db); err != nil {
		log.Fatalf("初始化默认通知模板失败: %v", err)
	} else if created {
		log.Printf("已创建默认通知模板")
	}

	auth := service.NewAuth(db, time.Duration(cfg.SessionTTLHours)*time.Hour)
	go cleanupLoop(auth)

	channels := service.NewChannelService(db)
	audit := service.NewAudit(db)
	rules := service.NewRuleService(db, cfg.ScanMinInterval, schedLoc)
	deps := &api.Deps{
		DB:        db,
		Rules:     rules,
		Alerts:    service.NewAlertService(db, rules, channels),
		Stats:     service.NewStatsService(db, rules),
		Auth:      auth,
		Keys:      service.NewAPIKeyService(db),
		Allows:    service.NewAllowService(db),
		Channels:  channels,
		Templates: service.NewTemplateService(db),
		Users:     service.NewUserService(db),
		Settings:  service.NewSettingsService(db),
		Audit:     audit,
	}
	engineCtx, stopEngine := context.WithCancel(context.Background())
	defer stopEngine()
	retention := service.NewRetention(db, audit)
	go retention.Start(engineCtx)

	if cfg.ScanEnabled {
		deps.Engine = engine.New(db, service.NewKeyPool(db), channels, deps.Alerts, audit, cfg.APIRatePerMin, schedLoc)
		go deps.Engine.Run(engineCtx)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(cfg, db, auth, deps, Version),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("灰鸦 GrayCrow %s 启动于 :%s（db=%s, scan=%v）", Version, cfg.Port, cfg.DBDriver, cfg.ScanEnabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("启动失败: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopEngine()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("关闭异常: %v", err)
	}
	log.Print("已退出")
}

func cleanupLoop(auth *service.Auth) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if n, err := auth.CleanupExpired(); err != nil {
			log.Printf("清理过期会话失败: %v", err)
		} else if n > 0 {
			log.Printf("清理过期会话 %d 条", n)
		}
	}
}
