package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/crypto"
	"graycrow/internal/model"
	"graycrow/internal/notify"
)

var (
	ErrChannelNotFound = errors.New("渠道不存在")
	ErrChannelType     = errors.New("不支持的渠道类型")
	ErrChannelInUse    = errors.New("该渠道仍被规则引用，请先解绑")
	ErrChannelURL      = errors.New("请填写有效的 Webhook 地址")
	ErrChannelSign     = errors.New("开启加签时必须填写签名密钥")
	ErrChannelHeaders  = errors.New("自定义 Header 需为 JSON 对象")
)

var channelTypes = map[string]bool{"feishu": true, "wecom": true, "dingtalk": true, "webhook": true, "smtp": true}

// channelSecrets 加密存储的敏感值；v=1 起为 JSON，旧数据是裸密码或 Header JSON，读时兼容。
type channelSecrets struct {
	Version    int               `json:"v"`
	URL        string            `json:"url,omitempty"`
	SignSecret string            `json:"sign_secret,omitempty"`
	Password   string            `json:"password,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

type ChannelService struct {
	db *gorm.DB
}

func NewChannelService(db *gorm.DB) *ChannelService { return &ChannelService{db: db} }

func (s *ChannelService) List() ([]model.Channel, error) {
	rows := []model.Channel{}
	if err := s.db.Order("id asc").Find(&rows).Error; err != nil {
		return rows, err
	}
	for i := range rows {
		maskChannel(&rows[i])
	}
	return rows, nil
}

type ChannelInput struct {
	ID         uint
	Name       string
	Type       string
	TemplateID *uint
	Config     map[string]any
	Secret     *string
	Headers    *string
	Enabled    bool
}

func (s *ChannelService) Create(actor *model.User, in ChannelInput) (*model.Channel, error) {
	if !channelTypes[in.Type] {
		return nil, ErrChannelType
	}
	if in.Name == "" {
		return nil, ErrNameRequired
	}
	if err := s.checkTemplate(in.TemplateID); err != nil {
		return nil, err
	}
	row := &model.Channel{
		Name:       in.Name,
		Type:       in.Type,
		TemplateID: in.TemplateID,
		Enabled:    in.Enabled,
		CreatedBy:  actor.ID,
	}
	if err := s.storeConfig(row, in, nil); err != nil {
		return nil, err
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}
	maskChannel(row)
	return row, nil
}

func (s *ChannelService) Update(id uint, in ChannelInput) (*model.Channel, error) {
	row, err := s.get(id)
	if err != nil {
		return nil, err
	}
	if !channelTypes[in.Type] {
		return nil, ErrChannelType
	}
	if in.Name == "" {
		return nil, ErrNameRequired
	}
	if err := s.checkTemplate(in.TemplateID); err != nil {
		return nil, err
	}
	row.Name, row.Type, row.TemplateID, row.Enabled = in.Name, in.Type, in.TemplateID, in.Enabled
	if err := s.storeConfig(row, in, row); err != nil {
		return nil, err
	}
	// 结构体 + Select 更新，保证 config 走 serializer:json
	updates := model.Channel{Name: row.Name, Type: row.Type, TemplateID: row.TemplateID, Config: row.Config, SecretEnc: row.SecretEnc, Enabled: row.Enabled}
	if err := s.db.Model(&model.Channel{ID: id}).Select("name", "type", "template_id", "config", "secret_enc", "enabled").Updates(updates).Error; err != nil {
		return nil, err
	}
	updated, err := s.get(id)
	if err != nil {
		return nil, err
	}
	maskChannel(updated)
	return updated, nil
}

func (s *ChannelService) Delete(id uint) error {
	if _, err := s.get(id); err != nil {
		return err
	}
	var count int64
	s.db.Model(&model.RuleChannel{}).Where("channel_id = ?", id).Count(&count)
	if count > 0 {
		return ErrChannelInUse
	}
	return s.db.Delete(&model.Channel{}, id).Error
}

type TestResult struct {
	OK       bool   `json:"ok"`
	Response string `json:"response"`
}

func (s *ChannelService) Test(id uint) (*TestResult, error) {
	row, err := s.get(id)
	if err != nil {
		return nil, err
	}
	return s.testRow(row)
}

// TestDraft 用未保存的表单配置试发，便于先验证再提交；带 ID 时沿用已存的地址与密钥。
func (s *ChannelService) TestDraft(in ChannelInput) (*TestResult, error) {
	if !channelTypes[in.Type] {
		return nil, ErrChannelType
	}
	var existing *model.Channel
	if in.ID > 0 {
		row, err := s.get(in.ID)
		if err != nil {
			return nil, err
		}
		existing = row
	}
	row := &model.Channel{Type: in.Type, TemplateID: in.TemplateID, Enabled: true}
	if err := s.storeConfig(row, in, existing); err != nil {
		return nil, err
	}
	return s.testRow(row)
}

func (s *ChannelService) testRow(row *model.Channel) (*TestResult, error) {
	sample := notify.Batch{
		Title: "测试规则",
		Items: []notify.BatchItem{{
			Repo:     "acme/backend",
			Path:     "config/app.yaml",
			Detector: "generic-password",
			Keyword:  "acme.com",
			URL:      "https://github.com/acme/backend/blob/main/config/app.yaml",
		}},
	}
	ok, message := s.deliver(row, sample.Text(), sample.Vars(), true)
	return &TestResult{OK: ok, Response: message}, nil
}

// Broadcast 把平台级通知（如密钥池耗尽）发往启用渠道；channelIDs 为空表示全部启用渠道。
func (s *ChannelService) Broadcast(p notify.Payload, channelIDs []uint) []string {
	rows, err := s.Enabled(channelIDs)
	if err != nil {
		return []string{err.Error()}
	}
	failures := []string{}
	for i := range rows {
		if ok, message := s.deliver(&rows[i], p.Text(), p.Vars(), false); !ok {
			failures = append(failures, rows[i].Name+": "+message)
		}
	}
	return failures
}

// Send 单次投递，返回上游状态码与响应体；不落库、不改渠道状态，由调用方决定重试与记录。
// digest 为 true 时优先使用渠道绑定的汇总模板。
func (s *ChannelService) Send(row *model.Channel, text string, vars map[string]string, digest bool) (int, string, error) {
	msg := notify.Message{Text: text}
	jsonMode := row.Type == "webhook" && (configString(row.Config, "body_format") == "" || configString(row.Config, "body_format") == "json")
	rendered := false
	if digest {
		// 未绑定模板时用数据库里的默认模板；JSON webhook 没显式绑定则沿用旧的变量 JSON 正文
		if tpl := s.pickTemplate(row.TemplateID); tpl != nil && !(jsonMode && row.TemplateID == nil) {
			msg.Text = notify.Render(tpl.Body, vars, jsonMode)
			if tpl.Title != "" {
				msg.Subject = notify.Render(tpl.Title, vars, false)
			}
			rendered = true
		}
	}
	if jsonMode && !rendered {
		msg.Text = ""
	}
	return notify.Send(row, s.credentials(row), msg, vars)
}

// pickTemplate 取渠道绑定模板；未绑定时回落到数据库里的默认模板。
func (s *ChannelService) pickTemplate(id *uint) *model.NotifyTemplate {
	tpl := model.NotifyTemplate{}
	tx := s.db
	if id != nil {
		tx = tx.Where("id = ?", *id)
	} else {
		tx = tx.Where("is_default = ?", true)
	}
	if err := tx.First(&tpl).Error; err != nil || strings.TrimSpace(tpl.Body) == "" {
		return nil
	}
	return &tpl
}

// Enabled 返回全部启用渠道；ids 非空时只取这些（规则绑定）。
func (s *ChannelService) Enabled(ids []uint) ([]model.Channel, error) {
	rows := []model.Channel{}
	tx := s.db.Where("enabled = ?", true)
	if len(ids) > 0 {
		tx = tx.Where("id IN ?", ids)
	}
	err := tx.Order("id asc").Find(&rows).Error
	return rows, err
}

func (s *ChannelService) deliver(row *model.Channel, text string, vars map[string]string, digest bool) (bool, string) {
	status, body, err := s.Send(row, text, vars, digest)
	now := time.Now().UTC()
	updates := map[string]any{"last_sent_at": now}
	ok, message := true, body
	switch {
	case err != nil:
		ok, message = false, err.Error()
		updates["last_status"] = "failed"
		updates["last_error"] = message
	case status >= 400:
		ok, message = false, "HTTP "+strconv.Itoa(status)+": "+body
		updates["last_status"] = "failed"
		updates["last_error"] = message
	default:
		updates["last_status"] = "success"
		updates["last_error"] = ""
	}
	if row.ID > 0 {
		if err := s.db.Model(row).Updates(updates).Error; err != nil {
			log.Printf("更新渠道 %d 投递状态失败: %v", row.ID, err)
		}
	}
	return ok, message
}

func (s *ChannelService) get(id uint) (*model.Channel, error) {
	var row model.Channel
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, ErrChannelNotFound
	}
	return &row, nil
}

func (s *ChannelService) checkTemplate(id *uint) error {
	if id == nil {
		return nil
	}
	var count int64
	if err := s.db.Model(&model.NotifyTemplate{}).Where("id = ?", *id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// storeConfig 拆分明文配置与加密敏感值，并把 Webhook 地址从 config 移到 secret_enc。
func (s *ChannelService) storeConfig(row *model.Channel, in ChannelInput, existing *model.Channel) error {
	cfg := map[string]any{}
	if in.Config == nil && existing != nil {
		for k, v := range existing.Config {
			cfg[k] = v
		}
	}
	for k, v := range in.Config {
		cfg[k] = v
	}
	sec := channelSecrets{Version: 1}
	if existing != nil {
		sec = s.loadSecrets(existing)
		// 旧渠道地址还在 config 里，迁移到密文
		if in.Type != "smtp" && sec.URL == "" {
			sec.URL = configString(existing.Config, "url")
			if sec.URL == "" {
				sec.URL = configString(existing.Config, "webhook")
			}
		}
	}
	if in.Secret != nil && *in.Secret != "" {
		if in.Type == "smtp" {
			sec.Password = *in.Secret
		} else {
			sec.SignSecret = *in.Secret
		}
	}
	if in.Headers != nil && strings.TrimSpace(*in.Headers) != "" {
		headers := map[string]string{}
		if err := json.Unmarshal([]byte(*in.Headers), &headers); err != nil {
			return ErrChannelHeaders
		}
		sec.Headers = headers
	}
	if in.Type != "smtp" {
		// 兼容旧前端把地址放在 url / webhook 字段
		if v := configString(in.Config, "url"); v != "" {
			sec.URL = v
		} else if v := configString(in.Config, "webhook"); v != "" {
			sec.URL = v
		}
		if u, err := url.Parse(sec.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return ErrChannelURL
		}
		if configString(cfg, "sign_mode") != "" && sec.SignSecret == "" {
			return ErrChannelSign
		}
		delete(cfg, "url")
		delete(cfg, "webhook")
		cfg["url_hint"] = maskURL(sec.URL)
	} else {
		delete(cfg, "url_hint")
		delete(cfg, "sign_mode")
	}
	row.Config = cfg
	enc, err := crypto.Encrypt(mustJSON(sec))
	if err != nil {
		return err
	}
	row.SecretEnc = enc
	return nil
}

func (s *ChannelService) loadSecrets(row *model.Channel) channelSecrets {
	sec := channelSecrets{Version: 1}
	if row == nil || row.SecretEnc == "" {
		return sec
	}
	raw, err := crypto.Decrypt(row.SecretEnc)
	if err != nil || raw == "" {
		return sec
	}
	if err := json.Unmarshal([]byte(raw), &sec); err == nil && sec.Version > 0 {
		return sec
	}
	// 旧格式：非 SMTP 的 secret 是 Header JSON，SMTP 是密码
	sec = channelSecrets{Version: 1}
	if row.Type == "smtp" {
		sec.Password = raw
	} else {
		_ = json.Unmarshal([]byte(raw), &sec.Headers)
	}
	return sec
}

func (s *ChannelService) credentials(row *model.Channel) notify.Credentials {
	sec := s.loadSecrets(row)
	creds := notify.Credentials{URL: sec.URL, SignSecret: sec.SignSecret, Headers: sec.Headers, Password: sec.Password}
	if creds.URL == "" {
		creds.URL = configString(row.Config, "url")
	}
	if creds.URL == "" {
		creds.URL = configString(row.Config, "webhook")
	}
	return creds
}

// maskChannel 返回给前端前把地址打码，密文里的完整地址不出网。
func maskChannel(row *model.Channel) {
	if row.Config == nil {
		return
	}
	for _, key := range []string{"url", "webhook"} {
		if v, ok := row.Config[key].(string); ok && v != "" {
			row.Config[key] = maskURL(v)
		}
	}
}

func maskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "已配置"
	}
	base := u.Scheme + "://" + u.Host + u.Path
	if u.RawQuery != "" {
		q := u.Query()
		keys := make([]string, 0, len(q))
		for key := range q {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"=****")
		}
		return base + "?" + strings.Join(parts, "&")
	}
	path := strings.TrimSuffix(u.Path, "/")
	if i := strings.LastIndex(path, "/"); i > 0 {
		return u.Scheme + "://" + u.Host + path[:i] + "/****"
	}
	return u.Scheme + "://" + u.Host + "/****"
}

func configString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	s, _ := cfg[key].(string)
	return s
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{}")
	}
	return string(raw)
}
