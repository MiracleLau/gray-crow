package model

import "time"

const (
	AlertPending       = "pending"
	AlertResolved      = "resolved"
	AlertFalsePositive = "false_positive"
	AlertIgnored       = "ignored"
	AlertAllowlisted   = "allowlisted"
)

type Alert struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	RuleID         uint       `gorm:"index" json:"rule_id"`
	RuleName       string     `gorm:"-" json:"rule_name"`
	RepoFullName   string     `gorm:"size:191;index" json:"repo_full_name"`
	RepoURL        string     `gorm:"size:500" json:"repo_url"`
	HTMLURL        string     `gorm:"size:500" json:"html_url"`
	Path           string     `gorm:"size:500" json:"path"`
	MatchedKeyword string     `gorm:"size:191" json:"matched_keyword"`
	DetectorID     string     `gorm:"size:64" json:"detector_id"`
	Snippet        string     `gorm:"type:text" json:"snippet"`
	Status         string     `gorm:"size:16;not null;default:pending;index" json:"status"`
	Fingerprint    string     `gorm:"size:64;uniqueIndex" json:"-"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	NotifiedAt     *time.Time `json:"notified_at"`
	HandledBy      *uint      `json:"-"`
	HandledByName  string     `gorm:"-" json:"handled_by_name"`
	HandledAt      *time.Time `json:"handled_at"`
	HandleSeconds  int        `json:"-"`
	Note           string     `gorm:"size:500" json:"note"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	Events  []AlertEvent `gorm:"-" json:"events,omitempty"`
	Allowed bool         `gorm:"-" json:"allowed"`
}

func (Alert) TableName() string { return "alerts" }

type AlertEvent struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AlertID    uint      `gorm:"index" json:"-"`
	ActorID    uint      `json:"-"`
	ActorName  string    `gorm:"size:191" json:"actor_name"`
	Action     string    `gorm:"size:32" json:"action"`
	FromStatus string    `gorm:"size:16" json:"from_status,omitempty"`
	ToStatus   string    `gorm:"size:16" json:"to_status,omitempty"`
	Note       string    `gorm:"size:500" json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}

func (AlertEvent) TableName() string { return "alert_events" }

const (
	RunSuccess = "success"
	RunFailed  = "failed"
	RunPartial = "partial"
)

type ScanRun struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	RuleID              uint       `gorm:"index" json:"rule_id"`
	APIKeyID            uint       `json:"-"`
	KeyName             string     `gorm:"-" json:"key_name"`
	Query               string     `gorm:"size:500" json:"query"`
	Status              string     `gorm:"size:16" json:"status"`
	ResultCount         int        `json:"result_count"`
	NewAlertCount       int        `json:"new_alert_count"`
	APICost             int        `json:"-"`
	QuotaRemainingAfter int        `json:"quota_remaining_after"`
	Error               string     `gorm:"size:500" json:"error"`
	StartedAt           time.Time  `gorm:"index" json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at"`
}

func (ScanRun) TableName() string { return "scan_runs" }

// NotifyRecord 一次通知发送的记录：批次通知一行（AlertCount 为条数），手动重发一行（AlertID 非空）。
type NotifyRecord struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	RuleID      uint      `gorm:"index" json:"rule_id"`
	RuleName    string    `gorm:"-" json:"rule_name"`
	AlertID     *uint     `json:"alert_id"`
	ChannelID   uint      `gorm:"index" json:"-"`
	ChannelName string    `gorm:"-" json:"channel_name"`
	AlertCount  int       `gorm:"not null;default:1" json:"alert_count"`
	Status      string    `gorm:"size:16;index" json:"status"`
	HTTPStatus  *int      `json:"http_status"`
	Attempt     int       `json:"attempt"`
	Error       string    `gorm:"size:500" json:"error"`
	Content     string    `gorm:"type:text" json:"content"`
	SentAt      time.Time `gorm:"index" json:"sent_at"`
}

func (NotifyRecord) TableName() string { return "notify_records" }
