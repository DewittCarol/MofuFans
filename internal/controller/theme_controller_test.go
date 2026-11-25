package controller

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"mofufans/internal/theme"
	"testing"
)

func TestThemeSaveConflictAndPresetIsolation(t *testing.T) {
	appearanceDB(t)
	if err := database.InitializeTheme(); err != nil {
		t.Fatal(err)
	}
	ctl := new(ThemeController)
	r := gin.New()
	r.GET("/theme", ctl.Get)
	r.PUT("/theme", ctl.Put)
	r.GET("/presets", ctl.Presets)
	r.POST("/presets", ctl.SavePreset)
	r.DELETE("/presets/:id", ctl.DeletePreset)
	call := func(method, path string, body any) testAPIResponse {
		raw, _ := json.Marshal(body)
		_, res := performJSONRequest(r, method, path, string(raw), nil)
		return res
	}
	c := theme.Default()
	c.Brand.SiteName = "另一位主播"
	c.Colors["primary"] = "#123456"
	if got := call("PUT", "/theme", gin.H{"revision": 1, "theme": c}); got.Code != 200 {
		t.Fatal(got)
	}
	if got := call("PUT", "/theme", gin.H{"revision": 1, "theme": theme.Default()}); got.Code != 409 {
		t.Fatal("stale update was accepted", got)
	}
	if got := call("POST", "/presets", gin.H{"name": "收藏", "theme": theme.Default()}); got.Code != 200 {
		t.Fatal(got)
	}
	if got := call("DELETE", "/presets/mofu", nil); got.Code != 400 {
		t.Fatal("builtin preset is mutable", got)
	}
	if err := database.InitializeTheme(); err != nil {
		t.Fatal(err)
	} // Restart cannot reset an edited theme.
	var row database.SiteTheme
	database.DB.First(&row, 1)
	current, err := theme.Decode([]byte(row.Config))
	if err != nil || current.Brand.SiteName != c.Brand.SiteName || row.Revision != 2 {
		t.Fatal("preset or restart changed active theme")
	}
	c.Colors["primary"] = "not-a-color"
	if got := call("PUT", "/theme", gin.H{"revision": 2, "theme": c}); got.Code != 400 {
		t.Fatal("invalid theme persisted", got)
	}
	database.DB.First(&row, 1)
	if row.Revision != 2 {
		t.Fatal("invalid write changed revision")
	}
	var presets []database.ThemePreset
	database.DB.Find(&presets)
	if len(presets) != 1 {
		t.Fatal("missing saved preset")
	}
}
