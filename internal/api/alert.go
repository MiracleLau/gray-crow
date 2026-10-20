package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"graycrow/internal/httpx"
	"graycrow/internal/service"
)

func (d *Deps) alertQuery(c *gin.Context) service.AlertQuery {
	page, size := httpx.Pagination(c)
	query := service.AlertQuery{
		Status:  c.Query("status"),
		Repo:    c.Query("repo"),
		Keyword: c.Query("keyword"),
		Q:       c.Query("q"),
		Page:    page,
		Size:    size,
	}
	if raw := c.Query("rule_id"); raw != "" {
		if id, err := parseUint(raw); err == nil {
			query.RuleID = &id
		}
	}
	if from, ok := parseDate(c.Query("from")); ok {
		query.From = &from
	}
	if to, ok := parseDate(c.Query("to")); ok {
		end := to.Add(24 * time.Hour)
		query.To = &end
	}
	return query
}

func (d *Deps) ListAlerts(c *gin.Context) {
	query := d.alertQuery(c)
	rows, total, err := d.Alerts.List(d.actor(c), query)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPage(rows, total, query.Page, query.Size))
}

func (d *Deps) GetAlert(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	alert, err := d.Alerts.Get(d.actor(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, alert)
}

func (d *Deps) SetAlertStatus(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	alert, err := d.Alerts.SetStatus(d.actor(c), id, payload.Status, payload.Note)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "alert.status", "alert#"+c.Param("id"), payload.Status)
	httpx.OK(c, alert)
}

func (d *Deps) BulkAlertStatus(c *gin.Context) {
	var payload struct {
		IDs    []uint `json:"ids"`
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return
	}
	updated, skipped, err := d.Alerts.BulkStatus(d.actor(c), payload.IDs, payload.Status, payload.Note)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "alert.bulk", "alerts", payload.Status)
	httpx.OK(c, gin.H{"updated": updated, "skipped": skipped})
}

func (d *Deps) AllowlistAlert(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var payload struct {
		Scope     string  `json:"scope"`
		Reason    string  `json:"reason"`
		ExpiresAt *string `json:"expires_at"`
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
	alert, err := d.Alerts.AllowlistAlert(d.actor(c), id, payload.Scope, payload.Reason, expires)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "alert.allowlist", "alert#"+c.Param("id"), alert.RepoFullName+" / "+payload.Scope)
	httpx.OK(c, alert)
}

func (d *Deps) ListNotifyRecords(c *gin.Context) {
	page, size := httpx.Pagination(c)
	query := service.NotifyQuery{Status: c.Query("status"), Page: page, Size: size}
	if raw := c.Query("rule_id"); raw != "" {
		if id, err := parseUint(raw); err == nil {
			query.RuleID = &id
		}
	}
	if raw := c.Query("channel_id"); raw != "" {
		if id, err := parseUint(raw); err == nil {
			query.ChannelID = &id
		}
	}
	rows, total, err := d.Alerts.NotifyRecords(d.actor(c), query)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPage(rows, total, page, size))
}

// DeleteAlert 单条物理删除（路由仅 admin）。
func (d *Deps) DeleteAlert(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	n, err := d.Alerts.DeleteAlerts([]uint{id})
	if err != nil {
		fail(c, err)
		return
	}
	if n == 0 {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "告警不存在")
		return
	}
	d.log(c, "alert.delete", "alert#"+c.Param("id"), "")
	httpx.OK(c, gin.H{"deleted": n})
}

func (d *Deps) BulkDeleteAlerts(c *gin.Context) {
	ids, ok := bulkIDs(c)
	if !ok {
		return
	}
	n, err := d.Alerts.DeleteAlerts(ids)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "alert.delete", "alerts", fmt.Sprintf("批量删除 %d 条", n))
	httpx.OK(c, gin.H{"deleted": n})
}

func (d *Deps) DeleteNotifyRecord(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	n, err := d.Alerts.DeleteNotifyRecords([]uint{id})
	if err != nil {
		fail(c, err)
		return
	}
	if n == 0 {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "通知记录不存在")
		return
	}
	d.log(c, "notify.delete", "notify#"+c.Param("id"), "")
	httpx.OK(c, gin.H{"deleted": n})
}

func (d *Deps) BulkDeleteNotifyRecords(c *gin.Context) {
	ids, ok := bulkIDs(c)
	if !ok {
		return
	}
	n, err := d.Alerts.DeleteNotifyRecords(ids)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "notify.delete", "notify-records", fmt.Sprintf("批量删除 %d 条", n))
	httpx.OK(c, gin.H{"deleted": n})
}

func bulkIDs(c *gin.Context) ([]uint, bool) {
	var payload struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || len(payload.IDs) == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "参数格式错误")
		return nil, false
	}
	return payload.IDs, true
}

func (d *Deps) RenotifyAlert(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	okCount, failed, err := d.Alerts.Renotify(d.actor(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	d.log(c, "alert.renotify", "alert#"+c.Param("id"), "")
	httpx.OK(c, gin.H{"ok": failed == 0, "delivered": okCount, "failed": failed})
}
