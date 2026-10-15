package main

import (
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/dns"
	"github.com/hackliu/DNSLogHub/internal/handler"
	"github.com/hackliu/DNSLogHub/internal/i18n"
	"github.com/hackliu/DNSLogHub/internal/logger"
	"github.com/hackliu/DNSLogHub/internal/model"
	"github.com/hackliu/DNSLogHub/internal/router"
	"github.com/hackliu/DNSLogHub/internal/service"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func readConfig() {
	file, err := os.ReadFile("config.yaml")
	if err != nil {
		logger.Logger.Panic("Failed to read config file", zap.Error(err))
	}
	err = yaml.Unmarshal(file, &config.Config)
	if err != nil {
		logger.Logger.Panic("Failed to unmarshal config", zap.Error(err))
	}
}

func main() {
	logger.InitLogger()
	defer logger.Sync()
	logger.Logger.Info("DNSLogHUB starting", zap.String("version", version), zap.String("commit", commit), zap.String("buildTime", buildTime))
	readConfig()
	config.Config.Normalize()
	logger.InitAuditLogger()
	i18n.Init()
	service.ReloadOnSignal()
	model.UserDnsDataMap = model.NewDnsDataMap(config.Config.User)
	if err := service.InitStore(config.Config.Data.DbFile); err != nil {
		logger.Logger.Panic("init store error", zap.Error(err))
	}
	service.StartPersistence(time.Duration(config.Config.Data.FlushInterval) * time.Second)
	go dns.ListingDnsServer()
	gin.SetMode(gin.ReleaseMode)
	r := router.SetupRouter()
	logger.Logger.Info("Http Server start...")
	err := handler.MultiProtocolListener(r, config.Config.Http.Port)
	if err != nil {
		logger.Logger.Panic("Failed to start server", zap.Error(err))
	}
}
