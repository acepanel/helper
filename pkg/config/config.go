package config

// Config 全局配置
type Config struct {
	Verbose bool
}

// Global 全局配置实例
var Global = &Config{}
