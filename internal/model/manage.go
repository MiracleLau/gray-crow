package model

import "time"

type APIKey struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:191;not null" json:"name"`
	Provider       string     `gorm:"size:32;not null;default:github" json:"provider"`
	TokenEnc       string     `gorm:"type:text;not null" json:"-"`
	TokenHint      string     `gorm:"size:32" json:"token_hint"`
	AccountLogin   string     `gorm:"size:191;index" json:"account_login"`
	AccountID      string     `gorm:"size:64" json:"account_id"`
	Scopes         string     `gorm:"size:255" json:"scopes"`
	Enabled        bool       `gorm:"not null" json:"enabled"`
	SortOrder      int        `gorm:"not null;default:0" json:"sort_order"`
	Status         string     `gorm:"size:16;not null;default:disabled" json:"status"`
	QuotaLimit     int        `json:"quota_limit"`
	QuotaRemaining int        `json:"quota_remaining"`
	QuotaResetAt   *time.Time `json:"quota_reset_at"`
	QuotaUpdatedAt *time.Time `json:"quota_updated_at"`
	QuotaSource    string     `gorm:"size:16" json:"quota_source"`
	CooldownUntil  *time.Time `json:"cooldown_until"`
	FailCount      int        `json:"fail_count"`
	LastError      string     `gorm:"size:500" json:"last_error"`
	TotalUsed      int        `json:"total_used"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	LastCheckedAt  *time.Time `json:"-"`
	CreatedBy      uint       `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (APIKey) TableName() string { return "api_keys" }

const (
	KeyStatusActive   = "active"
	KeyStatusLow      = "low"
	KeyStatusCooling  = "cooling"
	KeyStatusInvalid  = "invalid"
	KeyStatusDisabled = "disabled"
)

type Rule struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Name          string     `gorm:"size:191;not null" json:"name"`
	Description   string     `gorm:"size:500" json:"description"`
	Keywords      []string   `gorm:"serializer:json" json:"keywords"`
	QueryTemplate string     `gorm:"size:500" json:"query_template"`
	ScheduleType  string     `gorm:"size:16;not null;default:interval" json:"schedule_type"` // interval | cron
	CronExpr      string     `gorm:"size:100" json:"cron_expr"`
	IntervalSec   int        `gorm:"not null" json:"interval_sec"`
	Severity      string     `gorm:"size:16;not null;default:high" json:"severity"`
	Enabled       bool       `gorm:"not null" json:"enabled"`
	DryRun        bool       `gorm:"not null" json:"dry_run"`
	NextScanAt    time.Time  `gorm:"index;not null" json:"next_scan_at"`
	LastScanAt    *time.Time `json:"last_scan_at"`
	CreatedBy     uint       `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (Rule) TableName() string { return "rules" }

type RuleChannel struct {
	ID        uint `gorm:"primaryKey"`
	RuleID    uint `gorm:"uniqueIndex:idx_rule_channel"`
	ChannelID uint `gorm:"uniqueIndex:idx_rule_channel"`
}

func (RuleChannel) TableName() string { return "rule_channels" }

type RuleViewer struct {
	ID     uint `gorm:"primaryKey"`
	RuleID uint `gorm:"uniqueIndex:idx_rule_viewer"`
	UserID uint `gorm:"uniqueIndex:idx_rule_viewer"`
}

func (RuleViewer) TableName() string { return "rule_viewers" }

type Allowlist struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Scope         string     `gorm:"size:16;not null" json:"scope"` // global | rule
	RuleID        *uint      `gorm:"index" json:"rule_id"`
	RuleName      string     `gorm:"-" json:"rule_name"`
	RepoFullName  string     `gorm:"size:191;index;not null" json:"repo_full_name"`
	Reason        string     `gorm:"size:500" json:"reason"`
	ExpiresAt     *time.Time `json:"expires_at"`
	CreatedByName string     `gorm:"size:191" json:"created_by_name"`
	CreatedBy     uint       `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (Allowlist) TableName() string { return "allowlist" }

type Channel struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	Name       string         `gorm:"size:191;not null" json:"name"`
	Type       string         `gorm:"size:32;not null" json:"type"`
	TemplateID *uint          `gorm:"index" json:"template_id"`
	Config     map[string]any `gorm:"serializer:json" json:"config"`
	SecretEnc  string         `gorm:"type:text" json:"-"`
	Enabled    bool           `gorm:"not null" json:"enabled"`
	LastStatus string         `gorm:"size:16" json:"last_status"`
	LastError  string         `gorm:"size:500" json:"last_error"`
	LastSentAt *time.Time     `json:"last_sent_at"`
	CreatedBy  uint           `json:"-"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (Channel) TableName() string { return "channels" }

// NotifyTemplate 汇总通知正文模板：一次扫描的命中合成一条消息，变量见 notify.Batch.Vars。
type NotifyTemplate struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:191;not null" json:"name"`
	Title     string    `gorm:"size:255" json:"title"`
	Body      string    `gorm:"type:text" json:"body"`
	IsDefault bool      `gorm:"not null;default:false" json:"is_default"`
	CreatedBy uint      `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (NotifyTemplate) TableName() string { return "notify_templates" }

type Setting struct {
	Key   string `gorm:"primaryKey;size:191" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

func (Setting) TableName() string { return "settings" }

type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ActorID   uint      `gorm:"index" json:"actor_id"`
	ActorName string    `gorm:"size:191" json:"actor_name"`
	Action    string    `gorm:"size:64;index" json:"action"`
	Target    string    `gorm:"size:255" json:"target"`
	Detail    string    `gorm:"size:500" json:"detail"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }
