package httpx

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	CodeOK              = 0
	CodeInvalidParam    = 1001
	CodeParamFormat     = 1002
	CodeIntervalTooLow  = 1003
	CodeScheduleInvalid = 1004
	CodeRuleDisabled    = 1005
	CodeUnauthorized    = 2001
	CodeBadCredential   = 2002
	CodeUserDisabled    = 2003
	CodeTooManyAttempt  = 2004
	CodeForbidden       = 3001
	CodeOutOfScope      = 3002
	CodeCSRF            = 3003
	CodeNotFound        = 4001
	CodeDuplicate       = 4002
	CodeInUse           = 4003
	CodeInternal        = 5000
)

type body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, body{Code: CodeOK, Message: "ok", Data: data})
}

func Fail(c *gin.Context, httpStatus, code int, msg string) {
	c.AbortWithStatusJSON(httpStatus, body{Code: code, Message: msg})
}

type Page struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func NewPage(items any, total int64, page, pageSize int) Page {
	return Page{Items: items, Total: total, Page: page, PageSize: pageSize}
}

func Pagination(c *gin.Context) (page, pageSize int) {
	page = queryInt(c, "page", 1)
	pageSize = queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return
}

func queryInt(c *gin.Context, key string, def int) int {
	n, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return def
	}
	return n
}
