package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// windowLimiter 滑动窗口限流：统计窗口内调用次数，超过 max 则拒绝。
// max 在每次调用时传入（读取实时配置），0 表示不限。
type windowLimiter struct {
	mu    sync.Mutex
	win   int64 // 窗口秒数
	table map[string][]int64
	calls int64 // 用于周期性清理
}

func newWindowLimiter(winSec int) *windowLimiter {
	if winSec <= 0 {
		winSec = 60
	}
	return &windowLimiter{win: int64(winSec), table: map[string][]int64{}}
}

// allow 返回该 key 在当前窗口内是否允许新的一次调用
func (l *windowLimiter) allow(key string, max int) bool {
	if max <= 0 {
		return true
	}
	now := time.Now().Unix()
	cutoff := now - l.win

	l.mu.Lock()
	defer l.mu.Unlock()

	sl := l.table[key]
	// 剪掉窗口外的旧时间戳
	i := 0
	for i < len(sl) && sl[i] < cutoff {
		i++
	}
	sl = sl[i:]

	allowed := len(sl) < max
	if allowed {
		l.table[key] = append(sl, now)
	} else {
		l.table[key] = sl
	}

	// 周期性清理空 key，防止 map 无限增长
	l.calls++
	if l.calls%1000 == 0 {
		for k, v := range l.table {
			if len(v) == 0 {
				delete(l.table, k)
			}
		}
	}
	return allowed
}

var (
	uploadLimiter   = newWindowLimiter(60)
	downloadLimiter = newWindowLimiter(60)
)

// clientIP 取客户端 IP（默认 RemoteAddr；如需反向代理透传可启用 X-Forwarded-For）
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host
}
