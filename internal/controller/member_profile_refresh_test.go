package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type loginAvatarStore struct {
	mu      sync.Mutex
	calls   int
	deletes int
	uid     int64
	remote  string
	stored  string
	save    func(context.Context, int64, string) (string, error)
}

func (s *loginAvatarStore) Save(ctx context.Context, uid int64, remote string) (string, error) {
	s.mu.Lock()
	s.calls++
	s.uid, s.remote = uid, remote
	s.mu.Unlock()
	if s.save != nil {
		return s.save(ctx, uid, remote)
	}
	return s.stored, nil
}

func (s *loginAvatarStore) Delete(string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return nil
}

func loginProfileFixture(t *testing.T) (*signatureRig, database.User, *loginAvatarStore) {
	t.Helper()
	s := newSignatureRig(t)
	s.seedLegacy(10001, "legacy-name")
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := database.DB.Model(&database.User{}).Where("bilibili_uid = ?", 10001).Updates(map[string]any{
		"bilibili_name": "旧昵称", "bilibili_avatar": "/upload-img/bilibili-avatar-10001-old.jpg",
		"auth_version": "profile-original-version", "signature_verified_at": old, "updated_at": old,
	}).Error; err != nil {
		t.Fatal(err)
	}
	var user database.User
	if err := database.DB.First(&user).Error; err != nil {
		t.Fatal(err)
	}
	store := &loginAvatarStore{stored: "/upload-img/bilibili-avatar-10001-new.jpg"}
	s.ctl.AvatarStorage = store
	return s, user, store
}

func assertLoginProfileResponse(t *testing.T, result testAPIResponse, uid, name, avatar string) {
	t.Helper()
	var view struct {
		UID    string `json:"bilibili_uid"`
		Name   string `json:"bilibili_name"`
		Avatar string `json:"bilibili_avatar"`
	}
	if err := json.Unmarshal(result.Data, &view); err != nil {
		t.Fatal(err)
	}
	if view.UID != uid || view.Name != name || view.Avatar != avatar {
		t.Fatalf("unexpected account view: %+v", view)
	}
}

func TestLoginProfileRefreshReturnsNewIdentityWithoutChangingCredentials(t *testing.T) {
	s, before, store := loginProfileFixture(t)
	reads := 0
	s.ctl.LoginProfileReader = signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
		reads++
		if uid != before.BilibiliUID {
			t.Fatalf("looked up UID %d", uid)
		}
		return bilibili.Profile{MID: uid, Name: "新昵称", Face: "https://i0.hdslb.com/bfs/face/new.jpg"}, nil
	})
	c, result := s.request("POST", "/login", `{"bilibili_uid":"00010001","password":"legacy-password"}`, nil, 200)
	assertLoginProfileResponse(t, result, "10001", "新昵称", store.stored)
	s.request("GET", "/strong", "", c, 200)
	var after database.User
	database.DB.First(&after)
	if reads != 1 || store.calls != 1 || store.uid != 10001 || store.remote != "https://i0.hdslb.com/bfs/face/new.jpg" || store.deletes != 0 {
		t.Fatal("unexpected profile/avatar requests")
	}
	if after.PasswordHash != before.PasswordHash || after.AuthVersion != before.AuthVersion || after.Username != before.Username || after.IsDisabled || after.SignatureVerifiedAt == nil || !after.SignatureVerifiedAt.Equal(*before.SignatureVerifiedAt) {
		t.Fatal("profile update changed authentication fields")
	}
}

func TestLoginProfileRefreshRunsOnEachLoginButDoesNotWriteUnchangedData(t *testing.T) {
	s, before, store := loginProfileFixture(t)
	store.stored = before.BilibiliAvatar
	reads := 0
	s.ctl.LoginProfileReader = signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
		reads++
		return bilibili.Profile{MID: uid, Name: before.BilibiliName, Face: "https://i0.hdslb.com/bfs/face/same.jpg"}, nil
	})
	for n := 0; n < 2; n++ {
		_, result := s.request("POST", "/login", `{"username":"legacy-name","password":"legacy-password"}`, nil, 200)
		assertLoginProfileResponse(t, result, "10001", before.BilibiliName, before.BilibiliAvatar)
	}
	var after database.User
	database.DB.First(&after)
	if reads != 2 || store.calls != 2 || !after.UpdatedAt.Equal(before.UpdatedAt) || store.deletes != 0 {
		t.Fatal("unchanged data was cached indefinitely, rewritten or deleted")
	}
}

func TestLoginProfileRefreshFailureRetainsOldDataAndAllowsLogin(t *testing.T) {
	for _, kind := range []string{"upstream", "wrong-uid", "invalid-name", "avatar-failure", "empty-avatar", "busy"} {
		t.Run(kind, func(t *testing.T) {
			s, before, store := loginProfileFixture(t)
			reads := 0
			s.ctl.LoginProfileReader = signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
				reads++
				if kind == "upstream" {
					return bilibili.Profile{}, errors.New("upstream unavailable")
				}
				p := bilibili.Profile{MID: uid, Name: "新昵称", Face: "https://i0.hdslb.com/bfs/face/new.jpg"}
				if kind == "wrong-uid" {
					p.MID++
				}
				if kind == "invalid-name" {
					p.Name = "  "
				}
				if kind == "empty-avatar" {
					p.Face = ""
				}
				return p, nil
			})
			if kind == "avatar-failure" {
				store.save = func(context.Context, int64, string) (string, error) {
					return "", errors.New("invalid image or download failure")
				}
			}
			if kind == "busy" {
				for n := 0; n < cap(s.ctl.Limits.network); n++ {
					s.ctl.Limits.network <- struct{}{}
				}
			}
			c, result := s.request("POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil, 200)
			name := before.BilibiliName
			if kind == "avatar-failure" || kind == "empty-avatar" {
				name = "新昵称"
			}
			assertLoginProfileResponse(t, result, "10001", name, before.BilibiliAvatar)
			s.request("GET", "/strong", "", c, 200)
			if kind == "busy" && reads != 0 {
				t.Fatal("busy refresh unexpectedly queued a lookup")
			}
			if kind != "avatar-failure" && store.calls != 0 {
				t.Fatal("invalid or unavailable profile reached avatar download")
			}
			if store.deletes != 0 {
				t.Fatal("failed refresh deleted an existing avatar")
			}
		})
	}
}

func TestLoginProfileRefreshNeverRunsForRejectedPasswordsOrAccounts(t *testing.T) {
	s, _, store := loginProfileFixture(t)
	reads := 0
	s.ctl.LoginProfileReader = signatureReadFunc(func(context.Context, int64) (bilibili.Profile, error) { reads++; return bilibili.Profile{}, nil })
	s.request("POST", "/login", `{"bilibili_uid":"10001","password":"wrong-password"}`, nil, 401)
	s.request("POST", "/login", `{"bilibili_uid":"999999","password":"legacy-password"}`, nil, 401)
	database.DB.Model(&database.User{}).Where("bilibili_uid = ?", 10001).Update("is_disabled", true)
	s.request("POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil, 403)
	if reads != 0 || store.calls != 0 || store.deletes != 0 {
		t.Fatal("rejected login queried public profile")
	}
}

func TestLoginProfileRefreshUsesAuthenticatedUIDForLegacyNumericName(t *testing.T) {
	s, _, _ := loginProfileFixture(t)
	s.seedLegacy(999, "10001")
	lookedUp := int64(0)
	s.ctl.LoginProfileReader = signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
		lookedUp = uid
		return bilibili.Profile{MID: uid, Name: "原数字登录名用户"}, nil
	})
	_, result := s.request("POST", "/login", `{"username":"10001","password":"legacy-password"}`, nil, 200)
	assertLoginProfileResponse(t, result, "999", "原数字登录名用户", "")
	if lookedUp != 999 {
		t.Fatalf("requested profile for ambiguous login text: %d", lookedUp)
	}
	var original database.User
	database.DB.Where("bilibili_uid = ?", 10001).First(&original)
	if original.BilibiliName != "旧昵称" {
		t.Fatal("numeric legacy login modified another UID")
	}
}

func TestLoginProfileRefreshSharesTwoSecondDeadlineAndFailsOpen(t *testing.T) {
	t.Run("card-timeout", func(t *testing.T) {
		s, before, store := loginProfileFixture(t)
		s.ctl.LoginProfileReader = signatureReadFunc(func(ctx context.Context, uid int64) (bilibili.Profile, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > memberProfileRefreshTimeout || time.Until(deadline) < time.Second {
				t.Fatal("profile lookup missing short deadline")
			}
			<-ctx.Done()
			return bilibili.Profile{}, ctx.Err()
		})
		_, result := s.request("POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil, 200)
		assertLoginProfileResponse(t, result, "10001", before.BilibiliName, before.BilibiliAvatar)
		if store.calls != 0 {
			t.Fatal("timed-out card caused avatar download")
		}
	})
	t.Run("avatar-shares-deadline", func(t *testing.T) {
		s, before, store := loginProfileFixture(t)
		var deadline time.Time
		s.ctl.LoginProfileReader = signatureReadFunc(func(ctx context.Context, uid int64) (bilibili.Profile, error) {
			deadline, _ = ctx.Deadline()
			return bilibili.Profile{MID: uid, Name: "新昵称", Face: "https://i0.hdslb.com/bfs/face/new.jpg"}, nil
		})
		store.save = func(ctx context.Context, _ int64, _ string) (string, error) {
			actual, ok := ctx.Deadline()
			if !ok || !actual.Equal(deadline) {
				t.Fatal("avatar got a separate extended deadline")
			}
			return "", context.DeadlineExceeded
		}
		_, result := s.request("POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil, 200)
		assertLoginProfileResponse(t, result, "10001", "新昵称", before.BilibiliAvatar)
	})
}

type loginProfileResult struct {
	recorder *httptest.ResponseRecorder
	response testAPIResponse
}

func TestLoginProfileRefreshCannotRestoreChangedCredentialsDuringNetworkRequest(t *testing.T) {
	for _, change := range []string{"password", "epoch", "disabled", "failed-reader-epoch"} {
		t.Run(change, func(t *testing.T) {
			s, before, store := loginProfileFixture(t)
			entered, resume := make(chan struct{}), make(chan struct{})
			s.ctl.LoginProfileReader = signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
				close(entered)
				<-resume
				if change == "failed-reader-epoch" {
					return bilibili.Profile{}, errors.New("network failed")
				}
				return bilibili.Profile{MID: uid, Name: "迟到的昵称", Face: "https://i0.hdslb.com/bfs/face/new.jpg"}, nil
			})
			result := make(chan loginProfileResult, 1)
			go func() {
				recorder, response := performJSONRequest(s.r, "POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil)
				result <- loginProfileResult{recorder, response}
			}()
			<-entered
			updates := map[string]any{}
			switch change {
			case "password":
				hash, _ := bcrypt.GenerateFromPassword([]byte("replacement-password"), bcrypt.MinCost)
				updates["password_hash"] = string(hash)
			case "disabled":
				updates["is_disabled"] = true
			default:
				updates["auth_version"] = "replacement-version"
			}
			if err := database.DB.Model(&database.User{}).Where("bilibili_uid = ?", 10001).Updates(updates).Error; err != nil {
				close(resume)
				t.Fatal(err)
			}
			close(resume)
			got := <-result
			if got.response.Code != 401 || len(got.recorder.Result().Cookies()) != 0 {
				t.Fatalf("stale credentials gained a session: %+v", got.response)
			}
			var after database.User
			database.DB.First(&after)
			if after.BilibiliName != before.BilibiliName || after.BilibiliAvatar != before.BilibiliAvatar || store.deletes != 0 {
				t.Fatal("stale login overwrote account data or deleted images")
			}
			if change == "password" && bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("replacement-password")) != nil {
				t.Fatal("concurrent password change lost")
			}
			if (change == "epoch" || change == "failed-reader-epoch") && after.AuthVersion != "replacement-version" {
				t.Fatal("concurrent revocation lost")
			}
			if change == "disabled" && !after.IsDisabled {
				t.Fatal("concurrent disable lost")
			}
		})
	}
}

func TestConcurrentLoginProfileRefreshKeepsNewestCommittedProfile(t *testing.T) {
	s, before, store := loginProfileFixture(t)
	entered, resume := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	s.ctl.LoginProfileReader = signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
		if reads.Add(1) == 1 {
			close(entered)
			<-resume
			return bilibili.Profile{MID: uid, Name: "较早读取", Face: "https://i0.hdslb.com/bfs/face/older.jpg"}, nil
		}
		return bilibili.Profile{MID: uid, Name: "较新资料", Face: "https://i0.hdslb.com/bfs/face/newer.jpg"}, nil
	})
	store.save = func(_ context.Context, uid int64, remote string) (string, error) {
		return fmt.Sprintf("/upload-img/bilibili-avatar-%d-%s.jpg", uid, map[bool]string{true: "newer", false: "older"}[strings.Contains(remote, "newer")]), nil
	}
	result := make(chan loginProfileResult, 1)
	go func() {
		recorder, response := performJSONRequest(s.r, "POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil)
		result <- loginProfileResult{recorder, response}
	}()
	<-entered
	_, second := s.request("POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil, 200)
	close(resume)
	first := <-result
	if first.response.Code != 200 {
		t.Fatalf("concurrent same-credential login failed: %+v", first.response)
	}
	for _, res := range []testAPIResponse{first.response, second} {
		assertLoginProfileResponse(t, res, "10001", "较新资料", "/upload-img/bilibili-avatar-10001-newer.jpg")
	}
	var after database.User
	database.DB.First(&after)
	if after.BilibiliName != "较新资料" || after.AuthVersion != before.AuthVersion || store.deletes != 0 {
		t.Fatal("late response overwrote newer data or removed referenced image")
	}
}
