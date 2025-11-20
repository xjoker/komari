package admin

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/tasks"
)

// isPrivateIP 检查是否为内网地址（防止 SSRF）
func isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 检查是否为私有 IP 地址范围
	privateIPBlocks := []string{
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"127.0.0.0/8",    // Loopback
		"169.254.0.0/16", // Link-local
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local addr
		"fe80::/10",      // IPv6 link-local
	}

	for _, cidr := range privateIPBlocks {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// validateTarget 验证 target 不是内网地址（防止 SSRF 漏洞）
func validateTarget(target string) error {
	// 如果是 URL，提取主机名
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		parsedURL, err := url.Parse(target)
		if err != nil {
			return err
		}
		target = parsedURL.Hostname()
	}

	// 解析主机名或 IP
	ips, err := net.LookupIP(target)
	if err != nil {
		// 如果无法解析，尝试直接解析为 IP
		if ip := net.ParseIP(target); ip != nil {
			if isPrivateIP(target) {
				return &net.OpError{Op: "validate", Net: "ip", Err: &net.DNSError{Err: "target cannot be a private IP address", Name: target, IsNotFound: true}}
			}
			return nil
		}
		return err
	}

	// 检查所有解析的 IP 是否为内网地址
	for _, ip := range ips {
		if isPrivateIP(ip.String()) {
			return &net.OpError{Op: "validate", Net: "ip", Err: &net.DNSError{Err: "target resolves to a private IP address", Name: target, IsNotFound: true}}
		}
	}

	return nil
}

// POST body: clients []string, target, task_type string, interval int
func AddPingTask(c *gin.Context) {
	var req struct {
		Clients  []string `json:"clients" binding:"required"`
		Name     string   `json:"name" binding:"required"`
		Target   string   `json:"target" binding:"required"`
		TaskType string   `json:"type" binding:"required"`     // icmp, tcp, http
		Interval int      `json:"interval" binding:"required"` // 间隔时间，单位秒
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 验证 clients 数组不为空
	if len(req.Clients) == 0 {
		api.RespondError(c, http.StatusBadRequest, "At least one client is required")
		return
	}

	// 验证 target 不是内网地址（防止 SSRF）
	if err := validateTarget(req.Target); err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid target: cannot target private IP addresses or internal networks")
		return
	}

	// 验证 interval 范围
	if req.Interval < 10 || req.Interval > 3600 {
		api.RespondError(c, http.StatusBadRequest, "Interval must be between 10 and 3600 seconds")
		return
	}

	// 验证 task type
	validTypes := map[string]bool{"icmp": true, "tcp": true, "http": true}
	if !validTypes[req.TaskType] {
		api.RespondError(c, http.StatusBadRequest, "Invalid task type: must be icmp, tcp, or http")
		return
	}

	if taskID, err := tasks.AddPingTask(req.Clients, req.Name, req.Target, req.TaskType, req.Interval); err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
	} else {
		api.RespondSuccess(c, gin.H{"task_id": taskID})
	}
}

// POST body: id []uint
func DeletePingTask(c *gin.Context) {
	var req struct {
		ID []uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := tasks.DeletePingTask(req.ID); err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
	} else {
		api.RespondSuccess(c, nil)
	}
}

// POST body: id []uint, updates map[string]interface{}
func EditPingTask(c *gin.Context) {
	var req struct {
		Tasks []*models.PingTask `json:"tasks" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid request data")
		return
	}

	if err := tasks.EditPingTask(req.Tasks); err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
	} else {
		// for _, task := range req.Tasks {
		// 	tasks.DeletePingRecords([]uint{task.Id})
		// }
		api.RespondSuccess(c, nil)
	}
}

func GetAllPingTasks(c *gin.Context) {
	tasks, err := tasks.GetAllPingTasks()
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondSuccess(c, tasks)
}
