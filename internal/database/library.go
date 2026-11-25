package database

type Song struct {
	BaseModel
	Title     string    `gorm:"size:160;not null" json:"title"`
	Artist    string    `gorm:"size:160" json:"artist"`
	Performed bool      `gorm:"not null;default:false;index" json:"performed"`
	Videos    []string  `gorm:"serializer:json;type:text" json:"videos"`
	Tags      []SongTag `gorm:"many2many:song_tag_links;" json:"tags"`
}

type SongTag struct {
	BaseModel
	Kind    string `gorm:"size:16;not null;uniqueIndex:idx_song_tag_key" json:"kind"`
	Name    string `gorm:"size:80;not null" json:"name"`
	NameKey string `gorm:"size:80;not null;uniqueIndex:idx_song_tag_key" json:"-"`
}

type SongTagLink struct {
	SongID    uint `gorm:"primaryKey"`
	SongTagID uint `gorm:"primaryKey;index"`
}

type GalleryWork struct {
	SourceURL string `gorm:"size:500" json:"source_url"`
	Credit    string `gorm:"size:300" json:"credit"`
	BaseModel
	Title   string         `gorm:"size:160" json:"title"`
	Source  string         `gorm:"size:16;not null;index" json:"source"`
	Status  string         `gorm:"size:16;not null;index" json:"status"`
	Version int            `gorm:"not null;default:1" json:"version"`
	Images  []GalleryImage `gorm:"foreignKey:WorkID" json:"images"`
}

type GalleryImage struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	WorkID   uint   `gorm:"not null;index" json:"-"`
	Filename string `gorm:"size:80;uniqueIndex;not null" json:"-"`
	URL      string `gorm:"-" json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}
