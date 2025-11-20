package ws

import (
	"net/http"
	"os"
	"strings"
)

// CheckOrigin 检查 WebSocket 连接的来源（防止 CORS 绕过）
func CheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// 如果没有 Origin 头，检查 Host 头
		return true
	}

	// 获取允许的来源列表
	allowedOrigins := os.Getenv("KOMARI_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		// 如果没有配置，检查 origin 是否与 Host 匹配
		host := r.Host
		return strings.HasSuffix(origin, "://"+host)
	}

	// 检查 origin 是否在白名单中
	origins := strings.Split(allowedOrigins, ",")
	for _, allowed := range origins {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}

	return false
}
