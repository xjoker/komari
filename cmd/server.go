package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/api/admin"
	"github.com/komari-monitor/komari/api/admin/clipboard"
	log_api "github.com/komari-monitor/komari/api/admin/log"
	"github.com/komari-monitor/komari/api/admin/notification"
	"github.com/komari-monitor/komari/api/middleware"
	"github.com/komari-monitor/komari/api/admin/test"
	"github.com/komari-monitor/komari/api/admin/update"
	"github.com/komari-monitor/komari/api/client"
	"github.com/komari-monitor/komari/api/jsonRpc"
	"github.com/komari-monitor/komari/api/record"
	"github.com/komari-monitor/komari/api/task"
	"github.com/komari-monitor/komari/cmd/flags"
	"github.com/komari-monitor/komari/database"
	"github.com/komari-monitor/komari/database/accounts"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/config"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	d_notification "github.com/komari-monitor/komari/database/notification"
	"github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/public"
	"github.com/komari-monitor/komari/utils"
	"github.com/komari-monitor/komari/utils/cloudflared"
	"github.com/komari-monitor/komari/utils/geoip"
	"github.com/komari-monitor/komari/utils/logger"
	logutil "github.com/komari-monitor/komari/utils/log"
	"github.com/komari-monitor/komari/utils/messageSender"
	"github.com/komari-monitor/komari/utils/notifier"
	"github.com/komari-monitor/komari/utils/oauth"
	"github.com/spf13/cobra"
)

var (
	DynamicCorsEnabled bool = false
)

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the server",
	Long:  `Start the server`,
	Run: func(cmd *cobra.Command, args []string) {
		RunServer()
	},
}

func init() {
	// 从环境变量获取监听地址
	listenAddr := GetEnv("KOMARI_LISTEN", "0.0.0.0:25774")
	ServerCmd.PersistentFlags().StringVarP(&flags.Listen, "listen", "l", listenAddr, "监听地址 [env: KOMARI_LISTEN]")
	RootCmd.AddCommand(ServerCmd)
}

func RunServer() {
	// #region 初始化
	// Initialize structured logging first (supports Loki-compatible JSON output)
	logger.InitDefault()

	// 安全检查: 确保数据库密码已配置
	if flags.DatabasePass == "" {
		logger.Fatal().Msg("Database password is required. Please set KOMARI_DB_PASS environment variable or use --db-pass flag.")
	}

	if err := os.MkdirAll("./data", os.ModePerm); err != nil {
		logger.Fatal().Err(err).Str("path", "./data").Msg("Failed to create data directory")
	}
	// 创建主题目录
	if err := os.MkdirAll("./data/theme", os.ModePerm); err != nil {
		logger.Fatal().Err(err).Str("path", "./data/theme").Msg("Failed to create theme directory")
	}
	InitDatabase()
	if utils.VersionHash != "unknown" {
		gin.SetMode(gin.ReleaseMode)
	}
	conf, err := config.Get()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to get configuration")
	}
	go geoip.InitGeoIp()
	go DoScheduledWork()
	go messageSender.Initialize()
	// oidcInit
	go oauth.Initialize()

	if conf.NezhaCompatEnabled {
		go func() {
			if err := StartNezhaCompat(conf.NezhaCompatListen); err != nil {
				logger.Error().Err(err).Str("component", "nezha-compat").Msg("Nezha compat server error")
				auditlog.EventLog("error", fmt.Sprintf("Nezha compat server error: %v", err))
			}
		}()
	}

	config.Subscribe(func(event config.ConfigEvent) {
		if event.New.OAuthProvider != event.Old.OAuthProvider {
			oidcProvider, err := database.GetOidcConfigByName(event.New.OAuthProvider)
			if err != nil {
				logger.Error().Err(err).Str("component", "oauth").Msg("Failed to get OIDC provider config")
			} else {
				logger.Info().Str("component", "oauth").Str("provider", oidcProvider.Name).Msg("Using OIDC provider")
			}
			err = oauth.LoadProvider(oidcProvider.Name, oidcProvider.Addition)
			if err != nil {
				auditlog.EventLog("error", fmt.Sprintf("Failed to load OIDC provider: %v", err))
			}
		}
		if event.New.NotificationMethod != event.Old.NotificationMethod {
			messageSender.Initialize()
		}
		if event.New.NezhaCompatEnabled != event.Old.NezhaCompatEnabled {
			if event.New.NezhaCompatEnabled {
				if err := StartNezhaCompat(event.New.NezhaCompatListen); err != nil {
					logger.Error().Err(err).Str("component", "nezha-compat").Msg("Failed to start Nezha compat server")
					auditlog.EventLog("error", fmt.Sprintf("start Nezha compat server error: %v", err))
				}
			} else {
				if err := StopNezhaCompat(); err != nil {
					logger.Error().Err(err).Str("component", "nezha-compat").Msg("Failed to stop Nezha compat server")
					auditlog.EventLog("error", fmt.Sprintf("stop Nezha compat server error: %v", err))
				}
			}
		}

	})
	// 初始化 cloudflared
	if strings.ToLower(GetEnv("KOMARI_ENABLE_CLOUDFLARED", "false")) == "true" {
		err := cloudflared.RunCloudflared() // 阻塞，确保cloudflared跑起来
		if err != nil {
			logger.Fatal().Err(err).Str("component", "cloudflared").Msg("Failed to run cloudflared")
		}
	}

	r := gin.New()
	r.Use(logutil.GinLogger())
	r.Use(logutil.GinRecovery())

	// Global rate limiting - prevent system overload (20,000 req/min)
	r.Use(middleware.RateLimitGlobal())

	// Per-IP rate limiting - prevent DoS attacks (1,000 req/IP/min)
	r.Use(middleware.RateLimitByIP())

	// Zlib compression (DEFLATE algorithm) for all responses
	// Benefits over Gzip:
	// - 10-15% less CPU usage on client side (critical for monitoring agents)
	// - Smaller header overhead (2 bytes vs 10 bytes)
	// - Same compression ratio as Gzip (both use DEFLATE)
	// - Ideal for reducing client machine resource usage
	r.Use(middleware.Zlib())

	// Decompress incoming zlib-compressed requests from clients
	r.Use(middleware.ZlibDecompress())

	// 动态 CORS 中间件

	DynamicCorsEnabled = conf.AllowCors
	config.Subscribe(func(event config.ConfigEvent) {
		DynamicCorsEnabled = event.New.AllowCors
		if event.New.GeoIpProvider != event.Old.GeoIpProvider {
			go geoip.InitGeoIp()
		}
		if event.New.NotificationMethod != event.Old.NotificationMethod {
			go messageSender.Initialize()
		}
	})
	r.Use(func(c *gin.Context) {
		if DynamicCorsEnabled {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Length, Content-Type, Authorization, Accept, X-CSRF-Token, X-Requested-With, Set-Cookie")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Authorization, Set-Cookie")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "43200") // 12 hours
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
		}
		c.Next()
	})

	r.Use(api.PrivateSiteMiddleware())

	r.Use(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	})

	r.Any("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})
	// #region 公开路由
	r.POST("/api/login", api.Login)
	r.GET("/api/me", api.GetMe)
	r.GET("/api/clients", api.GetClients)
	r.GET("/api/nodes", api.GetNodesInformation)
	r.GET("/api/public", api.GetPublicSettings)
	r.GET("/api/oauth", api.OAuth)
	r.GET("/api/oauth_callback", api.OAuthCallback)
	r.GET("/api/logout", api.Logout)
	r.GET("/api/version", api.GetVersion)
	r.GET("/api/recent/:uuid", api.GetClientRecentRecords)

	r.GET("/api/records/load", record.GetRecordsByUUID)
	r.GET("/api/records/ping", record.GetPingRecords)
	r.GET("/api/task/ping", task.GetPublicPingTasks)
	r.GET("/api/rpc2", jsonRpc.OnRpcRequest)
	r.POST("/api/rpc2", jsonRpc.OnRpcRequest)

	// #region Agent
	r.POST("/api/clients/register", client.RegisterClient)
	tokenAuthrized := r.Group("/api/clients", api.TokenAuthMiddleware(), middleware.RateLimitByToken())
	{
		tokenAuthrized.GET("/report", client.WebSocketReport) // websocket
		tokenAuthrized.POST("/uploadBasicInfo", client.UploadBasicInfo)
		tokenAuthrized.POST("/report", client.UploadReport)
		tokenAuthrized.GET("/terminal", client.EstablishConnection)

		// Protobuf endpoints (v2.0 protocol) with stricter rate limiting
		v2Group := tokenAuthrized.Group("/v2", middleware.RateLimitProtobuf())
		{
			v2Group.POST("/metrics", client.UploadProtobufReport)
			v2Group.POST("/profile", client.UploadClientProfile)
			v2Group.POST("/ping", client.UploadPingResult)
			v2Group.GET("/profile/:uuid", client.GetClientProfile)
		}
	}
	// #region 管理员
	adminAuthrized := r.Group("/api/admin", api.AdminAuthMiddleware())
	{
		adminAuthrized.GET("/download/backup", admin.DownloadBackup)
		adminAuthrized.POST("/upload/backup", admin.UploadBackup)
		// test
		testGroup := adminAuthrized.Group("/test")
		{
			testGroup.GET("/geoip", test.TestGeoIp)
			testGroup.POST("/sendMessage", test.TestSendMessage)
		}
		// update
		updateGroup := adminAuthrized.Group("/update")
		{
			updateGroup.POST("/mmdb", update.UpdateMmdbGeoIP)
			updateGroup.POST("/user", update.UpdateUser)
			updateGroup.PUT("/favicon", update.UploadFavicon)
			updateGroup.POST("/favicon", update.DeleteFavicon)
		}
		// settings
		settingsGroup := adminAuthrized.Group("/settings")
		{
			settingsGroup.GET("/", admin.GetSettings)
			settingsGroup.POST("/", admin.EditSettings)
			settingsGroup.POST("/oidc", admin.SetOidcProvider)
			settingsGroup.GET("/oidc", admin.GetOidcProvider)
			settingsGroup.POST("/message-sender", admin.SetMessageSenderProvider)
			settingsGroup.GET("/message-sender", admin.GetMessageSenderProvider)
		}
		// themes
		themeGroup := adminAuthrized.Group("/theme")
		{
			themeGroup.PUT("/upload", admin.UploadTheme)
			themeGroup.GET("/list", admin.ListThemes)
			themeGroup.POST("/delete", admin.DeleteTheme)
			themeGroup.GET("/set", admin.SetTheme)
			themeGroup.POST("/update", admin.UpdateTheme)
			themeGroup.POST("/settings", admin.UpdateThemeSettings)
		}
		// clients
		clientGroup := adminAuthrized.Group("/client")
		{
			clientGroup.POST("/add", admin.AddClient)
			clientGroup.GET("/list", admin.ListClients)
			clientGroup.GET("/:uuid", admin.GetClient)
			clientGroup.POST("/:uuid/edit", admin.EditClient)
			clientGroup.POST("/:uuid/remove", admin.RemoveClient)
			clientGroup.GET("/:uuid/token", admin.GetClientToken)
			clientGroup.POST("/order", admin.OrderWeight)
			// client terminal
			clientGroup.GET("/:uuid/terminal", api.RequestTerminal)
		}

		// records
		recordGroup := adminAuthrized.Group("/record")
		{
			recordGroup.POST("/clear", admin.ClearRecord)
			recordGroup.POST("/clear/all", admin.ClearAllRecords)
		}
		// oauth2
		oauth2Group := adminAuthrized.Group("/oauth2")
		{
			oauth2Group.GET("/bind", admin.BindingExternalAccount)
			oauth2Group.POST("/unbind", admin.UnbindExternalAccount)
		}
		sessionGroup := adminAuthrized.Group("/session")
		{
			sessionGroup.GET("/get", admin.GetSessions)
			sessionGroup.POST("/remove", admin.DeleteSession)
			sessionGroup.POST("/remove/all", admin.DeleteAllSession)
		}
		two_factorGroup := adminAuthrized.Group("/2fa")
		{
			two_factorGroup.GET("/generate", admin.Generate2FA)
			two_factorGroup.POST("/enable", admin.Enable2FA)
			two_factorGroup.POST("/disable", admin.Disable2FA)
		}
		adminAuthrized.GET("/logs", log_api.GetLogs)

		// clipboard
		clipboardGroup := adminAuthrized.Group("/clipboard")
		{
			clipboardGroup.GET("/:id", clipboard.GetClipboard)
			clipboardGroup.GET("", clipboard.ListClipboard)
			clipboardGroup.POST("", clipboard.CreateClipboard)
			clipboardGroup.POST("/:id", clipboard.UpdateClipboard)
			clipboardGroup.POST("/remove", clipboard.BatchDeleteClipboard)
			clipboardGroup.POST("/:id/remove", clipboard.DeleteClipboard)
		}

		notificationGroup := adminAuthrized.Group("/notification")
		{
			// offline notifications
			notificationGroup.GET("/offline", notification.ListOfflineNotifications)
			notificationGroup.POST("/offline/edit", notification.EditOfflineNotification)
			notificationGroup.POST("/offline/enable", notification.EnableOfflineNotification)
			notificationGroup.POST("/offline/disable", notification.DisableOfflineNotification)
			loadAlertGroup := notificationGroup.Group("/load")
			{
				loadAlertGroup.GET("/", notification.GetAllLoadNotifications)
				loadAlertGroup.POST("/add", notification.AddLoadNotification)
				loadAlertGroup.POST("/delete", notification.DeleteLoadNotification)
				loadAlertGroup.POST("/edit", notification.EditLoadNotification)
			}
		}

		pingTaskGroup := adminAuthrized.Group("/ping")
		{
			pingTaskGroup.GET("/", admin.GetAllPingTasks)
			pingTaskGroup.POST("/add", admin.AddPingTask)
			pingTaskGroup.POST("/delete", admin.DeletePingTask)
			pingTaskGroup.POST("/edit", admin.EditPingTask)

		}

	}

	public.Static(r.Group("/"), func(handlers ...gin.HandlerFunc) {
		r.NoRoute(handlers...)
	})
	// #region 静态文件服务
	public.UpdateIndex(conf)
	config.Subscribe(func(event config.ConfigEvent) {
		public.UpdateIndex(event.New)
	})

	srv := &http.Server{
		Addr:    flags.Listen,
		Handler: r,
	}
	logger.Info().
		Str("address", flags.Listen).
		Str("commit", utils.VersionHash).
		Msg("Starting Komari server")
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			OnFatal(err)
			logger.Fatal().Err(err).Str("address", flags.Listen).Msg("Server failed to start")
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("Received shutdown signal, shutting down gracefully...")
	OnShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Server forced to shutdown")
	}

}

func InitDatabase() {
	// // 打印数据库类型和连接信息
	// if flags.DatabaseType == "mysql" {
	// 	log.Printf("使用 MySQL 数据库连接: %s@%s:%s/%s",
	// 		flags.DatabaseUser, flags.DatabaseHost, flags.DatabasePort, flags.DatabaseName)
	// 	log.Printf("环境变量配置: [KOMARI_DB_TYPE=%s] [KOMARI_DB_HOST=%s] [KOMARI_DB_PORT=%s] [KOMARI_DB_USER=%s] [KOMARI_DB_NAME=%s]",
	// 		os.Getenv("KOMARI_DB_TYPE"), os.Getenv("KOMARI_DB_HOST"), os.Getenv("KOMARI_DB_PORT"),
	// 		os.Getenv("KOMARI_DB_USER"), os.Getenv("KOMARI_DB_NAME"))
	// } else {
	// 	log.Printf("使用 SQLite 数据库文件: %s", flags.DatabaseFile)
	// 	log.Printf("环境变量配置: [KOMARI_DB_TYPE=%s] [KOMARI_DB_FILE=%s]",
	// 		os.Getenv("KOMARI_DB_TYPE"), os.Getenv("KOMARI_DB_FILE"))
	// }
	var count int64 = 0
	if dbcore.GetDBInstance().Model(&models.User{}).Count(&count); count == 0 {
		user, passwd, err := accounts.CreateDefaultAdminAccount()
		if err != nil {
			panic(err)
		}
		logger.Info().
			Str("username", user).
			Str("password", passwd).
			Msg("Default admin account created")
	}
}

// #region 定时任务
func DoScheduledWork() {
	tasks.ReloadPingSchedule()
	d_notification.ReloadLoadNotificationSchedule()
	ticker := time.NewTicker(time.Minute * 30)
	minute := time.NewTicker(60 * time.Second)

	// 获取配置并处理错误
	cfg, err := config.Get()
	if err != nil {
		logger.Fatal().Err(err).Str("component", "scheduler").Msg("Failed to get configuration in DoScheduledWork")
	}

	go notifier.CheckExpireScheduledWork()

	// 延迟5分钟后执行首次数据压缩，避免启动时IO峰值
	time.AfterFunc(5*time.Minute, func() {
		logger.Info().Str("component", "compaction").Msg("Running initial record compaction (delayed 5 minutes)")
		records.CompactRecord()
	})

	// 数据库健康检查 - 每分钟检查连接状态
	healthTicker := time.NewTicker(time.Minute)
	go func() {
		for range healthTicker.C {
			db := dbcore.GetDBInstance()
			sqlDB, err := db.DB()
			if err != nil {
				logger.Error().Err(err).Str("component", "health-check").Msg("Failed to get database instance")
				continue
			}

			// Ping 数据库以检测连接是否有效
			if err := sqlDB.Ping(); err != nil {
				logger.Warn().Err(err).Str("component", "health-check").Msg("Database connection lost (GORM will auto-reconnect)")
				// 注意: GORM 会自动重连，这里只是记录错误
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			records.DeleteRecordBefore(time.Now().Add(-time.Hour * time.Duration(cfg.RecordPreserveTime)))
			records.CompactRecord()
			tasks.DeletePingRecordsBefore(time.Now().Add(-time.Hour * time.Duration(cfg.PingRecordPreserveTime)))
			auditlog.RemoveOldLogs()
		case <-minute.C:
			api.SaveClientReportToDB()
			if !cfg.RecordEnabled {
				records.DeleteAll()
				tasks.DeleteAllPingRecords()
			}
			// 每分钟检查一次流量提醒
			go notifier.CheckTraffic()
		}
	}

}

func OnShutdown() {
	auditlog.Log("", "", "server is shutting down", "info")
	cloudflared.Kill()
}

func OnFatal(err error) {
	auditlog.Log("", "", "server encountered a fatal error: "+err.Error(), "error")
	cloudflared.Kill()
}
