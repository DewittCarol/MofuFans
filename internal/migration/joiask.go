// Package migration imports browser-exported JoiAsk data. It never contacts the source site.
package migration

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"mofufans/internal/database"
	"mofufans/internal/storage"
)

const MaxPackageBytes = 90 << 20
const Source = "quanrongmofu.com"
const LocalSource = "local-mofuask"

type Tag struct {
	ID          uint   `json:"id"`
	Name        string `json:"tag_name"`
	Description string `json:"description"`
}
type Question struct {
	ID         uint      `json:"id"`
	TagID      uint      `json:"tag_id"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Images     []string  `json:"image_assets"`
	Likes      int       `json:"likes"`
	IsHide     *bool     `json:"is_hide"`
	IsRainbow  *bool     `json:"is_rainbow"`
	IsArchive  *bool     `json:"is_archive"`
	IsPublish  *bool     `json:"is_publish"`
	IsSpam     *bool     `json:"is_spam"`
	IsRealName bool      `json:"is_real_name"`
	UID        string    `json:"bilibili_uid"`
	Name       string    `json:"bilibili_name"`
	Avatar     string    `json:"avatar_asset"`
	Emojis     string    `json:"emojis"`
}
type Omission struct {
	ID     uint   `json:"id"`
	Reason string `json:"reason"`
}
type Package struct {
	Format       string            `json:"format"`
	Version      int               `json:"version"`
	Source       string            `json:"source"`
	ExportedAt   time.Time         `json:"exported_at"`
	Batch        string            `json:"batch"`
	Part         int               `json:"part"`
	Parts        int               `json:"parts"`
	Total        int               `json:"total_questions"`
	IncludesSpam bool              `json:"includes_spam"`
	Omitted      []Omission        `json:"omitted_questions"`
	Warnings     []string          `json:"warnings"`
	Tags         []Tag             `json:"tags"`
	Questions    []Question        `json:"questions"`
	Assets       map[string]string `json:"assets"` // SHA-256 -> base64 image bytes
}
type Asset struct {
	Data      []byte
	Extension string
}
type Prepared struct {
	Package
	AssetsDecoded map[string]Asset
	Digest        string
}
type Preview struct {
	Digest           string     `json:"digest"`
	Source           string     `json:"source"`
	ExportedAt       time.Time  `json:"exported_at"`
	Batch            string     `json:"batch"`
	Part             int        `json:"part"`
	Parts            int        `json:"parts"`
	Total            int        `json:"total_questions"`
	Questions        int        `json:"questions"`
	New              int        `json:"new_questions"`
	Skipped          int        `json:"skipped_questions"`
	Published        int        `json:"published"`
	Archived         int        `json:"archived"`
	Spam             int        `json:"spam"`
	Images           int        `json:"images"`
	NewTags          []string   `json:"new_tags"`
	Warnings         []string   `json:"warnings"`
	Omitted          []Omission `json:"omitted_questions"`
	UnknownReactions []string   `json:"unknown_reactions"`
}

func Decode(reader io.Reader) (*Prepared, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, MaxPackageBytes+1))
	if err != nil || len(raw) > MaxPackageBytes {
		return nil, errors.New("读取失败或文件超过 90 MB")
	}
	var p Package
	if json.Unmarshal(raw, &p) != nil {
		return nil, errors.New("文件不是有效的 MofuAsk 迁移包 JSON")
	}
	if p.Format != "mofuask-joiask-export" || p.Version != 1 || (p.Source != Source && p.Source != LocalSource) {
		return nil, errors.New("迁移包版本或来源不受支持，请使用本页提供的导出脚本")
	}
	if p.ExportedAt.IsZero() || len(p.Batch) == 0 || len(p.Batch) > 80 || p.Part < 1 || p.Part > p.Parts || p.Parts > 10000 || p.Total < len(p.Questions) || p.Total > 100000 || len(p.Questions) == 0 || len(p.Questions) > 2000 || len(p.Tags) > 5000 || len(p.Warnings) > 1000 || len(p.Omitted) > 100000 {
		return nil, errors.New("迁移包数量、时间或分卷信息无效")
	}
	tags := map[uint]bool{}
	for _, tag := range p.Tags {
		if tag.ID == 0 || tags[tag.ID] || strings.TrimSpace(tag.Name) == "" || len(tag.Name) > 240 || len(tag.Description) > 10000 {
			return nil, errors.New("话题编号、名称重复或格式无效")
		}
		tags[tag.ID] = true
	}
	used, ids := map[string]bool{}, map[uint]bool{}
	for i := range p.Questions {
		q := &p.Questions[i]
		bad := func(reason string) (*Prepared, error) { return nil, fmt.Errorf("旧站投稿 #%d：%s", q.ID, reason) }
		if q.ID == 0 || q.ID > 9007199254740991 || ids[q.ID] {
			return bad("编号无效或重复")
		}
		ids[q.ID] = true
		if !tags[q.TagID] {
			return bad("缺少对应话题，请重新导出")
		}
		if q.IsHide == nil || q.IsArchive == nil || q.IsPublish == nil || q.IsRainbow == nil || q.IsSpam == nil {
			return bad("缺少审核或展示状态，不能推断是否允许公开")
		}
		if q.CreatedAt.IsZero() || q.UpdatedAt.IsZero() || len(q.Content) > 100000 || len(q.Images) > 6 || q.Likes < 0 || q.Likes > 2147483647 {
			return bad("时间、正文、附图数量或点赞数无效")
		}
		if len(q.Name) > 255 {
			return bad("昵称过长")
		}
		if q.IsRealName {
			uid, err := strconv.ParseInt(q.UID, 10, 64)
			if err != nil || uid <= 0 {
				return bad("实名投稿缺少有效 UID")
			}
		} else {
			q.UID = ""
			q.Name = ""
			q.Avatar = ""
		}
		var reactions []struct {
			Value string `json:"value"`
			Count int    `json:"count"`
		}
		if q.Emojis == "" || q.Emojis == "null" {
			q.Emojis = "[]"
		}
		if len(q.Emojis) > 100000 || json.Unmarshal([]byte(q.Emojis), &reactions) != nil || len(reactions) > 500 {
			return bad("互动表情记录格式无效")
		}
		seen := map[string]bool{}
		for _, r := range reactions {
			if r.Value == "" || len(r.Value) > 512 || r.Count < 0 || r.Count > 2147483647 || seen[r.Value] {
				return bad("互动表情值或计数无效")
			}
			seen[r.Value] = true
		}
		refs := append([]string{}, q.Images...)
		if q.Avatar != "" {
			refs = append(refs, q.Avatar)
		}
		for _, ref := range refs {
			if len(ref) != 64 {
				return bad("图片索引无效")
			}
			if _, ok := p.Assets[ref]; !ok {
				return bad("图片未包含在此分卷中，请重新导出")
			}
			used[ref] = true
		}
	}
	decoded := map[string]Asset{}
	if len(used) != len(p.Assets) {
		return nil, errors.New("迁移包含未引用的图片资源")
	}
	for key, encoded := range p.Assets {
		if len(encoded) > (10<<20)*4/3+4 {
			return nil, errors.New("迁移图片超过 10 MB")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) > 10<<20 {
			return nil, errors.New("图片编码无效或超过 10 MB")
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != key {
			return nil, errors.New("图片校验不一致，文件可能不完整")
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 16384 || cfg.Height > 16384 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
			return nil, errors.New("图片格式或尺寸无效；仅支持 PNG/JPEG/GIF/WebP，4000 万像素以内")
		}
		if format == "jpeg" {
			format = "jpg"
		}
		decoded[key] = Asset{Data: data, Extension: format}
	}
	hash := sha256.Sum256(raw)
	return &Prepared{Package: p, AssetsDecoded: decoded, Digest: hex.EncodeToString(hash[:])}, nil
}

func (p *Prepared) Preview(db *gorm.DB) (Preview, error) {
	result := Preview{Digest: p.Digest, Source: p.Source, ExportedAt: p.ExportedAt, Batch: p.Batch, Part: p.Part, Parts: p.Parts, Total: p.Total, Questions: len(p.Questions), NewTags: []string{}, Warnings: append([]string{}, p.Warnings...), Omitted: append([]Omission{}, p.Omitted...), UnknownReactions: []string{}}
	var imported []database.QuestionImport
	if err := db.Where("source = ?", p.Source).Find(&imported).Error; err != nil {
		return result, err
	}
	ids := map[uint]bool{}
	for _, r := range imported {
		ids[r.LegacyID] = true
	}
	var tags []database.Tag
	if err := db.Find(&tags).Error; err != nil {
		return result, err
	}
	names := map[string]bool{}
	for _, t := range tags {
		names[t.TagName] = true
	}
	var catalog []database.EmojiAsset
	if err := db.Where("kind = ?", "reaction").Find(&catalog).Error; err != nil {
		return result, err
	}
	known := map[string]bool{}
	for _, e := range catalog {
		known[e.Value] = true
	}
	unknown, usedTags := map[string]bool{}, map[uint]bool{}
	for _, q := range p.Questions {
		if ids[q.ID] {
			result.Skipped++
			continue
		}
		result.New++
		usedTags[q.TagID] = true
		if *q.IsPublish && !*q.IsSpam {
			result.Published++
		}
		if *q.IsArchive {
			result.Archived++
		}
		if *q.IsSpam {
			result.Spam++
		}
		result.Images += len(q.Images)
		var records []struct {
			Value string `json:"value"`
		}
		_ = json.Unmarshal([]byte(q.Emojis), &records)
		for _, r := range records {
			if !known[r.Value] {
				unknown[r.Value] = true
			}
		}
	}
	for _, t := range p.Tags {
		if usedTags[t.ID] && !names[t.Name] {
			result.NewTags = append(result.NewTags, t.Name)
			names[t.Name] = true
		}
	}
	for v := range unknown {
		result.UnknownReactions = append(result.UnknownReactions, v)
	}
	sort.Strings(result.UnknownReactions)
	return result, nil
}

// Apply only appends questions. Existing source IDs, including deleted imports, stay untouched.
// Callers serialize imports; a DB unique index is the final protection against concurrent requests.
func (p *Prepared) Apply(db *gorm.DB, store storage.Storage) (Preview, error) {
	var result Preview
	var uploaded []string
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return result, err
	}
	prefix := "migration-" + hex.EncodeToString(nonce) + "-"
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = p.Preview(tx)
		if err != nil {
			return err
		}
		tagIDs := map[uint]uint{}
		sourceTags := map[uint]Tag{}
		for _, t := range p.Tags {
			sourceTags[t.ID] = t
		}
		for _, q := range p.Questions {
			var existing int64
			if err = tx.Model(&database.QuestionImport{}).Where("source = ? AND legacy_id = ?", p.Source, q.ID).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				continue
			}
			if tagIDs[q.TagID] == 0 {
				t := sourceTags[q.TagID]
				local := database.Tag{TagName: t.Name, Description: t.Description}
				if err = tx.Where("tag_name = ?", t.Name).FirstOrCreate(&local).Error; err != nil {
					return err
				}
				tagIDs[q.TagID] = local.ID
			}
			upload := func(ref string) (string, error) {
				a := p.AssetsDecoded[ref]
				filename := fmt.Sprintf("%s%d-%s.%s", prefix, q.ID, ref, a.Extension)
				url, err := store.Upload(filename, bytes.NewReader(a.Data))
				if err == nil {
					uploaded = append(uploaded, filename)
				}
				return url, err
			}
			urls := []string{}
			for _, ref := range q.Images {
				url, err := upload(ref)
				if err != nil {
					return err
				}
				urls = append(urls, url)
			}
			local := database.Question{BaseModel: database.BaseModel{CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt}, TagID: int(tagIDs[q.TagID]), Content: q.Content, Images: strings.Join(urls, ";"), ImagesNum: len(urls), Likes: q.Likes, Emojis: q.Emojis, IsHide: *q.IsHide, IsRainbow: *q.IsRainbow, IsArchive: *q.IsArchive, IsPublish: *q.IsPublish, IsSpam: *q.IsSpam, IsRealName: q.IsRealName}
			if q.IsRealName {
				uid, _ := strconv.ParseInt(q.UID, 10, 64)
				local.BilibiliUID = &uid
				local.BilibiliName = q.Name
				if q.Avatar != "" {
					local.BilibiliAvatar, err = upload(q.Avatar)
					if err != nil {
						return err
					}
				}
			}
			if err = tx.Omit("Tag").Create(&local).Error; err != nil {
				return err
			}
			if err = tx.Create(&database.QuestionImport{Source: p.Source, LegacyID: q.ID, QuestionID: local.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		for _, path := range uploaded {
			_ = store.Delete(path)
		}
	}
	return result, err
}
