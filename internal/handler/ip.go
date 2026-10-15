package handler

import (
	"net/http"

	"github.com/hackliu/DNSLogHub/internal/ipwry"

	"github.com/gin-gonic/gin"
)

// Query IP 归属地查询
func Query(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		ip = c.ClientIP()
	}
	result, err := ipwry.Query(ip)
	if err != nil {
		respond(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	respondOK(c, gin.H{"ip": ip, "location": result})
}
