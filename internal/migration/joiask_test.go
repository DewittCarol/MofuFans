package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mofufans/internal/database"
)

func ptr(v bool) *bool { return &v }
func fixture(t *testing.T) Package {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b.Bytes())
	key := hex.EncodeToString(h[:])
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return Package{Format: "mofuask-joiask-export", Version: 1, Source: Source, ExportedAt: now, Batch: "offline-fixture", Part: 1, Parts: 1, Total: 1, Tags: []Tag{{ID: 999, Name: "旧站话题", Description: "描述"}}, Assets: map[string]string{key: base64.StdEncoding.EncodeToString(b.Bytes())}, Questions: []Question{{ID: 42, TagID: 999, Content: "离线测试 <script>not executed</script>", CreatedAt: now, UpdatedAt: now, Images: []string{key}, IsHide: ptr(true), IsPublish: ptr(false), IsArchive: ptr(true), IsRainbow: ptr(true), IsSpam: ptr(false), Likes: 7, Emojis: `[{"value":"👍","count":3}]`}}}
}
func decode(t *testing.T, p Package) *Prepared {
	t.Helper()
	data, _ := json.Marshal(p)
	result, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func databaseForTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "offline.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&database.Tag{}, &database.Question{}, &database.QuestionImport{}, &database.EmojiAsset{}); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.DB()
	t.Cleanup(func() { raw.Close() })
	return db
}

type memoryStore struct {
	files  map[string][]byte
	failAt int
	calls  int
}

func (s *memoryStore) Upload(name string, r *bytes.Reader) (string, error) {
	s.calls++
	if s.failAt > 0 && s.calls == s.failAt {
		return "", errors.New("simulated disk failure")
	}
	data := make([]byte, r.Len())
	_, _ = r.Read(data)
	s.files[name] = data
	return "upload-img/" + name, nil
}
func (s *memoryStore) Delete(name string) error { delete(s.files, name); return nil }

func TestImportPreservesDataMapsIDsAndIsIdempotent(t *testing.T) {
	db := databaseForTest(t)
	store := &memoryStore{files: map[string][]byte{}}
	tag := database.Tag{TagName: "旧站话题", Description: "新站保留描述"}
	db.Create(&tag)
	existing := database.Question{Content: "新站投稿不能覆盖", TagID: int(tag.ID)}
	db.Create(&existing)
	p := decode(t, fixture(t))
	preview, err := p.Preview(db)
	if err != nil || preview.New != 1 || preview.Images != 1 || preview.Published != 0 || len(preview.NewTags) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	var count int64
	db.Model(&database.Question{}).Count(&count)
	if count != 1 || len(store.files) != 0 {
		t.Fatal("preview wrote data")
	}
	result, err := p.Apply(db, store)
	if err != nil || result.New != 1 {
		t.Fatalf("import: %+v %v", result, err)
	}
	var row database.Question
	db.Where("id <> ?", existing.ID).First(&row)
	if row.ID == 42 || row.TagID != int(tag.ID) || row.IsPublish || !row.IsArchive || !row.IsHide || !row.IsRainbow || row.Likes != 7 || row.Emojis != p.Questions[0].Emojis || !row.CreatedAt.Equal(p.Questions[0].CreatedAt) || !row.UpdatedAt.Equal(p.Questions[0].UpdatedAt) || !strings.HasPrefix(row.Images, "upload-img/migration-") || row.ImagesNum != 1 {
		t.Fatalf("data changed: %+v", row)
	}
	row.Content = "导入后修改"
	db.Save(&row)
	result, err = p.Apply(db, store)
	if err != nil || result.New != 0 || result.Skipped != 1 || len(store.files) != 1 {
		t.Fatalf("retry not idempotent: %+v %v", result, err)
	}
	db.First(&row, row.ID)
	if row.Content != "导入后修改" {
		t.Fatal("overwrote moderated question")
	}
	db.Delete(&row)
	result, err = p.Apply(db, store)
	if err != nil || result.New != 0 {
		t.Fatal("deleted question resurrected")
	}
	db.First(&tag, tag.ID)
	if tag.Description != "新站保留描述" {
		t.Fatal("overwrote tag")
	}
}

func TestRejectedPackagesNeverReachDatabase(t *testing.T) {
	for name, mutate := range map[string]func(*Package){
		"wrong origin":       func(p *Package) { p.Source = "evil.invalid" },
		"unknown format":     func(p *Package) { p.Version = 2 },
		"missing moderation": func(p *Package) { p.Questions[0].IsPublish = nil },
		"missing topic":      func(p *Package) { p.Questions[0].TagID = 12 },
		"missing picture":    func(p *Package) { p.Assets = nil },
		"bad checksum": func(p *Package) {
			for k := range p.Assets {
				p.Assets[k] = base64.StdEncoding.EncodeToString([]byte("not a picture"))
			}
		},
		"duplicate ID":          func(p *Package) { p.Questions = append(p.Questions, p.Questions[0]); p.Total++ },
		"invalid reactions":     func(p *Package) { p.Questions[0].Emojis = `[{"value":"👍","count":-1}]` },
		"real name without UID": func(p *Package) { p.Questions[0].IsRealName = true },
	} {
		t.Run(name, func(t *testing.T) {
			p := fixture(t)
			mutate(&p)
			raw, _ := json.Marshal(p)
			if _, err := Decode(bytes.NewReader(raw)); err == nil {
				t.Fatal("accepted invalid package")
			}
		})
	}
	if _, err := Decode(strings.NewReader(`<html>login</html>`)); err == nil {
		t.Fatal("accepted html")
	}
}

func TestUploadFailureRollsBackWholePartAndRemovesUploadedAssets(t *testing.T) {
	db := databaseForTest(t)
	store := &memoryStore{files: map[string][]byte{}, failAt: 2}
	pkg := fixture(t)
	second := pkg.Questions[0]
	second.ID = 43
	pkg.Questions = append(pkg.Questions, second)
	pkg.Total = 2
	_, err := decode(t, pkg).Apply(db, store)
	if err == nil {
		t.Fatal("expected simulated failure")
	}
	for _, model := range []any{&database.Question{}, &database.QuestionImport{}, &database.Tag{}} {
		var count int64
		db.Model(model).Count(&count)
		if count != 0 {
			t.Fatal("partial write survived rollback")
		}
	}
	if len(store.files) != 0 {
		t.Fatal("failed import left uploaded files")
	}
}

func TestLocalAndLegacyIDsAreIndependent(t *testing.T) {
	db := databaseForTest(t)
	store := &memoryStore{files: map[string][]byte{}}
	for _, source := range []string{LocalSource, Source} {
		pkg := fixture(t)
		pkg.Source = source
		prepared := decode(t, pkg)
		preview, err := prepared.Preview(db)
		if err != nil || preview.New != 1 || preview.Source != source {
			t.Fatalf("source ID collision: %+v %v", preview, err)
		}
		result, err := prepared.Apply(db, store)
		if err != nil || result.New != 1 {
			t.Fatalf("import: %+v %v", result, err)
		}
		result, err = prepared.Apply(db, store)
		if err != nil || result.Skipped != 1 || result.New != 0 {
			t.Fatalf("retry: %+v %v", result, err)
		}
	}
	var count int64
	db.Model(&database.Question{}).Count(&count)
	if count != 2 {
		t.Fatalf("wanted two independent questions, got %d", count)
	}
}
