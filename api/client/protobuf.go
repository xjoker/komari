package client

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/common"
	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/tasks"
	pb "github.com/komari-monitor/komari/proto"
	"github.com/komari-monitor/komari/ws"
	"google.golang.org/protobuf/proto"
)

// UploadProtobufReport handles Protobuf-encoded metrics reports
// This endpoint expects zlib-compressed Protobuf data with MetricsReport message
func UploadProtobufReport(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Println("Failed to read protobuf request body:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Deserialize Protobuf message
	var metricsReport pb.MetricsReport
	if err := proto.Unmarshal(bodyBytes, &metricsReport); err != nil {
		log.Println("Failed to unmarshal protobuf message:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid protobuf format"})
		return
	}

	// Validate protocol version
	if metricsReport.Version != "2.0" && metricsReport.Version != "" {
		log.Printf("Unsupported protocol version: %s", metricsReport.Version)
	}

	// Convert Protobuf to internal Report structure for backward compatibility
	report := convertProtobufToReport(&metricsReport)
	report.UpdatedAt = time.Now()

	// Save to cache and database
	err = SaveClientReport(report.UUID, report)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("%v", err)})
		return
	}

	// Update latest report for WebSocket
	ws.SetLatestReport(report.UUID, &report)

	c.JSON(200, gin.H{"status": "success"})
}

// UploadClientProfile handles static profile updates
// Clients send this only when their hardware configuration changes
func UploadClientProfile(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Println("Failed to read profile request body:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var profileReq pb.ProfileUpdateRequest
	if err := proto.Unmarshal(bodyBytes, &profileReq); err != nil {
		log.Println("Failed to unmarshal profile message:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid protobuf format"})
		return
	}

	if profileReq.Profile == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Profile is required"})
		return
	}

	// Get profile cache
	profileCache := clients.GetProfileCache()

	// Check if update is needed
	if !profileCache.NeedsUpdate(profileReq.Profile.Uuid, profileReq.Profile) {
		// Profile hasn't changed, no need to update
		c.JSON(200, gin.H{"status": "success", "message": "Profile unchanged"})
		return
	}

	// Update cache
	profileCache.Set(profileReq.Profile.Uuid, profileReq.Profile)

	log.Printf("Updated profile for client %s: CPU=%s, Cores=%d, RAM=%d GB",
		profileReq.Profile.Uuid,
		profileReq.Profile.CpuName,
		profileReq.Profile.CpuCores,
		profileReq.Profile.RamTotal/(1024*1024*1024))

	c.JSON(200, gin.H{"status": "success", "message": "Profile updated"})
}

// UploadPingResult handles Protobuf-encoded ping results
func UploadPingResult(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Println("Failed to read ping result body:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var pingResult pb.PingResult
	if err := proto.Unmarshal(bodyBytes, &pingResult); err != nil {
		log.Println("Failed to unmarshal ping result:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid protobuf format"})
		return
	}

	// Get client UUID from token
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token required"})
		return
	}

	uuid, err := clients.GetClientUUIDByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	// Save ping record
	record := models.PingRecord{
		Client: uuid,
		TaskId: uint(pingResult.TaskId),
		Value:  int(pingResult.Value),
		Time:   models.FromTime(time.Unix(pingResult.FinishedAt, 0)),
	}

	tasks.SavePingRecord(record)

	c.JSON(200, gin.H{"status": "success"})
}

// convertProtobufToReport converts Protobuf MetricsReport to internal Report structure
// This maintains backward compatibility with existing database schema
func convertProtobufToReport(pbMsg *pb.MetricsReport) common.Report {
	report := common.Report{
		Version: pbMsg.Version,
		UUID:    pbMsg.Uuid,
		CPU: common.CPUReport{
			Usage: float64(pbMsg.CpuUsage),
		},
		Ram: common.RamReport{
			Used:  pbMsg.RamUsed,
			Total: 0, // Total is stored in profile, not sent every time
		},
		Swap: common.RamReport{
			Used:  pbMsg.SwapUsed,
			Total: 0,
		},
		Load: common.LoadReport{
			Load1:  float64(pbMsg.Load1),
			Load5:  float64(pbMsg.Load5),
			Load15: float64(pbMsg.Load15),
		},
		Disk: common.DiskReport{
			Used:  pbMsg.DiskUsed,
			Total: 0, // Total is in profile
		},
		Network: common.NetworkReport{
			Up:        pbMsg.NetInSpeed,
			Down:      pbMsg.NetOutSpeed,
			TotalUp:   pbMsg.NetTotalOut,
			TotalDown: pbMsg.NetTotalIn,
		},
		Connections: common.ConnectionsReport{
			TCP: int(pbMsg.TcpConnections),
			UDP: int(pbMsg.UdpConnections),
		},
		Uptime:  pbMsg.Uptime,
		Process: int(pbMsg.ProcessCount),
		Message: pbMsg.Message,
		Method:  "protobuf",
	}

	// Convert GPU metrics if present
	if len(pbMsg.Gpu) > 0 {
		gpuReport := &common.GPUDetailReport{
			Count:        int(len(pbMsg.Gpu)),
			DetailedInfo: make([]common.GPUDeviceInfo, len(pbMsg.Gpu)),
		}

		var totalUsage float64
		for i, gpu := range pbMsg.Gpu {
			gpuReport.DetailedInfo[i] = common.GPUDeviceInfo{
				MemoryUsed:  gpu.MemoryUsed,
				Utilization: float64(gpu.Utilization),
				Temperature: int(gpu.Temperature),
			}
			totalUsage += float64(gpu.Utilization)
		}

		if len(pbMsg.Gpu) > 0 {
			gpuReport.AverageUsage = totalUsage / float64(len(pbMsg.Gpu))
		}

		report.GPU = gpuReport
	}

	// Enrich with profile data for total values
	profileCache := clients.GetProfileCache()
	if profile, exists := profileCache.Get(pbMsg.Uuid); exists {
		report.Ram.Total = profile.RamTotal
		report.Swap.Total = profile.SwapTotal
		report.Disk.Total = profile.DiskTotal

		report.CPU.Name = profile.CpuName
		report.CPU.Cores = int(profile.CpuCores)
		report.CPU.Arch = profile.CpuArch

		// Add GPU names from profile
		if report.GPU != nil && len(profile.Gpu) > 0 {
			for i, gpuProfile := range profile.Gpu {
				if i < len(report.GPU.DetailedInfo) {
					report.GPU.DetailedInfo[i].Name = gpuProfile.Name
					report.GPU.DetailedInfo[i].MemoryTotal = gpuProfile.MemoryTotal
				}
			}
		}
	}

	return report
}

// GetClientProfile returns cached profile for a client
func GetClientProfile(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID required"})
		return
	}

	profileCache := clients.GetProfileCache()
	profile, exists := profileCache.Get(uuid)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile not found"})
		return
	}

	c.JSON(200, gin.H{"profile": profile})
}
