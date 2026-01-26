package config

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Config 全局配置
type Config struct {
	Verbose   bool
	LogFile   string
	logWriter *os.File
	logMu     sync.Mutex
}

// Global 全局配置实例
var Global = &Config{}

// InitLogFile 初始化日志文件
func (c *Config) InitLogFile() error {
	if c.LogFile == "" {
		return nil
	}
	f, err := os.OpenFile(c.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	c.logWriter = f
	return nil
}

// CloseLogFile 关闭日志文件
func (c *Config) CloseLogFile() {
	if c.logWriter != nil {
		_ = c.logWriter.Close()
	}
}

// WriteLog 写日志
func (c *Config) WriteLog(format string, args ...any) {
	if c.logWriter == nil {
		return
	}
	c.logMu.Lock()
	defer c.logMu.Unlock()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(c.logWriter, "[%s] %s\n", timestamp, msg)
}
