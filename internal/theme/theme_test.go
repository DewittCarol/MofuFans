package theme

import (
	"encoding/json"
	"testing"
)

func TestThemeRejectsExecutableOrMalformedInput(t *testing.T) {
	if err := Validate(Default()); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"javascript:alert(1)", "//example.com/icon.png", "/\\example.com/a", "data:image/svg+xml,<svg/>", "http://example.com/icon.png", "https://user:pass@example.com/a"} {
		if AssetURL(bad) {
			t.Fatalf("unsafe asset accepted: %s", bad)
		}
	}
	for _, good := range []string{"", "/api/theme-assets/abc.png", "/mofu.png", "https://example.com/icon.png?size=64"} {
		if !AssetURL(good) {
			t.Fatalf("valid asset rejected: %s", good)
		}
	}
	invalid := []func(*Config){
		func(c *Config) { c.Colors["primary"] = "red;display:none" },
		func(c *Config) { delete(c.Colors, "background") },
		func(c *Config) { c.Colors["unknown"] = "#123456" },
		func(c *Config) { c.AdminColors["unknown"] = "#123456" },
		func(c *Config) { c.Modules["songs"] = Module{Label: "", Icon: ""} },
		func(c *Config) { c.Brand.SiteName = "" },
		func(c *Config) { c.SchemaVersion = 2 },
	}
	for i, change := range invalid {
		c := Default()
		change(&c)
		if Validate(c) == nil {
			t.Fatalf("accepted invalid theme %d", i)
		}
	}
	raw, _ := json.Marshal(Default())
	raw = append(raw, []byte(` {}`)...)
	if _, err := Decode(raw); err == nil {
		t.Fatal("accepted trailing JSON")
	}
	c := Default()
	c.AdminColors = map[string]string{}
	if Validate(c) != nil {
		t.Fatal("inheritance must be valid")
	}
}
