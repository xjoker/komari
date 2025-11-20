package admin

import (
	"database/sql"

	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/config"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/database/tasks"

	"github.com/gin-gonic/gin"
)

// GetSettings 获取自定义配置
func GetSettings(c *gin.Context) {
	cst, err := config.Get()
	if err != nil {
		if err == sql.ErrNoRows {
			//override
			cst = models.Config{Sitename: "Komari"}
			cst.ID = 1
			config.Save(cst)
			api.RespondSuccess(c, cst)
			return
		}
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Internal Server Error: " + err.Error(),
		})
	}
	api.RespondSuccess(c, cst)
}

// EditSettingsRequest 定义允许更新的配置字段（防止批量赋值漏洞）
type EditSettingsRequest struct {
	Sitename                   *string  `json:"sitename"`
	Description                *string  `json:"description"`
	AllowCors                  *bool    `json:"allow_cors"`
	Theme                      *string  `json:"theme"`
	PrivateSite                *bool    `json:"private_site"`
	ApiKey                     *string  `json:"api_key"`
	AutoDiscoveryKey           *string  `json:"auto_discovery_key"`
	ScriptDomain               *string  `json:"script_domain"`
	SendIpAddrToGuest          *bool    `json:"send_ip_addr_to_guest"`
	EulaAccepted               *bool    `json:"eula_accepted"`
	GeoIpEnabled               *bool    `json:"geo_ip_enabled"`
	GeoIpProvider              *string  `json:"geo_ip_provider"`
	NezhaCompatEnabled         *bool    `json:"nezha_compat_enabled"`
	NezhaCompatListen          *string  `json:"nezha_compat_listen"`
	OAuthEnabled               *bool    `json:"o_auth_enabled"`
	OAuthProvider              *string  `json:"o_auth_provider"`
	DisablePasswordLogin       *bool    `json:"disable_password_login"`
	CustomHead                 *string  `json:"custom_head"`
	CustomBody                 *string  `json:"custom_body"`
	NotificationEnabled        *bool    `json:"notification_enabled"`
	NotificationMethod         *string  `json:"notification_method"`
	NotificationTemplate       *string  `json:"notification_template"`
	ExpireNotificationEnabled  *bool    `json:"expire_notification_enabled"`
	ExpireNotificationLeadDays *int     `json:"expire_notification_lead_days"`
	LoginNotification          *bool    `json:"login_notification"`
	TrafficLimitPercentage     *float64 `json:"traffic_limit_percentage"`
	RecordEnabled              *bool    `json:"record_enabled"`
	RecordPreserveTime         *int     `json:"record_preserve_time"`
	PingRecordPreserveTime     *int     `json:"ping_record_preserve_time"`
}

// EditSettings 更新自定义配置
func EditSettings(c *gin.Context) {
	var req EditSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, 400, "Invalid or missing request body: "+err.Error())
		return
	}

	// 构建更新 map，只包含非 nil 字段，并进行验证
	updates := make(map[string]interface{})

	if req.Sitename != nil {
		if len(*req.Sitename) > 100 {
			api.RespondError(c, 400, "Sitename must not exceed 100 characters")
			return
		}
		updates["sitename"] = *req.Sitename
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.AllowCors != nil {
		updates["allow_cors"] = *req.AllowCors
	}
	if req.Theme != nil {
		if len(*req.Theme) > 100 {
			api.RespondError(c, 400, "Theme name must not exceed 100 characters")
			return
		}
		updates["theme"] = *req.Theme
	}
	if req.PrivateSite != nil {
		updates["private_site"] = *req.PrivateSite
	}
	if req.ApiKey != nil {
		if len(*req.ApiKey) > 255 {
			api.RespondError(c, 400, "API key must not exceed 255 characters")
			return
		}
		updates["api_key"] = *req.ApiKey
	}
	if req.AutoDiscoveryKey != nil {
		if len(*req.AutoDiscoveryKey) > 255 {
			api.RespondError(c, 400, "Auto discovery key must not exceed 255 characters")
			return
		}
		updates["auto_discovery_key"] = *req.AutoDiscoveryKey
	}
	if req.ScriptDomain != nil {
		if len(*req.ScriptDomain) > 255 {
			api.RespondError(c, 400, "Script domain must not exceed 255 characters")
			return
		}
		updates["script_domain"] = *req.ScriptDomain
	}
	if req.SendIpAddrToGuest != nil {
		updates["send_ip_addr_to_guest"] = *req.SendIpAddrToGuest
	}
	if req.EulaAccepted != nil {
		updates["eula_accepted"] = *req.EulaAccepted
	}
	if req.GeoIpEnabled != nil {
		updates["geo_ip_enabled"] = *req.GeoIpEnabled
	}
	if req.GeoIpProvider != nil {
		validProviders := map[string]bool{"": true, "mmdb": true, "ip-api": true, "geojs": true}
		if !validProviders[*req.GeoIpProvider] {
			api.RespondError(c, 400, "Invalid geo IP provider")
			return
		}
		updates["geo_ip_provider"] = *req.GeoIpProvider
	}
	if req.NezhaCompatEnabled != nil {
		updates["nezha_compat_enabled"] = *req.NezhaCompatEnabled
	}
	if req.NezhaCompatListen != nil {
		if len(*req.NezhaCompatListen) > 100 {
			api.RespondError(c, 400, "Nezha compat listen must not exceed 100 characters")
			return
		}
		updates["nezha_compat_listen"] = *req.NezhaCompatListen
	}
	if req.OAuthEnabled != nil {
		updates["o_auth_enabled"] = *req.OAuthEnabled
	}
	if req.OAuthProvider != nil {
		if len(*req.OAuthProvider) > 50 {
			api.RespondError(c, 400, "OAuth provider must not exceed 50 characters")
			return
		}
		updates["o_auth_provider"] = *req.OAuthProvider
	}
	if req.DisablePasswordLogin != nil {
		updates["disable_password_login"] = *req.DisablePasswordLogin
	}
	if req.CustomHead != nil {
		updates["custom_head"] = *req.CustomHead
	}
	if req.CustomBody != nil {
		updates["custom_body"] = *req.CustomBody
	}
	if req.NotificationEnabled != nil {
		updates["notification_enabled"] = *req.NotificationEnabled
	}
	if req.NotificationMethod != nil {
		if len(*req.NotificationMethod) > 64 {
			api.RespondError(c, 400, "Notification method must not exceed 64 characters")
			return
		}
		updates["notification_method"] = *req.NotificationMethod
	}
	if req.NotificationTemplate != nil {
		updates["notification_template"] = *req.NotificationTemplate
	}
	if req.ExpireNotificationEnabled != nil {
		updates["expire_notification_enabled"] = *req.ExpireNotificationEnabled
	}
	if req.ExpireNotificationLeadDays != nil {
		if *req.ExpireNotificationLeadDays < 0 || *req.ExpireNotificationLeadDays > 365 {
			api.RespondError(c, 400, "Expire notification lead days must be between 0 and 365")
			return
		}
		updates["expire_notification_lead_days"] = *req.ExpireNotificationLeadDays
	}
	if req.LoginNotification != nil {
		updates["login_notification"] = *req.LoginNotification
	}
	if req.TrafficLimitPercentage != nil {
		if *req.TrafficLimitPercentage < 0 || *req.TrafficLimitPercentage > 100 {
			api.RespondError(c, 400, "Traffic limit percentage must be between 0 and 100")
			return
		}
		updates["traffic_limit_percentage"] = *req.TrafficLimitPercentage
	}
	if req.RecordEnabled != nil {
		updates["record_enabled"] = *req.RecordEnabled
	}
	if req.RecordPreserveTime != nil {
		if *req.RecordPreserveTime < 1 || *req.RecordPreserveTime > 87600 {
			api.RespondError(c, 400, "Record preserve time must be between 1 and 87600 hours (10 years)")
			return
		}
		updates["record_preserve_time"] = *req.RecordPreserveTime
	}
	if req.PingRecordPreserveTime != nil {
		if *req.PingRecordPreserveTime < 1 || *req.PingRecordPreserveTime > 8760 {
			api.RespondError(c, 400, "Ping record preserve time must be between 1 and 8760 hours (1 year)")
			return
		}
		updates["ping_record_preserve_time"] = *req.PingRecordPreserveTime
	}

	if len(updates) == 0 {
		api.RespondError(c, 400, "No valid fields to update")
		return
	}

	updates["id"] = 1 // Only one record
	if err := config.Update(updates); err != nil {
		api.RespondError(c, 500, "Failed to update settings: "+err.Error())
		return
	}

	uuid, _ := c.Get("uuid")
	message := "update settings: "
	for key := range updates {
		if key != "id" {
			message += key + ", "
		}
	}
	if len(message) > 2 {
		message = message[:len(message)-2]
	}
	auditlog.Log(c.ClientIP(), uuid.(string), message, "info")
	api.RespondSuccess(c, nil)
}

func ClearAllRecords(c *gin.Context) {
	records.DeleteAll()
	tasks.DeleteAllPingRecords()
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "clear all records", "info")
	api.RespondSuccess(c, nil)
}
