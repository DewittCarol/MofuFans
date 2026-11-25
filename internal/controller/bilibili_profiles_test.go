package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
)

type sharedProfileFake struct {
	calls    int
	fail     bool
	wrongUID bool
}

func (f *sharedProfileFake) PublicCard(_ context.Context, uid int64) (bilibili.Profile, error) {
	f.calls++
	if f.fail {
		return bilibili.Profile{}, errors.New("upstream failure")
	}
	if f.wrongUID {
		uid++
	}
	return bilibili.Profile{MID: uid, Name: "B站昵称", Face: "https://i1.hdslb.com/a.png"}, nil
}
func profileSetup(t *testing.T) (*BilibiliProfilesController, *sharedProfileFake) {
	appearanceDB(t)
	if err := database.InitializeBilibiliProfiles(); err != nil {
		t.Fatal(err)
	}
	f := &sharedProfileFake{}
	return &BilibiliProfilesController{Client: f, AvatarStorage: voyageAvatarFake{}, limiter: rate.NewLimiter(rate.Inf, 1)}, f
}
func TestSharedProfilesPersistAndDoNotChangeRosterOverrides(t *testing.T) {
	ctl, f := profileSetup(t)
	database.DB.AutoMigrate(&database.VoyageRosterMember{})
	member := database.VoyageRosterMember{RosterID: 1, UID: "10001", Name: "自定义名字", Tier: "captain", Version: 7}
	database.DB.Create(&member)
	p, err := ctl.lookup(context.Background(), "10001", 10001, false)
	if err != nil || p.Name != "B站昵称" || p.Avatar == "" {
		t.Fatalf("profile not fetched: %+v %v", p, err)
	}
	other := &BilibiliProfilesController{Client: f, AvatarStorage: voyageAvatarFake{}}
	if _, err = other.lookup(context.Background(), "10001", 10001, true); err != nil || f.calls != 1 {
		t.Fatal("shared persistent cache not used", err, f.calls)
	}
	database.DB.First(&member, member.ID)
	if member.Name != "自定义名字" || member.Version != 7 {
		t.Fatal("business override changed")
	}
	if err = database.InitializeBilibiliProfiles(); err != nil {
		t.Fatal(err)
	}
	var count int64
	database.DB.Model(&database.BilibiliProfile{}).Count(&count)
	if count != 1 {
		t.Fatal("migration erased cache")
	}
}
func TestSharedProfilesFailureCooldownAndStaleFallback(t *testing.T) {
	ctl, f := profileSetup(t)
	old := database.BilibiliProfile{UID: "10001", Name: "原昵称", Avatar: "/old.png", CheckedAt: time.Now().Add(-8 * 24 * time.Hour)}
	database.DB.Create(&old)
	f.fail = true
	p, err := ctl.lookup(context.Background(), "10001", 10001, false)
	if err != nil || p.Error == "" || p.RetryAt == nil || p.PauseUntil == nil || p.Name != "原昵称" || p.Avatar != "/old.png" {
		t.Fatalf("bad fallback: %+v %v", p, err)
	}
	ctl.lookup(context.Background(), "10001", 10001, false)
	ctl.lookup(context.Background(), "10002", 10002, false)
	if f.calls != 1 {
		t.Fatal("cooldown did not prevent more upstream requests")
	}
	ctl.cooldown = time.Now().Add(-time.Second)
	database.DB.Model(&old).Update("retry_at", time.Now().Add(-time.Second))
	f.fail = false
	p, err = ctl.lookup(context.Background(), "10001", 10001, false)
	if err != nil || p.Error != "" || p.RetryAt != nil || p.Name != "B站昵称" {
		t.Fatal("could not recover after cooldown")
	}
}
func TestSharedProfilesUIDValidationAndIdentityMatch(t *testing.T) {
	ctl, f := profileSetup(t)
	r := gin.New()
	r.POST("/profiles/:uid", ctl.Get)
	for _, uid := range []string{"0", "001", "-3", "10001x", "9223372036854775808"} {
		_, response := performJSONRequest(r, "POST", "/profiles/"+uid, `{}`, nil)
		if response.Code != 400 {
			t.Fatalf("accepted UID %s", uid)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid UID reached upstream")
	}
	f.wrongUID = true
	p, err := ctl.lookup(context.Background(), "10001", 10001, false)
	if err != nil || p.Error == "" || p.Name != "" || p.Avatar != "" {
		t.Fatal("mismatched profile was accepted")
	}
}

func TestSharedProfilesManualRefreshBypassesDailyCacheOnly(t *testing.T) {
	ctl, f := profileSetup(t)
	old := database.BilibiliProfile{UID: "10001", Name: "原昵称", Avatar: "/old.png", CheckedAt: time.Now().Add(-2 * time.Minute)}
	database.DB.Create(&old)
	p, err := ctl.lookup(context.Background(), "10001", 10001, false)
	if err != nil || p.Name != "原昵称" || f.calls != 0 {
		t.Fatal("ordinary lookup should use daily cache")
	}
	p, err = ctl.lookup(context.Background(), "10001", 10001, true)
	if err != nil || p.Name != "B站昵称" || p.Avatar == "/old.png" || f.calls != 1 {
		t.Fatalf("manual refresh did not update profile: %+v %v", p, err)
	}
	ctl.lookup(context.Background(), "10001", 10001, true)
	if f.calls != 1 {
		t.Fatal("repeated refresh should reuse the latest minute")
	}
	database.DB.Model(&old).Update("checked_at", time.Now().Add(-2*time.Minute))
	f.fail = true
	p, err = ctl.lookup(context.Background(), "10001", 10001, true)
	if err != nil || p.Error == "" || p.Name != "B站昵称" || p.Avatar == "" {
		t.Fatalf("refresh should preserve old profile on failure: %+v %v", p, err)
	}
	ctl.lookup(context.Background(), "10001", 10001, true)
	ctl.lookup(context.Background(), "10002", 10002, true)
	if f.calls != 2 {
		t.Fatal("manual refresh bypassed failure cooldown")
	}
}

func TestSharedProfilesRefreshRequestValidation(t *testing.T) {
	ctl, f := profileSetup(t)
	r := gin.New()
	r.POST("/profiles/:uid", ctl.Get)
	old := database.BilibiliProfile{UID: "10001", Name: "原昵称", Avatar: "/old.png", CheckedAt: time.Now().Add(-2 * time.Minute)}
	database.DB.Create(&old)
	for _, body := range []string{`{"refresh":"yes"}`, `{"refresh":`} {
		_, response := performJSONRequest(r, "POST", "/profiles/10001", body, nil)
		if response.Code != 400 {
			t.Fatalf("accepted invalid refresh input: %s", body)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid input reached upstream")
	}
	_, response := performJSONRequest(r, "POST", "/profiles/10001", `{"refresh":true}`, nil)
	if response.Code != 200 || f.calls != 1 {
		t.Fatalf("refresh flag not honored: %+v calls=%d", response, f.calls)
	}
}

func TestProfilesSevenDayCacheAndBulkReadNeverFetchUpstream(t *testing.T) {
	ctl, f := profileSetup(t)
	old := database.BilibiliProfile{UID: "10001", Name: "六天前昵称", Avatar: "/old.png", CheckedAt: time.Now().Add(-6 * 24 * time.Hour)}
	database.DB.Create(&old)
	p, err := ctl.lookup(context.Background(), "10001", 10001, false)
	if err != nil || p.Name != old.Name || f.calls != 0 {
		t.Fatal("seven day cache was bypassed")
	}
	r := gin.New()
	r.POST("/cache", ctl.Cached)
	_, response := performJSONRequest(r, "POST", "/cache", `{"uids":["10001","10002"]}`, nil)
	if response.Code != 200 || f.calls != 0 {
		t.Fatal("bulk cache read contacted upstream")
	}
	database.DB.Model(&old).Update("checked_at", time.Now().Add(-8*24*time.Hour))
	_, err = ctl.lookup(context.Background(), "10001", 10001, false)
	if err != nil || f.calls != 1 {
		t.Fatal("expired cache never refreshed")
	}
}

type profileBlockingFake struct {
	started chan struct{}
	release chan struct{}
}

func (f *profileBlockingFake) PublicCard(ctx context.Context, uid int64) (bilibili.Profile, error) {
	close(f.started)
	select {
	case <-f.release:
		return bilibili.Profile{MID: uid, Name: "新昵称", Face: "https://i1.hdslb.com/a.png"}, nil
	case <-ctx.Done():
		return bilibili.Profile{}, ctx.Err()
	}
}
func TestProfilesCachedReadDoesNotWaitForUnrelatedUpstream(t *testing.T) {
	ctl, _ := profileSetup(t)
	database.DB.Create(&database.BilibiliProfile{UID: "10001", Name: "已有昵称", Avatar: "/cached.png", CheckedAt: time.Now()})
	f := &profileBlockingFake{started: make(chan struct{}), release: make(chan struct{})}
	ctl.Client = f
	done := make(chan error, 1)
	go func() { _, err := ctl.lookup(context.Background(), "10002", 10002, false); done <- err }()
	<-f.started
	cached := make(chan error, 1)
	go func() { _, err := ctl.lookup(context.Background(), "10001", 10001, false); cached <- err }()
	select {
	case err := <-cached:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("valid cache was blocked by upstream")
	}
	close(f.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type reuseAvatarFake struct {
	calls     int
	available bool
}

func (s *reuseAvatarFake) Delete(string) error { return nil }
func (s *reuseAvatarFake) Save(context.Context, int64, string) (string, error) {
	s.calls++
	return "/local-avatar.png", nil
}
func (s *reuseAvatarFake) Available(string) bool { return s.available }
func TestProfilesReuseUnchangedAvatarOnlyWhenFileExists(t *testing.T) {
	ctl, _ := profileSetup(t)
	storage := &reuseAvatarFake{available: true}
	ctl.AvatarStorage = storage
	if _, err := ctl.lookup(context.Background(), "10001", 10001, false); err != nil {
		t.Fatal(err)
	}
	database.DB.Model(&database.BilibiliProfile{}).Where("uid = ?", "10001").Update("checked_at", time.Now().Add(-2*time.Minute))
	if _, err := ctl.lookup(context.Background(), "10001", 10001, true); err != nil || storage.calls != 1 {
		t.Fatal("unchanged avatar was downloaded again")
	}
	storage.available = false
	database.DB.Model(&database.BilibiliProfile{}).Where("uid = ?", "10001").Update("checked_at", time.Now().Add(-2*time.Minute))
	if _, err := ctl.lookup(context.Background(), "10001", 10001, true); err != nil || storage.calls != 2 {
		t.Fatal("missing avatar was not downloaded")
	}
}
