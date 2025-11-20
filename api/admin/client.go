package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/ws"
)

func AddClient(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		uuid, token, err := clients.CreateClient()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "uuid": uuid, "token": token})
		return
	}
	uuid, token, err := clients.CreateClientWithName(req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "create client:"+uuid, "info")
	c.JSON(http.StatusOK, gin.H{"status": "success", "uuid": uuid, "token": token, "message": ""})
}

// EditClientRequest 定义允许更新的客户端字段（防止批量赋值漏洞）
type EditClientRequest struct {
	Name             *string  `json:"name"`
	Remark           *string  `json:"remark"`
	PublicRemark     *string  `json:"public_remark"`
	Weight           *int     `json:"weight"`
	Price            *float64 `json:"price"`
	BillingCycle     *int     `json:"billing_cycle"`
	AutoRenewal      *bool    `json:"auto_renewal"`
	Currency         *string  `json:"currency"`
	ExpiredAt        *string  `json:"expired_at"`
	Group            *string  `json:"group"`
	Tags             *string  `json:"tags"`
	Hidden           *bool    `json:"hidden"`
	TrafficLimit     *int64   `json:"traffic_limit"`
	TrafficLimitType *string  `json:"traffic_limit_type"`
}

func EditClient(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid or missing UUID"})
		return
	}

	// 验证 UUID 是否存在
	_, err := clients.GetClientByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Client not found"})
		return
	}

	var req EditClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	// 构建更新 map，只包含非 nil 字段，并进行验证
	updates := make(map[string]interface{})

	if req.Name != nil {
		if len(*req.Name) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Name must not exceed 100 characters"})
			return
		}
		updates["name"] = *req.Name
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if req.PublicRemark != nil {
		updates["public_remark"] = *req.PublicRemark
	}
	if req.Weight != nil {
		if *req.Weight < 0 || *req.Weight > 10000 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Weight must be between 0 and 10000"})
			return
		}
		updates["weight"] = *req.Weight
	}
	if req.Price != nil {
		if *req.Price < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Price must be non-negative"})
			return
		}
		updates["price"] = *req.Price
	}
	if req.BillingCycle != nil {
		if *req.BillingCycle < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Billing cycle must be non-negative"})
			return
		}
		updates["billing_cycle"] = *req.BillingCycle
	}
	if req.AutoRenewal != nil {
		updates["auto_renewal"] = *req.AutoRenewal
	}
	if req.Currency != nil {
		if len(*req.Currency) > 20 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Currency must not exceed 20 characters"})
			return
		}
		updates["currency"] = *req.Currency
	}
	if req.ExpiredAt != nil {
		updates["expired_at"] = *req.ExpiredAt
	}
	if req.Group != nil {
		if len(*req.Group) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Group must not exceed 100 characters"})
			return
		}
		updates["group"] = *req.Group
	}
	if req.Tags != nil {
		updates["tags"] = *req.Tags
	}
	if req.Hidden != nil {
		updates["hidden"] = *req.Hidden
	}
	if req.TrafficLimit != nil {
		if *req.TrafficLimit < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Traffic limit must be non-negative"})
			return
		}
		updates["traffic_limit"] = *req.TrafficLimit
	}
	if req.TrafficLimitType != nil {
		validTypes := map[string]bool{"sum": true, "max": true, "min": true, "up": true, "down": true}
		if !validTypes[*req.TrafficLimitType] {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid traffic limit type"})
			return
		}
		updates["traffic_limit_type"] = *req.TrafficLimitType
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "No valid fields to update"})
		return
	}

	updates["uuid"] = uuid
	err = clients.SaveClient(updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "edit client:"+uuid, "info")
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func RemoveClient(c *gin.Context) {
	uuid := c.Param("uuid")
	err := clients.DeleteClient(uuid)
	if err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  "Failed to delete client" + err.Error(),
		})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "delete client:"+uuid, "warn")
	c.JSON(200, gin.H{"status": "success"})
	ws.DeleteConnectedClients(uuid)
	ws.DeleteLatestReport(uuid)
}

func ClearRecord(c *gin.Context) {
	if err := records.DeleteAll(); err != nil {
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to delete Record" + err.Error(),
		})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "clear records", "warn")
	c.JSON(200, gin.H{"status": "success"})
}

func GetClient(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid or missing UUID",
		})
		return
	}

	result, err := clients.GetClientByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

func ListClients(c *gin.Context) {
	cls, err := clients.GetAllClientBasicInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cls)
}

func GetClientToken(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid or missing UUID",
		})
		return
	}

	token, err := clients.GetClientTokenByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "token": token, "message:": ""})
}
