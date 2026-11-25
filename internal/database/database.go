package database

import (
	"fmt"
	"mofufans/internal/ai"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

const DefaultTagName = "提问箱"

// Init opens connection and try to initialize the database.
func Init() {
	var err error
	switch viper.GetString("db_type") {
	case "sqlite":
		log.Info("Using sqlite database.")
		dsn := viper.GetString("sqlite")
		if !strings.Contains(dsn, "_busy_timeout=") {
			separator := "?"
			if strings.Contains(dsn, "?") {
				separator = "&"
			}
			dsn += separator + "_busy_timeout=10000"
		}
		DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	case "mysql":
		log.Info("Using mysql database.")
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True", viper.GetString("mysql.user"), viper.GetString("mysql.pass"), viper.GetString("mysql.host"), viper.GetInt("mysql.port"), viper.GetString("mysql.name"))
		DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	default:
		log.Fatal("db_type must be sqlite or mysql")
	}
	if err != nil {
		log.Fatal(err)
	}
	// Preview migrations are explicit; never initialize a database merely on restart.
	if !viper.GetBool("development.skip_database_initialization") {
		initializeDB()
	}
}

// initializeDB initializes the database, create tables and default records.
func initializeDB() {
	legacyAdminRoles := DB.Migrator().HasTable(&Admin{}) && !DB.Migrator().HasColumn(&Admin{}, "is_super_admin")
	err := DB.AutoMigrate(
		&User{},
		&SignatureChallenge{},
		&BilibiliVerificationAccount{},
		&BilibiliVerificationRequest{},
		&Question{},
		&QuestionImport{},
		&LikeRecord{},
		&Admin{},
		&Config{},
		&Tag{},
		&EmojiAsset{},
		&EmojiCollection{},
		&VoyageIssue{},
		&DigitalGift{},
		&DigitalGiftFile{},
		&DigitalGiftRecipient{},
		&VoyageRecipient{},
		&BilibiliProfile{},
		&VoyageDispatchConfig{},
		&VoyageDelivery{},
		&VoyageLongTermAddress{},
		&VoyageAddressBookEntry{},
		&VoyageAddressBookUse{},
		&VoyageAddressConsent{},
		&SongTag{},
		&Song{},
		&SongTagLink{},
		&GalleryWork{},
		&GalleryImage{},
		&WebsiteAssetRequest{},
		&WebsiteAssetFile{},
		&WebsiteAssetSettings{},
	)
	if err != nil {
		log.Fatal(err)
	}
	if err := InitializeTheme(); err != nil {
		log.Fatal(err)
	}
	if err := InitializeVoyageBatches(); err != nil {
		log.Fatal("Failed to initialize voyage batches: ", err)
	}
	if err := InitializeBilibiliMessages(); err != nil {
		log.Fatal("Failed to initialize private messages: ", err)
	}
	if err := InitializeDigitalGifts(); err != nil {
		log.Fatal("Failed to initialize digital gifts: ", err)
	}
	if err := InitializeVoyageFulfillment(); err != nil {
		log.Fatal("Failed to initialize voyage fulfillment: ", err)
	}
	if err := MigrateVoyageCatalogs(DB); err != nil {
		log.Fatal("Failed to separate monthly voyage catalogs: ", err)
	}
	if err := InitializeEmojiCatalog(); err != nil {
		log.Fatal("Failed to initialize emoji catalog: ", err)
	}
	if err := InitializeSongTags(); err != nil {
		log.Fatal("Failed to initialize song tags: ", err)
	}
	// Preserve the legacy owner's role, including after the owner is renamed.
	if legacyAdminRoles {
		if err := DB.Model(&Admin{}).Where("username = ?", "admin").Update("is_super_admin", true).Error; err != nil {
			log.Fatal(err)
		}
	}
	var adminCount int64
	if err := DB.Model(&Admin{}).Count(&adminCount).Error; err != nil {
		log.Fatal(err)
	}
	if adminCount == 0 && (viper.GetString("bootstrap_admin.username") != "" || viper.GetString("bootstrap_admin.password_hash") != "") {
		username, passwordHash := viper.GetString("bootstrap_admin.username"), viper.GetString("bootstrap_admin.password_hash")
		if username == "" || passwordHash == "" {
			log.Fatal("bootstrap_admin username and password_hash must both be configured")
		}
		if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
			log.Fatal("bootstrap_admin.password_hash must be bcrypt; use browser setup for a new installation")
		}
		log.Info("Initializing administrator account.")
		if err := DB.Create(&Admin{Username: username, Password: passwordHash, IsSuperAdmin: true}).Error; err != nil {
			log.Fatal("Failed to initialize default admin account.", err)
		}
	}
	// Initialize default config.
	var config Config
	if DB.First(&config).RowsAffected == 0 {
		log.Info("Initializing default config.")
		if err := DB.Create(&Config{
			Announcement: "提问内容将在审核后公开",
			SpamPrompt:   ai.DefaultSpamPrompt,
		}).Error; err != nil {
			log.Fatal("Failed to initialize default config.", err)
		}
	} else if strings.TrimSpace(config.SpamPrompt) == "" {
		if err := DB.Model(&config).Update("spam_prompt", ai.DefaultSpamPrompt).Error; err != nil {
			log.Fatal("Failed to initialize spam prompt.", err)
		}
	}
	if err := InitializeModeration(); err != nil {
		log.Fatal(err)
	}
	// Initialize default tag.
	if DB.First(&Tag{}).RowsAffected == 0 {
		log.Info("Initializing default tag.")
		if err := DB.Create(&Tag{TagName: DefaultTagName, Description: "默认话题"}).Error; err != nil {
			log.Fatal("Failed to initialize default tag.", err)
		}
	}
	var assetSettings WebsiteAssetSettings
	if DB.First(&assetSettings).RowsAffected == 0 {
		if err := DB.Create(&WebsiteAssetSettings{Instructions: WebsiteAssetDefaultInstructions}).Error; err != nil {
			log.Fatal(err)
		}
	}
}
