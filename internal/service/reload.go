package service

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/logger"

	"go.uber.org/zap"
)

// ReloadOnSignal 监听 SIGHUP 触发配置热加载（Windows 上主要靠管理接口触发）。
func ReloadOnSignal() {
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGHUP)
		for range ch {
			if err := config.Config.Reload(); err != nil {
				logger.Logger.Error("config reload error", zap.Error(err))
			} else {
				logger.Audit("config.reload.success", zap.String("trigger", "sighup"))
			}
		}
	}()
}
