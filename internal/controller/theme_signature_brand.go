package controller

import (
	"mofufans/internal/database"
	"mofufans/internal/theme"
)

// Signature branding follows this site's current theme without coupling the
// shared account controller to the satellite-only theme package.
func ThemeSignatureBrand() string {
	var row database.SiteTheme
	if err := database.DB.First(&row, 1).Error; err == nil {
		if current, err := theme.Decode([]byte(row.Config)); err == nil {
			return current.Brand.SiteName
		}
	}
	return theme.Default().Brand.SiteName
}
