package appearance

import (
	"embed"
	"encoding/json"
)

//go:embed cursor-themes.json
var files embed.FS

type CursorTheme struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Manifest string `json:"manifest,omitempty"`
}

// The registry is shared by the public client and the administrator's selector.
func CursorThemes() []CursorTheme {
	data, err := files.ReadFile("cursor-themes.json")
	if err != nil {
		panic(err)
	}
	var themes []CursorTheme
	if err := json.Unmarshal(data, &themes); err != nil {
		panic(err)
	}
	return themes
}

func ValidCursorTheme(id string) bool {
	for _, theme := range CursorThemes() {
		if theme.ID == id {
			return true
		}
	}
	return false
}
