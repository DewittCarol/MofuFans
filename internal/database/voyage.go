package database

// Gifts are saved as one document with their issue, so a publication is atomic.
type VoyageIssue struct {
	BaseModel
	Kind         string       `gorm:"size:16;not null;default:catalog;index" json:"kind,omitempty"`
	Title        string       `gorm:"size:160" json:"title"`
	Month        string       `gorm:"size:7;index" json:"month"`
	Images       []string     `gorm:"serializer:json;type:text" json:"images"`
	AssetImages  []string     `gorm:"serializer:json;type:text" json:"-"`
	ShippingNote string       `gorm:"type:text" json:"shipping_note"`
	Note         string       `gorm:"type:text" json:"note"`
	Published    bool         `gorm:"not null;default:false;index" json:"published"`
	Version      int          `gorm:"not null;default:1" json:"version"`
	Gifts        []VoyageGift `gorm:"serializer:json;type:text" json:"gifts"`
}

type VoyageGift struct {
	Section      string   `json:"section"`
	Event        string   `json:"event"`
	Audience     string   `json:"audience"`
	ShippingNote string   `json:"shipping_note"`
	Name         string   `json:"name"`
	Tiers        []string `json:"tiers"`
	Description  string   `json:"description"`
	Images       []string `json:"images"`
}
