package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"graycrow/internal/engine"
	"graycrow/internal/httpx"
	"graycrow/internal/middleware"
	"graycrow/internal/model"
	"graycrow/internal/schedule"
	"graycrow/internal/service"
)

type Deps struct {
	DB        *gorm.DB
	Rules     *service.RuleService
	Keys      *service.APIKeyService
	Allows    *service.AllowService
	Channels  *service.ChannelService
	Templates *service.TemplateService
	Users     *service.UserService
	Settings  *service.SettingsService
	Audit     *service.Audit
	Alerts    *service.AlertService
	Stats     *service.StatsService
	Auth      *service.Auth
	Engine    *engine.Engine
}

func (d *Deps) actor(c *gin.Context) *model.User { return middleware.CurrentUser(c) }

func (d *Deps) log(c *gin.Context, action, target, detail string) {
	d.Audit.Record(d.actor(c), action, target, detail, c.ClientIP())
}

// fail 把 service 层错误映射为统一错误码；未知错误一律 5000 且不回显内部细节。
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrRuleNotFound),
		errors.Is(err, service.ErrKeyNotFound),
		errors.Is(err, service.ErrAllowNotFound),
		errors.Is(err, service.ErrChannelNotFound),
		errors.Is(err, service.ErrUserNotFound),
		errors.Is(err, service.ErrAlertNotFound):
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, service.ErrOutOfScope):
		httpx.Fail(c, http.StatusForbidden, httpx.CodeOutOfScope, err.Error())
	case errors.Is(err, service.ErrStatusInvalid),
		errors.Is(err, service.ErrTooManyIDs):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, err.Error())
	case errors.Is(err, service.ErrIntervalTooLow):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeIntervalTooLow, err.Error())
	case errors.Is(err, service.ErrScheduleInvalid):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeScheduleInvalid, err.Error())
	case errors.Is(err, service.ErrRuleDisabled):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeRuleDisabled, err.Error())
	case errors.Is(err, service.ErrUserDuplicate):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeDuplicate, err.Error())
	case errors.Is(err, service.ErrChannelInUse),
		errors.Is(err, service.ErrTemplateInUse),
		errors.Is(err, service.ErrTemplateDefault),
		errors.Is(err, service.ErrLastAdmin):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInUse, err.Error())
	case errors.Is(err, service.ErrTemplateNotFound):
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, service.ErrRuleName),
		errors.Is(err, service.ErrKeywords),
		errors.Is(err, service.ErrQuerySyntax),
		errors.Is(err, service.ErrNameRequired),
		errors.Is(err, service.ErrTokenRequired),
		errors.Is(err, service.ErrRepoFormat),
		errors.Is(err, service.ErrScopeInvalid),
		errors.Is(err, service.ErrChannelType),
		errors.Is(err, service.ErrChannelURL),
		errors.Is(err, service.ErrChannelSign),
		errors.Is(err, service.ErrChannelHeaders),
		errors.Is(err, service.ErrTemplateName),
		errors.Is(err, service.ErrTemplateBody),
		errors.Is(err, service.ErrPasswordShort),
		errors.Is(err, service.ErrSelfOperation),
		errors.Is(err, service.ErrSettingInvalid):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, err.Error())
	default:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "服务器内部错误")
	}
}

func parseUint(raw string) (uint, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	return uint(value), err
}

func idParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeParamFormat, "ID 格式错误")
		return 0, false
	}
	return uint(id), true
}

// ---------- 规则 ----------

type rulePayload struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Keywords      []string `json:"keywords"`
	QueryTemplate string   `json:"query_template"`
	ScheduleType  string   `json:"schedule_type"`
	CronExpr      string   `json:"cron_expr"`
	IntervalSec   int      `json:"interval_sec"`
	Severity      string   `json:"severity"`
	Enabled       *bool    `json:"enabled"`
	DryRun        *bool    `json:"dry_run"`
	RunNow        *bool    `json:"run_now"`
	ViewerIDs     []uint   `json:"viewer_ids"`
	ChannelIDs    []uint   `json:"channel_ids"`
}

func (p rulePayload) toInput() service.RuleInput {
	in := service.RuleInput{
		Name:          p.Name,
		Description:   p.Description,
		Keywords:      p.Keywords,
		QueryTemplate: p.QueryTemplate,
		ScheduleType:  p.ScheduleType,
		CronExpr:      p.CronExpr,
		IntervalSec:   p.IntervalSec,
		Severity:      p.Severity,
		Enabled:       true,
		DryRun:        true,
		RunNow:        p.RunNow,
		ViewerIDs:     p.ViewerIDs,
		ChannelIDs:    p.ChannelIDs,
	}
	if p.Enabled != nil {
		in.Enabled = *p.Enabled
	}
	if p.DryRun != nil {
		in.DryRun = *p.DryRun
	}
	return in
}

func (d *Deps) ListRules(c *gin.Context) {
	items, err := d.Rules.List(d.actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPage(items, int64(len(items)), 1, len(items)+1))
}

func (d *Deps) GetRule(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	rule, err := d.Rules.Get(id, d.actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rule)
}

func (d *Deps) CreateRule(c *gin.Context) {
	var payload rulePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	rule, err := d.Rules.Create(d.actor(c), payload.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "rule.create", "rule#"+strconv.Itoa(int(rule.ID)), rule.Name)
	httpx.OK(c, rule)
}

func (d *Deps) UpdateRule(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload rulePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	rule, err := d.Rules.Update(id, payload.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "rule.update", "rule#"+strconv.Itoa(int(id)), rule.Name)
	httpx.OK(c, rule)
}

func (d *Deps) DeleteRule(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Rules.Delete(id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "rule.delete", "rule#"+strconv.Itoa(int(id)), "")
	httpx.OK(c, gin.H{})
}

// ScanRule 手动触发：只把 next_scan_at 拉到现在，由调度器消费。
func (d *Deps) ScanRule(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Rules.TriggerScan(id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "rule.scan", "rule#"+strconv.Itoa(int(id)), "手动触发")
	httpx.OK(c, gin.H{"queued": true})
}

// PreviewSchedule 只读校验 cron 表达式并给出接下来 3 次运行时间，供规则抽屉实时提示。
func (d *Deps) PreviewSchedule(c *gin.Context) {
	expr := strings.TrimSpace(c.Query("expr"))
	spec, err := schedule.Parse(schedule.TypeCron, expr, 0, d.Rules.Location())
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeScheduleInvalid, "调度表达式不合法："+err.Error())
		return
	}
	now := time.Now().UTC()
	if err := spec.Validate(now, time.Duration(d.Rules.MinInterval())*time.Second); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeScheduleInvalid, "调度表达式不合法："+err.Error())
		return
	}
	next := make([]string, 0, 3)
	at := now
	for i := 0; i < 3; i++ {
		at = spec.Next(at)
		next = append(next, at.In(spec.Location()).Format("2006-01-02 15:04"))
	}
	httpx.OK(c, gin.H{"timezone": spec.Location().String(), "next": next})
}

// DiagnoseRule 输出干跑期统计与建议，供管理员决定放量或收紧关键词。
func (d *Deps) DiagnoseRule(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	out, err := d.Rules.Diagnose(id, d.actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (d *Deps) ListRuleRuns(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if _, err := d.Rules.Get(id, d.actor(c)); err != nil {
		fail(c, err)
		return
	}
	page, size := httpx.Pagination(c)
	if d.Engine == nil {
		httpx.OK(c, httpx.NewPage([]model.ScanRun{}, 0, page, size))
		return
	}
	runs, total, err := d.Engine.Runs(id, page, size)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPage(runs, total, page, size))
}

// ---------- 密钥池 ----------

func (d *Deps) ListKeys(c *gin.Context) {
	keys, err := d.Keys.List()
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, keys)
}

func (d *Deps) CreateKey(c *gin.Context) {
	var payload struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	key, err := d.Keys.Create(d.actor(c), payload.Name, payload.Token)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "apikey.create", "key#"+strconv.Itoa(int(key.ID)), key.Name)
	httpx.OK(c, key)
}

func (d *Deps) UpdateKey(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload struct {
		Name    *string `json:"name"`
		Token   *string `json:"token"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	key, err := d.Keys.Update(id, payload.Name, payload.Enabled, payload.Token)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "apikey.update", "key#"+strconv.Itoa(int(id)), key.Name)
	httpx.OK(c, key)
}

func (d *Deps) DeleteKey(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Keys.Delete(id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "apikey.delete", "key#"+strconv.Itoa(int(id)), "")
	httpx.OK(c, gin.H{})
}

func (d *Deps) KeyQuota(c *gin.Context) {
	pool, err := d.Keys.Quota()
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, pool)
}

func (d *Deps) CheckKey(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	key, err := d.Keys.Check(id)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, key)
}

func (d *Deps) CheckAllKeys(c *gin.Context) {
	n, err := d.Keys.CheckAll()
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "apikey.check", "pool", "全池探活")
	httpx.OK(c, gin.H{"checked": n})
}

func (d *Deps) ReorderKeys(c *gin.Context) {
	var payload struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	if err := d.Keys.Reorder(payload.IDs); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, gin.H{})
}

func (d *Deps) KeyUsage(c *gin.Context) {
	usage, err := d.Keys.Usage()
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, usage)
}

// ---------- 白名单 ----------

func (d *Deps) ListAllowlist(c *gin.Context) {
	rows, err := d.Allows.List(c.Query("scope"), c.Query("repo"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPage(rows, int64(len(rows)), 1, len(rows)+1))
}

func (d *Deps) CreateAllowlist(c *gin.Context) {
	var payload struct {
		Scope        string  `json:"scope"`
		RuleID       *uint   `json:"rule_id"`
		RepoFullName string  `json:"repo_full_name"`
		Reason       string  `json:"reason"`
		ExpiresAt    *string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	var expires *time.Time
	if payload.ExpiresAt != nil && *payload.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, *payload.ExpiresAt)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeParamFormat, "到期时间格式应为 RFC3339")
			return
		}
		expires = &parsed
	}
	row, err := d.Allows.Create(d.actor(c), payload.Scope, payload.RuleID, payload.RepoFullName, payload.Reason, expires)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "allowlist.create", row.RepoFullName, payload.Scope)
	httpx.OK(c, row)
}

func (d *Deps) DeleteAllowlist(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Allows.Delete(id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "allowlist.delete", "allow#"+strconv.Itoa(int(id)), "")
	httpx.OK(c, gin.H{})
}

// ---------- 通知渠道 ----------

func (d *Deps) ListChannels(c *gin.Context) {
	rows, err := d.Channels.List()
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rows)
}

type channelPayload struct {
	ID         uint           `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	TemplateID *uint          `json:"template_id"`
	Config     map[string]any `json:"config"`
	Secret     *string        `json:"secret"`
	Headers    *string        `json:"headers"`
	Enabled    *bool          `json:"enabled"`
}

func (p channelPayload) toInput() service.ChannelInput {
	in := service.ChannelInput{
		ID: p.ID, Name: p.Name, Type: p.Type, TemplateID: p.TemplateID, Config: p.Config,
		Secret: p.Secret, Headers: p.Headers, Enabled: true,
	}
	if p.Enabled != nil {
		in.Enabled = *p.Enabled
	}
	if in.Config == nil {
		in.Config = map[string]any{}
	}
	return in
}

func (d *Deps) CreateChannel(c *gin.Context) {
	var payload channelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	row, err := d.Channels.Create(d.actor(c), payload.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "channel.create", "channel#"+strconv.Itoa(int(row.ID)), row.Name)
	httpx.OK(c, row)
}

func (d *Deps) UpdateChannel(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload channelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	row, err := d.Channels.Update(id, payload.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "channel.update", "channel#"+strconv.Itoa(int(id)), row.Name)
	httpx.OK(c, row)
}

func (d *Deps) DeleteChannel(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Channels.Delete(id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "channel.delete", "channel#"+strconv.Itoa(int(id)), "")
	httpx.OK(c, gin.H{})
}

func (d *Deps) TestChannel(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	result, err := d.Channels.Test(id)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "channel.test", "channel#"+strconv.Itoa(int(id)), result.Response)
	httpx.OK(c, result)
}

// TestChannelDraft 用未保存的表单配置试发，不落库。
func (d *Deps) TestChannelDraft(c *gin.Context) {
	var payload channelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	result, err := d.Channels.TestDraft(payload.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// ---------- 通知模板 ----------

func (d *Deps) ListTemplates(c *gin.Context) {
	rows, err := d.Templates.List()
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rows)
}

type templatePayload struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (p templatePayload) toInput() service.TemplateInput {
	return service.TemplateInput{Name: p.Name, Title: p.Title, Body: p.Body}
}

func (d *Deps) CreateTemplate(c *gin.Context) {
	var payload templatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	row, err := d.Templates.Create(d.actor(c), payload.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "template.create", "template#"+strconv.Itoa(int(row.ID)), row.Name)
	httpx.OK(c, row)
}

func (d *Deps) UpdateTemplate(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload templatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	row, err := d.Templates.Update(id, payload.toInput())
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "template.update", "template#"+strconv.Itoa(int(id)), row.Name)
	httpx.OK(c, row)
}

func (d *Deps) DeleteTemplate(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Templates.Delete(id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "template.delete", "template#"+strconv.Itoa(int(id)), "")
	httpx.OK(c, gin.H{})
}

// ---------- 用户 ----------

func (d *Deps) ListUsers(c *gin.Context) {
	page, size := httpx.Pagination(c)
	users, total, err := d.Users.List(page, size)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPage(users, total, page, size))
}

func (d *Deps) CreateUser(c *gin.Context) {
	var payload struct {
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	user, err := d.Users.Create(service.UserInput{
		Username: payload.Username, Nickname: payload.Nickname, Role: payload.Role, Password: payload.Password,
	})
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "user.create", "user#"+strconv.Itoa(int(user.ID)), user.Username)
	httpx.OK(c, user)
}

func (d *Deps) UpdateUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload struct {
		Nickname string `json:"nickname"`
		Role     string `json:"role"`
		Status   *int8  `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	user, err := d.Users.Update(id, service.UserInput{Nickname: payload.Nickname, Role: payload.Role, Status: payload.Status})
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "user.update", "user#"+strconv.Itoa(int(id)), user.Username)
	httpx.OK(c, user)
}

func (d *Deps) SetUserStatus(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload struct {
		Status int8 `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	user, err := d.Users.Update(id, service.UserInput{Status: &payload.Status})
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "user.status", "user#"+strconv.Itoa(int(id)), strconv.Itoa(int(payload.Status)))
	httpx.OK(c, user)
}

func (d *Deps) ResetUserPassword(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	if err := d.Users.ResetPassword(id, payload.Password); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "user.password", "user#"+strconv.Itoa(int(id)), "重置密码")
	httpx.OK(c, gin.H{})
}

// ResetUserTOTP 管理员为丢失设备的用户重置两步验证。
func (d *Deps) ResetUserTOTP(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Auth.ResetTOTP(id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "user.totp_reset", "user#"+strconv.Itoa(int(id)), "重置两步验证")
	httpx.OK(c, gin.H{})
}

func (d *Deps) DeleteUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := d.Users.Delete(d.actor(c), id); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "user.delete", "user#"+strconv.Itoa(int(id)), "")
	httpx.OK(c, gin.H{})
}

// ---------- 设置与审计 ----------

func (d *Deps) GetSettings(c *gin.Context) {
	values, err := d.Settings.Get()
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, values)
}

func (d *Deps) UpdateSettings(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	if err := d.Settings.Update(payload); err != nil {
		fail(c, err)
		return
	}
	d.log(c, "settings.update", "settings", "")
	values, err := d.Settings.Get()
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, values)
}

func (d *Deps) ListAuditLogs(c *gin.Context) {
	page, size := httpx.Pagination(c)
	query := service.AuditQuery{Action: c.Query("action"), Page: page, Size: size}
	if from, ok := parseDate(c.Query("from")); ok {
		query.From = &from
	}
	if to, ok := parseDate(c.Query("to")); ok {
		end := to.Add(24 * time.Hour)
		query.To = &end
	}
	logs, total, err := d.Audit.List(query)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPage(logs, total, page, size))
}

func parseDate(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
