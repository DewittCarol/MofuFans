package router

import (
	"context"
	"errors"
	"fmt"
	"mofufans/internal/avatar"
	"mofufans/internal/bilibili"
	"mofufans/internal/controller"
	"mofufans/internal/database"
	"mofufans/internal/guard"
	"mofufans/internal/moderation"
	"mofufans/internal/siteconfig"
	"mofufans/internal/storage"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Run() {
	r := gin.New()
	r.Use(gin.LoggerWithFormatter(func(p gin.LogFormatterParams) string {
		path := p.Request.URL.Path
		if route, ok := p.Keys["safe_route"].(string); ok {
			path = route
		} else {
			path = "unmatched"
		}
		return fmt.Sprintf("%s %s %s %d %s %s\n", p.TimeStamp.UTC().Format(time.RFC3339), p.ClientIP, p.Method, p.StatusCode, p.Latency, path)
	}))
	r.MaxMultipartMemory = 30 << 20 // 30 MB
	r.Use(gin.Recovery())
	installation, err := controller.NewInstallationController(siteconfig.Current())
	if err != nil {
		logrus.Fatal("Cannot prepare installation: ", err)
	}
	if installation.Required() {
		logrus.Info("Complete installation at /admin/setup/. One-time installation code: ", installation.Token)
	}
	if origins := viper.GetStringSlice("server.allowed_origins"); len(origins) > 0 {
		r.Use(cors.New(cors.Config{AllowOrigins: origins, AllowCredentials: true, AllowMethods: []string{"GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS"}, AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "Upload-Offset", "X-MofuFans-Request", "X-Setup-Token"}}))
	}
	r.Use(installation.Gate)
	if err := r.SetTrustedProxies(viper.GetStringSlice("server.trusted_proxies")); err != nil {
		logrus.Fatal("invalid trusted proxies")
	}
	policies := guard.DefaultPolicies()
	for name, p := range policies {
		prefix := "security.guard.policies." + name + "."
		if viper.IsSet(prefix + "per_minute") {
			p.PerMinute = viper.GetInt(prefix + "per_minute")
		}
		if viper.IsSet(prefix + "burst") {
			p.Burst = viper.GetInt(prefix + "burst")
		}
		if viper.IsSet(prefix + "global_per_minute") {
			p.GlobalPerMinute = viper.GetInt(prefix + "global_per_minute")
		}
		if viper.IsSet(prefix + "concurrent") {
			p.Concurrent = viper.GetInt(prefix + "concurrent")
		}
		policies[name] = p
	}
	protection, err := guard.New(guard.Options{Directory: filepath.Join(viper.GetString("state_directory"), "security"), Secret: viper.GetString("security.session_secret"), Policies: policies})
	if err != nil {
		logrus.Fatal("Cannot initialize application protection: ", err)
	}
	defer protection.Close()
	r.Use(protection.Middleware(viper.GetStringSlice("server.allowed_origins"), func() bool { return siteconfig.Active().SecureCookie }))
	r.Use(func(c *gin.Context) {
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		c.Set("safe_route", route)
		c.Next()
	})
	sessionSecret := viper.GetString("security.session_secret")
	if len(sessionSecret) < 32 {
		logrus.Fatal("security.session_secret must contain at least 32 bytes")
	}
	store := newSessionStore(sessionSecret, 12*60*60)
	store.Options(sessions.Options{
		Path:     "/",
		Secure:   siteconfig.Active().SecureCookie,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   12 * 60 * 60,
	})
	r.Use(sessions.Sessions("session", store))
	r.Use(func(c *gin.Context) {
		sessions.Default(c).Options(sessions.Options{Path: "/", MaxAge: 12 * 60 * 60, Secure: siteconfig.Active().SecureCookie, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		c.Next()
	})
	r.GET("/upload-img/:filename", func(c *gin.Context) { storage.Uploads().Serve(c.Writer, c.Request, c.Param("filename")) })
	r.HEAD("/upload-img/:filename", func(c *gin.Context) { storage.Uploads().Serve(c.Writer, c.Request, c.Param("filename")) })
	api := r.Group("/api")
	api.GET("/setup/status", installation.Status)
	api.POST("/setup/verify", installation.Verify)
	api.POST("/setup/complete", installation.Complete)
	api.POST("/setup/storage-test", installation.TestStorage)
	api.GET("/installation-settings", authMiddleware, installation.GetSettings)
	api.PUT("/installation-settings", authMiddleware, installation.PutSettings)
	api.POST("/installation-settings/test", authMiddleware, installation.TestStorage)
	gifts := new(controller.DigitalGiftController)
	api.GET("/gifts", gifts.List)
	api.GET("/gifts/:id", gifts.Detail)
	api.POST("/gifts/:id/unlock", gifts.Unlock)
	api.POST("/gifts/:id/lock", gifts.Lock)
	api.GET("/gifts/:id/files/:file", gifts.File)
	api.HEAD("/gifts/:id/files/:file", gifts.File)
	api.GET("/admin/gifts", authMiddleware, gifts.List)
	api.POST("/admin/gifts", authMiddleware, gifts.Save)
	api.GET("/admin/gifts/:id", authMiddleware, gifts.Detail)
	api.PUT("/admin/gifts/:id", authMiddleware, gifts.Save)
	api.DELETE("/admin/gifts/:id", authMiddleware, gifts.Delete)
	api.GET("/admin/gifts/:id/password", authMiddleware, gifts.Password)
	api.POST("/admin/gifts/:id/files", authMiddleware, gifts.Upload)
	api.POST("/admin/gifts/:id/uploads", authMiddleware, gifts.StartUpload)
	api.GET("/admin/gifts/:id/uploads/:upload", authMiddleware, gifts.UploadStatus)
	api.PUT("/admin/gifts/:id/uploads/:upload", authMiddleware, gifts.UploadChunk)
	api.POST("/admin/gifts/:id/uploads/:upload/complete", authMiddleware, gifts.CompleteUpload)
	api.DELETE("/admin/gifts/:id/uploads/:upload", authMiddleware, gifts.CancelUpload)
	api.GET("/admin/gifts/:id/files/:file", authMiddleware, gifts.File)
	api.DELETE("/admin/gifts/:id/files/:file", authMiddleware, gifts.DeleteFile)
	api.GET("/admin/gifts/:id/recipients", authMiddleware, gifts.Recipients)
	api.GET("/admin/gifts/:id/rosters", authMiddleware, gifts.Rosters)
	api.PUT("/admin/gifts/:id/rosters", authMiddleware, gifts.SaveRosters)
	api.PUT("/admin/gifts/:id/recipients/:recipient", authMiddleware, gifts.UpdateRecipient)
	tagController := new(controller.TagController)
	userController := new(controller.UserController)
	securityController := &controller.SecurityController{Protection: protection}
	api.GET("/admin/security", authMiddleware, securityController.View)
	api.POST("/admin/reauth", authMiddleware, userController.Reauthenticate)
	api.POST("/admin/security/bans", authMiddleware, controller.RequireRecentAdmin, securityController.Ban)
	api.DELETE("/admin/security/bans/:id", authMiddleware, controller.RequireRecentAdmin, securityController.Unban)
	questionController := controller.NewQuestionController()
	configController := new(controller.ConfigController)
	aiController := new(controller.AIController)
	reviews := new(controller.ModerationController)
	api.GET("/moderation/status", authMiddleware, reviews.Status)
	api.POST("/moderation/queue", authMiddleware, reviews.Queue)
	if !viper.GetBool("development.disable_background_workers") {
		(&moderation.Worker{DB: database.DB}).Start(context.Background())
	}
	emojiController := new(controller.EmojiAssetController)
	backgroundController := new(controller.BackgroundController)
	themes := new(controller.ThemeController)
	brandDirectory := viper.GetString("branding_directory")
	if brandDirectory == "" {
		backgroundDirectory := viper.GetString("background_directory")
		if backgroundDirectory == "" {
			backgroundDirectory = "data/backgrounds"
		}
		brandDirectory = filepath.Join(filepath.Dir(backgroundDirectory), "branding")
	}
	themeAssets := &controller.BackgroundController{Directory: brandDirectory, AssetPrefix: "/api/theme-assets/", Label: "品牌"}
	voyageController := new(controller.VoyageController)
	voyageDelivery := new(controller.VoyageDeliveryController)
	songs := new(controller.SongController)
	gallery := new(controller.GalleryController)
	statisticsController := new(controller.StatisticsController)
	accountController := &controller.AccountController{SignatureBrand: controller.ThemeSignatureBrand}
	bilibiliClient := bilibili.NewClient()
	voyageRecipients := &controller.VoyageRecipientsController{Client: bilibiliClient, AvatarStorage: avatar.NewStore()}
	profiles := &controller.BilibiliProfilesController{Client: bilibiliClient, AvatarStorage: avatar.NewStore()}
	api.POST("/bilibili/profiles/:uid", authMiddleware, profiles.Get)
	api.POST("/bilibili/profiles-cache", authMiddleware, profiles.Cached)
	memberController := &controller.MemberController{Client: bilibiliClient, AvatarStorage: avatar.NewStore()}
	verificationAccountController := &controller.BilibiliVerificationAccountController{Client: bilibiliClient}
	if !viper.GetBool("development.disable_background_workers") {
		// Identity verification is initiated by the user and checks a fresh
		// signature. Never poll follow events as an alternative proof.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			avatar.NewStore().SyncExistingUsers(ctx)
		}()
	}
	{
		api.GET("/theme", themes.Get)
		api.PUT("/theme", authMiddleware, themes.Put)
		api.GET("/theme/presets", authMiddleware, themes.Presets)
		api.POST("/theme/presets", authMiddleware, themes.SavePreset)
		api.DELETE("/theme/presets/:id", authMiddleware, themes.DeletePreset)
		api.POST("/theme-assets", authMiddleware, themeAssets.Upload)
		api.GET("/theme-assets/:filename", themeAssets.File)
		api.GET("/songs", songs.List)
		api.GET("/song-tags", songs.Tags)
		api.POST("/admin/song-tags", authMiddleware, songs.CreateTag)
		api.POST("/admin/songs", authMiddleware, songs.Save)
		api.PUT("/admin/songs/:id", authMiddleware, songs.Save)
		api.DELETE("/admin/songs/:id", authMiddleware, songs.Delete)
		api.GET("/gallery", gallery.List)
		api.POST("/gallery", gallery.Create)
		api.GET("/gallery/assets/:filename", gallery.File)
		api.GET("/admin/gallery", authMiddleware, gallery.List)
		api.POST("/admin/gallery", authMiddleware, gallery.Create)
		api.POST("/admin/gallery/:id/images", authMiddleware, gallery.Create)
		api.PUT("/admin/gallery/:id", authMiddleware, gallery.Update)
		api.DELETE("/admin/gallery/:id", authMiddleware, gallery.Delete)
		// Public member account
		{
			memberAPI := api.Group("/account", controller.MemberAccountsEnabled, controller.MemberRequestSecurity)
			memberAPI.POST("/signature/start", accountController.StartSignature)
			memberAPI.GET("/signature/status", accountController.SignatureStatus)
			memberAPI.POST("/signature/check", accountController.CheckSignature)
			memberAPI.POST("/signature/finish", accountController.FinishSignature)
			memberAPI.POST("/password", accountController.ChangePassword)
			memberAPI.POST("/logout-others", accountController.LogoutOthers)
			memberVoyage := &controller.MemberVoyageController{}
			memberAPI.GET("/address-book", memberVoyage.AddressBook)
			memberAPI.POST("/address-book", memberVoyage.SaveAddressBook)
			memberAPI.GET("/collections", memberVoyage.Collections)
			memberAPI.POST("/collections/:id/claim", voyageDelivery.MemberClaim)
			api.POST("/account/verification", controller.MemberAccountsEnabled, accountController.StartVerification)
			api.GET("/account/verification", controller.MemberAccountsEnabled, accountController.VerificationStatus)
			api.POST("/account/register", controller.MemberAccountsEnabled, accountController.Register)
			api.POST("/account/login", controller.MemberAccountsEnabled, accountController.Login)
			api.GET("/account/info", controller.MemberAccountsEnabled, accountController.Info)
			api.POST("/account/logout", accountController.Logout)
		}
		// User
		{
			api.POST("/login", userController.Login)
			api.GET("/info", authMiddleware, userController.Info)
			api.POST("/logout", authMiddleware, userController.Logout)

			api.GET("/user", authMiddleware, userController.Get)
			api.POST("/user", authMiddleware, controller.RequireRecentAdmin, userController.Post)
			api.PUT("/user/:id", authMiddleware, controller.RequireRecentAdmin, userController.Put)
			api.DELETE("/user/:id", authMiddleware, controller.RequireRecentAdmin, userController.Delete)
		}
		// Registered members
		{
			api.GET("/member", authMiddleware, memberController.Get)
			api.POST("/member", authMiddleware, memberController.Post)
			api.PUT("/member/:uid", authMiddleware, memberController.Put)
			api.DELETE("/member/:uid", authMiddleware, memberController.Delete)
		}
		// Bilibili verification account
		{
			api.GET("/bilibili-verification-account", authMiddleware, verificationAccountController.Get)
			api.PUT("/bilibili-verification-account", authMiddleware, verificationAccountController.Put)
			api.POST("/bilibili-verification-account/test", authMiddleware, verificationAccountController.Test)
			api.DELETE("/bilibili-verification-account", authMiddleware, verificationAccountController.Delete)
		}
		// Tag
		{
			api.GET("/tag", tagController.Get)
			api.PUT("/tag/:id", authMiddleware, tagController.Put)
			api.DELETE("/tag/:id", authMiddleware, tagController.Delete)
			api.POST("/tag", authMiddleware, tagController.Post)
		}
		// Question
		{
			api.GET("/question", questionController.Get)
			api.GET("/admin/questions", authMiddleware, questionController.Get)
			api.POST("/question", questionController.Post)
			api.PUT("/question/:id", authMiddleware, questionController.Put)
			api.POST("/question/:id/emoji", questionController.Emoji)
			api.GET("/sse", questionController.SSE)
			api.GET("/ws", questionController.WebSocket)
			api.DELETE("/question/:id", authMiddleware, questionController.Delete)
		}
		// Config
		{
			api.GET("/emojis", emojiController.List)
			api.GET("/emoji-catalog", emojiController.Catalog)
			api.POST("/emoji-collections", authMiddleware, emojiController.SaveCollection)
			api.PUT("/emoji-collections/:id", authMiddleware, emojiController.SaveCollection)
			api.DELETE("/emoji-collections/:id", authMiddleware, emojiController.DeleteCollection)
			api.PUT("/emojis/order", authMiddleware, emojiController.Reorder)
			api.GET("/emoji-assets/:filename", emojiController.File)
			api.POST("/emojis", authMiddleware, emojiController.Upload)
			api.POST("/emojis/unicode", authMiddleware, emojiController.AddUnicode)
			api.PUT("/emojis/:id/name", authMiddleware, emojiController.Rename)
			api.PUT("/emojis/status", authMiddleware, emojiController.SetStatus)
		}
		// Config
		{
			api.GET("/config", configController.Get)
			api.GET("/background-assets/:filename", backgroundController.File)
			api.POST("/background-assets", authMiddleware, backgroundController.Upload)
			api.PUT("/background", authMiddleware, backgroundController.Put)
			api.PUT("/config", authMiddleware, configController.Put)
			api.GET("/settings", authMiddleware, configController.GetSettings)
			api.GET("/ai-settings", authMiddleware, aiController.Get)
			api.PUT("/ai-settings", authMiddleware, aiController.Put)
			api.POST("/ai-settings/models", authMiddleware, aiController.Models)
			api.POST("/ai-settings/test", authMiddleware, aiController.Test)
			api.PUT("/footer-settings", authMiddleware, configController.PutFooter)
			api.PUT("/appearance", authMiddleware, configController.PutAppearance)
			api.PUT("/settings", authMiddleware, configController.PutSettings)
		}
		// Statistics
		{

			rosters := &controller.VoyageRostersController{}
			addressBook := &controller.VoyageAddressBookController{}
			api.GET("/voyage/address-book", authMiddleware, addressBook.List)
			api.GET("/voyage/address-book/:uid", authMiddleware, addressBook.Detail)
			api.POST("/voyage/address-book/:uid/action", authMiddleware, addressBook.Action)
			biliRosters := controller.NewVoyageBilibiliController()
			messages := controller.NewBilibiliMessagesController(biliRosters)
			if err := messages.Recover(); err != nil {
				logrus.Fatal("Private message recovery failed")
			}
			api.GET("/bilibili/messages", authMiddleware, messages.List)
			api.GET("/bilibili/messages/monitor", authMiddleware, messages.Monitor)
			api.POST("/bilibili/messages", authMiddleware, messages.Prepare)
			api.POST("/bilibili/messages/:queue/test", authMiddleware, messages.Test)
			api.POST("/bilibili/messages/:queue/action", authMiddleware, messages.Action)
			api.POST("/bilibili/messages/:queue/retry", authMiddleware, messages.Retry)

			api.GET("/voyage/bilibili/session", authMiddleware, biliRosters.Status)
			api.POST("/voyage/bilibili/session", authMiddleware, biliRosters.Connect)
			api.POST("/voyage/bilibili/session/poll", authMiddleware, biliRosters.Poll)
			api.DELETE("/voyage/bilibili/session", authMiddleware, biliRosters.Disconnect)
			api.POST("/voyage/bilibili/rosters/:roster/fetch", authMiddleware, biliRosters.Fetch)
			api.DELETE("/voyage/bilibili/job", authMiddleware, biliRosters.Cancel)
			batches := &controller.VoyageBatchesController{}
			api.GET("/voyage/rosters", authMiddleware, rosters.List)
			api.POST("/voyage/rosters", authMiddleware, rosters.Save)
			api.PUT("/voyage/rosters/:roster", authMiddleware, rosters.Save)
			api.GET("/voyage/rosters/:roster/members", authMiddleware, rosters.Members)
			api.POST("/voyage/rosters/:roster/members/preview", authMiddleware, rosters.Import)
			api.POST("/voyage/rosters/:roster/members/import", authMiddleware, rosters.Import)
			api.POST("/voyage/rosters/:roster/members/delete", authMiddleware, rosters.DeleteMembers)
			api.PUT("/voyage/rosters/:roster/members/:member", authMiddleware, rosters.Update)
			fulfillment := &controller.VoyageFulfillmentController{}
			api.GET("/voyage/fulfillment", authMiddleware, fulfillment.List)
			api.GET("/voyage/fulfillment/pending", authMiddleware, fulfillment.Pending)
			api.POST("/voyage/fulfillment", authMiddleware, fulfillment.Create)
			api.POST("/voyage/fulfillment/:id/parcels", authMiddleware, fulfillment.Change)
			api.GET("/voyage/batches", authMiddleware, batches.List)
			api.POST("/voyage/batches", authMiddleware, batches.Save)
			api.PUT("/voyage/batches/:id", authMiddleware, batches.Save)
			api.POST("/voyage/batches/:id/sync", authMiddleware, batches.Sync)
			api.GET("/voyage/collections", voyageAccessMiddleware, batches.Public)
			api.GET("/voyage", voyageAccessMiddleware, voyageController.List)
			api.POST("/voyage/issues/:id/claim", voyageAccessMiddleware, voyageDelivery.Claim)
			api.GET("/voyage/issues/:id/shipments", authMiddleware, voyageDelivery.List)
			api.POST("/voyage/issues/:id/shipments/batch", authMiddleware, voyageDelivery.Batch)
			api.POST("/voyage/issues/:id/shipments/export", authMiddleware, voyageDelivery.Export)
			api.PUT("/voyage/issues/:id/recipients/:recipient/shipment", authMiddleware, voyageDelivery.Ship)
			api.GET("/voyage/issues", authMiddleware, voyageController.AdminList)
			api.POST("/voyage/issues", authMiddleware, voyageController.Save)
			api.PUT("/voyage/issues/:id", authMiddleware, voyageController.Save)
			api.DELETE("/voyage/issues/:id", authMiddleware, voyageController.Delete)
			api.GET("/voyage/issues/:id/recipients", authMiddleware, voyageRecipients.List)
			api.POST("/voyage/issues/:id/recipients/preview", authMiddleware, voyageRecipients.Import)
			api.POST("/voyage/issues/:id/recipients/import", authMiddleware, voyageRecipients.Import)
			api.POST("/voyage/issues/:id/recipients/:recipient/profile", authMiddleware, voyageRecipients.Profile)
			api.PUT("/voyage/issues/:id/recipients/:recipient", authMiddleware, voyageRecipients.Update)
			api.GET("/voyage/issues/:id/recipients/:recipient/code", authMiddleware, voyageRecipients.Code)
			api.GET("/voyage/issues/:id/dispatch", authMiddleware, voyageRecipients.Dispatch)
			api.PUT("/voyage/issues/:id/dispatch", authMiddleware, voyageRecipients.SaveDispatch)
			api.GET("/voyage-assets/:filename", voyageAccessMiddleware, voyageController.File)
			api.GET("/voyage-assets", authMiddleware, voyageController.Assets)
			api.POST("/voyage-assets", authMiddleware, voyageController.Upload)
		}
		// Statistics
		{
			api.GET("/statistics", authMiddleware, statisticsController.Get)
		}
	}
	address := viper.GetString("server.host") + ":" + strconv.Itoa(viper.GetInt("server.port"))
	logrus.Info(address)
	server := &http.Server{Addr: address, Handler: r, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Minute, WriteTimeout: 15 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stopping)
	go func() {
		<-stopping
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if server.Shutdown(ctx) != nil {
			_ = server.Close()
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logrus.Error(err)
	}
}

func newSessionStore(secret string, maxAge int) cookie.Store {
	store := cookie.NewStore([]byte(secret))
	// Options alone changes browser age, not securecookie's server-side decoder age.
	store.(interface{ MaxAge(int) }).MaxAge(maxAge)
	return store
}
