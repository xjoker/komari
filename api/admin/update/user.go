package update

import (
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/accounts"
	"github.com/komari-monitor/komari/database/auditlog"
)

func UpdateUser(c *gin.Context) {
	var req struct {
		Uuid     string  `json:"uuid" binding:"required"`
		Name     *string `json:"username"`
		Password *string `json:"password"`
		SsoType  *string `json:"sso_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, 400, "Invalid or missing request body: "+err.Error())
		return
	}
	if req.Password == nil && req.Name == nil {
		api.RespondError(c, 400, "At least one field (username or password) must be provided")
		return
	}
	if req.Name != nil {
		if len(*req.Name) < 3 {
			api.RespondError(c, 400, "Username must be at least 3 characters long")
			return
		}
		if len(*req.Name) > 50 {
			api.RespondError(c, 400, "Username must not exceed 50 characters")
			return
		}
	}
	if req.Password != nil {
		// 强化密码策略：最少12位，包含大小写字母、数字
		if len(*req.Password) < 12 {
			api.RespondError(c, 400, "Password must be at least 12 characters long")
			return
		}
		if len(*req.Password) > 128 {
			api.RespondError(c, 400, "Password must not exceed 128 characters")
			return
		}
		// 检查是否包含大写字母、小写字母和数字
		hasUpper := false
		hasLower := false
		hasDigit := false
		for _, char := range *req.Password {
			if char >= 'A' && char <= 'Z' {
				hasUpper = true
			} else if char >= 'a' && char <= 'z' {
				hasLower = true
			} else if char >= '0' && char <= '9' {
				hasDigit = true
			}
		}
		if !hasUpper || !hasLower || !hasDigit {
			api.RespondError(c, 400, "Password must contain at least one uppercase letter, one lowercase letter, and one digit")
			return
		}
	}
	if err := accounts.UpdateUser(req.Uuid, req.Name, req.Password, req.SsoType); err != nil {
		api.RespondError(c, 500, "Failed to update user: "+err.Error())
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "User updated", "warn")
	api.RespondSuccess(c, gin.H{"uuid": req.Uuid})
}
