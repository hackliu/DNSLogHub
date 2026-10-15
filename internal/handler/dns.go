package handler

import (
	"net/http"

	"github.com/hackliu/DNSLogHub/internal/dns"
	"github.com/hackliu/DNSLogHub/internal/model"
	"github.com/hackliu/DNSLogHub/internal/service"

	"github.com/gin-gonic/gin"
)

// GetDnsData 获取 DNS 数据
func GetDnsData(c *gin.Context) {
	respondOK(c, model.UserDnsDataMap.Get(c.GetString("token")))
}

// GetDnsDataAndClean 取后即清：原子返回当前记录并清空
func GetDnsDataAndClean(c *gin.Context) {
	token := c.GetString("token")
	data := model.UserDnsDataMap.GetAndClear(token)
	_ = service.ClearDBRecords(token)
	respondOK(c, data)
}

func Clean(c *gin.Context) {
	_ = service.ClearRecords(c.GetString("token"))
	respondOK(c, nil)
}

type VerifyDnsReq struct {
	Query string `json:"query"`
}

func VerifyDns(c *gin.Context) {
	var req VerifyDnsReq
	if err := c.BindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	data := model.UserDnsDataMap.Get(c.GetString("token"))
	for _, v := range data {
		if v.Subdomain == req.Query {
			respondOK(c, map[string]interface{}{
				"subdomain": v.Subdomain,
				"ipaddress": v.Ipaddress,
				"time":      v.Time,
				"type":      v.Type,
			})
			return
		}
	}
	respond(c, http.StatusOK, "Not Found", nil)
}

type BulkVerifyDnsReq struct {
	Subdomain []string `json:"subdomain"`
}

func BulkVerifyDns(c *gin.Context) {
	var req BulkVerifyDnsReq
	if err := c.BindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	data := model.UserDnsDataMap.Get(c.GetString("token"))
	verifyData := make(map[string]struct{})
	for _, s := range req.Subdomain {
		for _, v := range data {
			if s == v.Subdomain {
				verifyData[v.Subdomain] = struct{}{}
			}
		}
	}
	dataList := make([]string, 0, len(verifyData))
	for key := range verifyData {
		dataList = append(dataList, key)
	}
	respondOK(c, dataList)
}

type SetARecordReq struct {
	Domain string `json:"domain"`
	Ip     string `json:"ip"`
}

func SetARecord(c *gin.Context) {
	var req SetARecordReq
	if err := c.BindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	if err := dns.SetARecord(c.GetString("token"), req.Domain, req.Ip); err != nil {
		respond(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	respondOK(c, nil)
}

type SetTXTRecordReq struct {
	Domain string `json:"domain"`
	Txt    string `json:"txt"`
}

func SetTXTRecord(c *gin.Context) {
	var req SetTXTRecordReq
	if err := c.BindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	if req.Domain == "" || req.Txt == "" {
		respond(c, http.StatusBadRequest, "Domain and TXT must not be empty", nil)
		return
	}
	dns.SetTXTRecord(c.GetString("token"), req.Domain, req.Txt)
	respondOK(c, nil)
}
