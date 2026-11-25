package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mofufans/internal/ai"
	"mofufans/internal/database"
	"mofufans/internal/secret"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, h http.HandlerFunc) (*Worker, database.Question, *httptest.Server) {
	t.Helper()
	viper.Set("security.bilibili_cookie_key", strings.Repeat("test-only-", 5))
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "review.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&database.Question{}, &database.Tag{}, &database.Config{}, &database.QuestionModeration{}); e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	options := ai.Default()
	options.Enabled = true
	options.Protocol = "openai_chat"
	options.BaseURL = srv.URL
	options.Model = "test-model"
	raw, _ := json.Marshal(options)
	key, e := secret.EncryptCookie("test-api-key")
	if e != nil {
		t.Fatal(e)
	}
	db.Create(&database.Config{AIConfig: string(raw), AIKeyCiphertext: key, SpamPrompt: "Test rules"})
	db.Create(&database.Tag{BaseModel: database.BaseModel{ID: 1}, TagName: "Test topic"})
	q := database.Question{Content: "test submission", TagID: 1}
	db.Create(&q)
	return &Worker{DB: db, Client: ai.NewClient()}, q, srv
}
func answer(w http.ResponseWriter, spam bool) {
	fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"is_spam\":%v,\"reason\":\"Test reason\"}"}}]}`, spam)
}
func getResult(t *testing.T, w *Worker, id uint) (database.Question, database.QuestionModeration) {
	t.Helper()
	var q database.Question
	var r database.QuestionModeration
	if e := w.DB.First(&q, id).Error; e != nil {
		t.Fatal(e)
	}
	if e := w.DB.First(&r, "question_id = ?", id).Error; e != nil {
		t.Fatal(e)
	}
	return q, r
}
func TestQueueIsNonBlockingAndDeduplicates(t *testing.T) {
	calls := 0
	w, q, _ := fixture(t, func(rw http.ResponseWriter, r *http.Request) { calls++; answer(rw, true) })
	q.ImagesNum = 1
	q.Images = "upload-img/missing.jpg"
	w.DB.Save(&q)
	if ok, e := Queue(w.DB, q.ID, false); e != nil || !ok {
		t.Fatal(ok, e)
	}
	if calls != 0 {
		t.Fatal("submission invoked upstream synchronously")
	}
	if ok, e := Queue(w.DB, q.ID, true); e != nil || ok {
		t.Fatal("duplicate queued", ok, e)
	}
	if !w.ProcessOne(context.Background()) {
		t.Fatal("job not claimed")
	}
	saved, result := getResult(t, w, q.ID)
	if !saved.IsSpam || result.Status != "blocked" || result.Reason != "Test reason" || result.ImageStatus != "disabled" || calls != 1 {
		t.Fatalf("bad review: %+v", result)
	}
	if w.ProcessOne(context.Background()) {
		t.Fatal("completed job repeated")
	}
}
func TestProviderFailurePreservesQuestionAndCanRetry(t *testing.T) {
	fail := true
	w, q, _ := fixture(t, func(rw http.ResponseWriter, r *http.Request) {
		if fail {
			rw.WriteHeader(402)
			fmt.Fprint(rw, "secret-server-body")
			return
		}
		answer(rw, false)
	})
	Queue(w.DB, q.ID, false)
	w.ProcessOne(context.Background())
	saved, result := getResult(t, w, q.ID)
	if saved.IsSpam || result.Status != "failed" || !strings.Contains(result.Error, "额度") || strings.Contains(result.Error, "secret") {
		t.Fatalf("failure mishandled: %+v", result)
	}
	fail = false
	Queue(w.DB, q.ID, true)
	w.ProcessOne(context.Background())
	_, result = getResult(t, w, q.ID)
	if result.Status != "passed" || result.Attempts != 2 {
		t.Fatalf("retry failed: %+v", result)
	}
}
func TestManualDecisionWinsOverLateResponseAndRetry(t *testing.T) {
	started, release := make(chan struct{}, 1), make(chan struct{})
	w, q, _ := fixture(t, func(rw http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		answer(rw, true)
	})
	Queue(w.DB, q.ID, false)
	done := make(chan struct{})
	go func() { w.ProcessOne(context.Background()); close(done) }()
	<-started
	if e := w.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&q).Update("is_spam", false).Error; e != nil {
			return e
		}
		return MarkManual(tx, q.ID)
	}); e != nil {
		t.Fatal(e)
	}
	close(release)
	<-done
	saved, result := getResult(t, w, q.ID)
	if saved.IsSpam || result.Status != "manual" || !result.DecisionLocked {
		t.Fatalf("manual overwritten: %+v", result)
	}
	Queue(w.DB, q.ID, true)
	w.ProcessOne(context.Background())
	saved, result = getResult(t, w, q.ID)
	if saved.IsSpam || result.Status != "blocked" || !result.DecisionLocked {
		t.Fatalf("retry changed manual decision: %+v", result)
	}
}
func TestSettingsChangedDuringCallInvalidatesResult(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	w, q, _ := fixture(t, func(rw http.ResponseWriter, r *http.Request) { close(started); <-release; answer(rw, true) })
	Queue(w.DB, q.ID, false)
	done := make(chan struct{})
	go func() { w.ProcessOne(context.Background()); close(done) }()
	<-started
	w.DB.Model(&database.Config{}).Where("id > 0").Update("spam_prompt", "new rules")
	close(release)
	<-done
	saved, result := getResult(t, w, q.ID)
	if saved.IsSpam || result.Status != "failed" || !strings.Contains(result.Error, "设置已变更") {
		t.Fatalf("stale result: %+v", result)
	}
}
func TestTimeoutAndPublishedPostsRemainAvailable(t *testing.T) {
	w, q, _ := fixture(t, func(rw http.ResponseWriter, r *http.Request) { time.Sleep(100 * time.Millisecond) })
	q.IsPublish = true
	w.DB.Save(&q)
	w.Client.HTTP.Timeout = 20 * time.Millisecond
	Queue(w.DB, q.ID, false)
	w.ProcessOne(context.Background())
	saved, result := getResult(t, w, q.ID)
	if !saved.IsPublish || saved.IsSpam || result.Status != "failed" || !result.DecisionLocked {
		t.Fatalf("timeout changed public state: %+v", result)
	}
}

func TestExpiredJobAndMissingImageFailWithoutLosingPost(t *testing.T) {
	calls := 0
	w, q, _ := fixture(t, func(rw http.ResponseWriter, r *http.Request) { calls++; answer(rw, true) })
	Queue(w.DB, q.ID, false)
	now := time.Now().UTC()
	w.DB.Model(&database.QuestionModeration{}).Where("question_id = ?", q.ID).Updates(map[string]any{"status": "running", "updated_at": now.Add(-5 * time.Minute)})
	w.RecoverExpired(now)
	saved, result := getResult(t, w, q.ID)
	if result.Status != "failed" || saved.IsSpam || calls != 0 {
		t.Fatal("expired job changed question or retried provider")
	}
	var config database.Config
	w.DB.First(&config)
	o, _ := config.AIOptions()
	o.ReviewImages = true
	raw, _ := json.Marshal(o)
	w.DB.Model(&config).Update("ai_config", string(raw))
	q.ImagesNum = 1
	q.Images = "upload-img/12345678901234567890123456789012.png"
	w.DB.Save(&q)
	viper.Set("upload_directory", t.TempDir())
	t.Cleanup(func() { viper.Set("upload_directory", "") })
	Queue(w.DB, q.ID, true)
	w.ProcessOne(context.Background())
	saved, result = getResult(t, w, q.ID)
	if result.Status != "failed" || result.ImageStatus != "failed" || saved.IsSpam || calls != 0 {
		t.Fatalf("missing image mistaken for pass: %+v", result)
	}
}
