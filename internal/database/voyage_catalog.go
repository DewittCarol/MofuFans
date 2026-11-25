package database

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// MigrateVoyageCatalogs separates old mixed catalogs into monthly documents.
// The first document retains its ID; unrelated batch credential scopes are never touched.
// All replacements and new documents are committed together, and reruns are no-ops.
func MigrateVoyageCatalogs(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var old []VoyageIssue
		if err := tx.Where("kind = ? OR kind = ?", "catalog", "").Order("id").Find(&old).Error; err != nil {
			return err
		}
		for _, original := range old {
			month := original.Month
			if _, err := time.Parse("2006-01", month); err != nil {
				if original.CreatedAt.IsZero() {
					return fmt.Errorf("catalog %d has no month or creation date", original.ID)
				}
				month = original.CreatedAt.In(time.FixedZone("CST", 8*60*60)).Format("2006-01")
			}
			type group struct {
				kind, title string
				gifts       []VoyageGift
			}
			guard := group{kind: "guard", title: original.Title, gifts: []VoyageGift{}}
			lotteries := []group{}
			byEvent := map[string]int{}
			assigned := map[string]bool{}
			for _, gift := range original.Gifts {
				for _, image := range gift.Images {
					assigned[image] = true
				}
				isGuard := gift.Section == "guard" || (gift.Section == "" && len(gift.Tiers) > 0)
				if isGuard {
					gift.Section, gift.Event, gift.Audience = "guard", "", ""
					guard.gifts = append(guard.gifts, gift)
				} else {
					title := strings.TrimSpace(gift.Event)
					if title == "" || title == "月度回馈" {
						title = original.Title
					}
					position, exists := byEvent[title]
					if !exists {
						position = len(lotteries)
						byEvent[title] = position
						lotteries = append(lotteries, group{kind: "lottery", title: title, gifts: []VoyageGift{}})
					}
					gift.Section, gift.Event, gift.Tiers = "other", "", []string{}
					lotteries[position].gifts = append(lotteries[position].gifts, gift)
				}
			}
			groups := []group{}
			if len(guard.gifts) > 0 || len(lotteries) == 0 {
				groups = append(groups, guard)
			}
			groups = append(groups, lotteries...)
			posters := []string{}
			for _, image := range original.Images {
				if !assigned[image] {
					posters = append(posters, image)
				}
			}
			for index, part := range groups {
				item := original
				item.Kind, item.Month, item.Title, item.Gifts, item.Images = part.kind, month, part.title, part.gifts, posters
				if index == 0 {
					item.Version = original.Version + 1
					if err := tx.Model(&VoyageIssue{}).Where("id = ?", original.ID).Select("kind", "month", "title", "gifts", "images", "version", "updated_at").Updates(&item).Error; err != nil {
						return err
					}
				} else {
					item.ID, item.Version = 0, 1
					if err := tx.Create(&item).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}
