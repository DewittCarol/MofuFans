package database

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAddressUpgradePreservesLegacySnapshotsAndConsent(t *testing.T) {
	for _, legacyConsent := range []bool{false, true} {
		t.Run(map[bool]string{false: "without-consent", true: "existing-consent"}[legacyConsent], func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "upgrade.sqlite")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			conn, _ := db.DB()
			defer conn.Close()
			statements := []string{
				"CREATE TABLE voyage_deliveries (recipient_id integer PRIMARY KEY,address_encrypted text,courier text,tracking text,shipping_note text,submitted_at datetime,shipped_at datetime,version integer NOT NULL DEFAULT 1)",
				"INSERT INTO voyage_deliveries (recipient_id,address_encrypted,courier,tracking,version) VALUES (17,'offline-snapshot','courier','tracking',8)",
			}
			if legacyConsent {
				statements = append(statements,
					"CREATE TABLE voyage_long_term_addresses (uid text PRIMARY KEY NOT NULL,address_encrypted text NOT NULL,enabled numeric NOT NULL DEFAULT false,version integer NOT NULL DEFAULT 1,updated_at datetime)",
					"INSERT INTO voyage_long_term_addresses (uid,address_encrypted,enabled,version) VALUES ('12345','offline-consent',1,4)",
				)
			}
			for _, sql := range statements {
				if err = db.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				if err = db.AutoMigrate(&VoyageDelivery{}, &VoyageLongTermAddress{}, &VoyageAddressBookEntry{}, &VoyageAddressBookUse{}, &VoyageAddressConsent{}); err != nil {
					t.Fatal(err)
				}
			}
			var delivery VoyageDelivery
			if err = db.First(&delivery, "recipient_id = ?", 17).Error; err != nil {
				t.Fatal(err)
			}
			if delivery.AddressEncrypted != "offline-snapshot" || delivery.Courier != "courier" || delivery.Tracking != "tracking" || delivery.Version != 8 {
				t.Fatal("existing snapshot changed")
			}
			var count int64
			db.Model(&VoyageLongTermAddress{}).Count(&count)
			if !legacyConsent && count != 0 {
				t.Fatal("upgrade opted a user in")
			}
			if legacyConsent {
				var consent VoyageLongTermAddress
				db.First(&consent, "uid = ?", "12345")
				if count != 1 || !consent.Enabled || consent.Version != 4 || consent.AddressEncrypted != "offline-consent" {
					t.Fatal("existing consent changed")
				}
			}
		})
	}
}
