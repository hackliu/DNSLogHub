package handler

import (
	"net/http"

	"github.com/hackliu/DNSLogHub/internal/model"

	"github.com/gin-gonic/gin"
)

type VerifyHttpReq struct {
	Query string `json:"query"`
}

func VerifyHttp(c *gin.Context) {
	var req VerifyHttpReq
	if err := c.BindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	token := c.GetString("token")
	for _, v := range model.UserDnsDataMap.Get(token) {
		if v.Subdomain == req.Query && v.Type == model.Http {
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

type BulkVerifyHttpReq struct {
	Query []string `json:"query"`
}

func BulkVerifyHttp(c *gin.Context) {
	var req BulkVerifyHttpReq
	if err := c.BindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	token := c.GetString("token")
	var respData []string
	for _, v := range model.UserDnsDataMap.Get(token) {
		for _, s := range req.Query {
			if v.Subdomain == s && v.Type == model.Http {
				respData = append(respData, s)
			}
		}
	}
	respondOK(c, respData)
}
