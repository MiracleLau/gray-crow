package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"graycrow/internal/httpx"
)

func (d *Deps) StatsOverview(c *gin.Context) {
	overview, err := d.Stats.Overview(d.actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	pool, err := d.Keys.Quota()
	if err == nil {
		overview.PoolRemaining = pool.Pool.Remaining
		overview.PoolLimit = pool.Pool.Limit
		overview.PoolAccounts = pool.Pool.Accounts
	}
	httpx.OK(c, overview)
}

func (d *Deps) StatsTrend(c *gin.Context) {
	points, err := d.Stats.Trend(d.actor(c), c.Query("range"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, points)
}

func (d *Deps) StatsByRule(c *gin.Context) {
	rows, err := d.Stats.ByRule(d.actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rows)
}

func (d *Deps) StatsByStatus(c *gin.Context) {
	rows, err := d.Stats.ByStatus(d.actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rows)
}

func (d *Deps) StatsQuota(c *gin.Context) {
	rows, err := d.Stats.QuotaTrend(c.Query("range"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, rows)
}

// StatsExport 用 Cookie 鉴权直接触发下载；viewer 只拿到自己可见范围的告警。
func (d *Deps) StatsExport(c *gin.Context) {
	if c.Query("type") != "" && c.Query("type") != "alerts" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidParam, "暂只支持 type=alerts")
		return
	}
	query := d.alertQuery(c)
	name := fmt.Sprintf("graycrow-alerts-%s.csv", time.Now().UTC().Format("20060102-150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+name)
	if err := d.Stats.ExportAlerts(d.actor(c), query, c.Writer); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "导出失败")
		return
	}
}
