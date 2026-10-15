package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/hackliu/DNSLogHub/internal/ipwry"
	"github.com/hackliu/DNSLogHub/internal/middleware"
	"github.com/hackliu/DNSLogHub/internal/model"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构。
//   - code: 业务码（与 HTTP 状态码一致：成功 200，客户端错误 4xx，服务端错误 5xx）
//   - msg: 兼容旧客户端（弃用中）；message 为统一消息字段
//   - traceId: 请求标识，便于关联审计日志
type Response struct {
	Code    int         `json:"code"`
	Msg     string      `json:"msg"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	TraceID string      `json:"traceId,omitempty"`
}

var SUCCESS = "success"

// respond 统一 JSON 响应：HTTP 状态码与业务 code 一致。
func respond(c *gin.Context, status int, msg string, data interface{}) {
	c.JSON(status, Response{
		Code:    status,
		Msg:     msg,
		Message: msg,
		Data:    data,
		TraceID: middleware.GetRequestID(c),
	})
}

// respondOK 成功响应（HTTP 200）。
func respondOK(c *gin.Context, data interface{}) {
	respond(c, http.StatusOK, SUCCESS, data)
}

func SafeDumpRequest(c *gin.Context) ([]byte, error) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	dump, err := httputil.DumpRequest(c.Request, true)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return dump, err
}

func NoRoute(c *gin.Context) {
	subDoamin := strings.Split(c.Request.URL.Path, "/")
	if len(subDoamin) < 2 {
		respond(c, http.StatusNotFound, "404 Not Found", nil)
		return
	}
	token, ok := model.UserDnsDataMap.GetTokenBySubdomain(subDoamin[1])
	if ok {
		dump, err := SafeDumpRequest(c)
		if err != nil {
			respond(c, http.StatusInternalServerError, "Internal Server Error", nil)
			return
		}
		IpLocation, _ := ipwry.Query(c.ClientIP())
		model.UserDnsDataMap.Set(token, model.DnsInfo{
			Type:       model.Http,
			Subdomain:  c.Request.URL.Path,
			Ipaddress:  c.ClientIP(),
			Time:       time.Now().Unix(),
			IpLocation: IpLocation,
			Request:    string(dump),
		})
	}
	respond(c, http.StatusNotFound, "404 Not Found", nil)
}
