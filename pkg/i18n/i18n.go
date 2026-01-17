package i18n

import (
	"sync"

	"github.com/leonelquinteros/gotext"
)

var (
	locale *gotext.Locale
	mu     sync.RWMutex
)

// Init 设置全局 locale
func Init(l *gotext.Locale) {
	mu.Lock()
	defer mu.Unlock()
	locale = l
}

// T 获取全局 locale
func T() *gotext.Locale {
	mu.RLock()
	defer mu.RUnlock()
	return locale
}
