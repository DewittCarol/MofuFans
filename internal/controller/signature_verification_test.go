package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type signatureReadFunc func(context.Context, int64) (bilibili.Profile, error)

func (f signatureReadFunc) PublicSignature(c context.Context, u int64) (bilibili.Profile, error) {
	return f(c, u)
}

type signatureRig struct {
	t            *testing.T
	r            *gin.Engine
	ctl          *AccountController
	now          time.Time
	text         string
	wrongUID     bool
	failRead     bool
	readCalls    int
	challengeIDs map[string]string
	cookieUIDs   map[string]string
}

func newSignatureRig(t *testing.T) *signatureRig {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "signature.db")+"?_busy_timeout=10000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := database.InitializeMemberIdentity(); err != nil {
		t.Fatal(err)
	}
	s := &signatureRig{t: t, now: time.Now().UTC(), challengeIDs: map[string]string{}, cookieUIDs: map[string]string{}}
	s.ctl = &AccountController{Now: func() time.Time { return s.now }, AvatarStorage: fakeAvatarStorage{}, LoginProfileReader: unavailableLoginProfileReader{}, Limits: NewMemberRateLimits(), SignatureReader: signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
		s.readCalls++
		if s.failRead {
			return bilibili.Profile{}, errors.New("network unavailable")
		}
		if s.wrongUID {
			uid++
		}
		return bilibili.Profile{MID: uid, Name: "验证测试账号", Signature: s.text}, nil
	})}
	r := gin.New()
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte("signature-tests-secret-at-least-32-bytes"))))
	r.POST("/start", s.ctl.StartSignature)
	r.GET("/status", s.ctl.SignatureStatus)
	r.POST("/check", s.ctl.CheckSignature)
	r.POST("/finish", s.ctl.FinishSignature)
	r.POST("/login", s.ctl.Login)
	r.GET("/info", s.ctl.Info)
	r.POST("/logout", s.ctl.Logout)
	r.POST("/password", s.ctl.ChangePassword)
	r.POST("/logout-others", s.ctl.LogoutOthers)
	r.GET("/strong", func(c *gin.Context) {
		if u, ok := MemberStrongUser(c); ok {
			Success(c, publicUser(u))
		} else {
			Fail(c, 403, "需要签名验证")
		}
	})
	r.GET("/fixture-old-cookie", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set(memberSessionKey, int64(10001))
		session.Delete(memberAuthSessionKey)
		session.Set("authed", true)
		session.Set("user", uint(17))
		session.Set("auth_stamp", "admin-stamp")
		session.Save()
		Success(c, nil)
	})
	r.GET("/fixture-admin-check", func(c *gin.Context) {
		s := sessions.Default(c)
		Success(c, gin.H{"authed": s.Get("authed"), "user": s.Get("user"), "stamp": s.Get("auth_stamp"), "challenge": s.Get(signatureRequestSessionKey), "secret": s.Get(signatureSecretSessionKey), "text": s.Get(signatureTextSessionKey)})
	})
	r.POST("/fixture-text", func(c *gin.Context) {
		var body struct {
			Text   *string `json:"text"`
			Secret *string `json:"secret"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			t.Fatal(err)
		}
		session := sessions.Default(c)
		if body.Text == nil {
			session.Delete(signatureTextSessionKey)
		} else {
			session.Set(signatureTextSessionKey, *body.Text)
		}
		if body.Secret != nil {
			session.Set(signatureSecretSessionKey, *body.Secret)
		}
		if err := session.Save(); err != nil {
			t.Fatal(err)
		}
		Success(c, nil)
	})
	members := &MemberController{}
	r.PUT("/member/:uid", members.Put)
	s.r = r
	return s
}

func signatureCookieKey(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if c.Name == "session" {
			return c.Value
		}
	}
	return ""
}

func (s *signatureRig) requestBody(path, body string, cookies []*http.Cookie) string {
	if path != "/check" && path != "/finish" && path != "/password" && path != "/logout-others" && path != "/logout" {
		return body
	}
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil || payload == nil {
		return body
	}
	key := signatureCookieKey(cookies)
	if path == "/check" || path == "/finish" {
		if _, provided := payload["challenge_id"]; !provided {
			payload["challenge_id"] = s.challengeIDs[key]
		}
	} else {
		if _, provided := payload["uid"]; !provided {
			payload["uid"] = s.cookieUIDs[key]
		}
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func (s *signatureRig) request(method, path, body string, cookies []*http.Cookie, want int) ([]*http.Cookie, testAPIResponse) {
	s.t.Helper()
	rec, res := performJSONRequest(s.r, method, path, s.requestBody(path, body, cookies), cookies)
	if res.Code != want {
		s.t.Fatalf("%s %s: want %d got %d (%s)", method, path, want, res.Code, res.Message)
	}
	updated := rec.Result().Cookies()
	if len(updated) == 0 {
		updated = cookies
	}
	if res.Code == 200 {
		key, oldKey := signatureCookieKey(updated), signatureCookieKey(cookies)
		var data struct {
			ChallengeID string `json:"challenge_id"`
			UID         string `json:"bilibili_uid"`
		}
		json.Unmarshal(res.Data, &data)
		if data.ChallengeID != "" {
			s.challengeIDs[key] = data.ChallengeID
		} else {
			s.challengeIDs[key] = s.challengeIDs[oldKey]
		}
		if data.UID != "" {
			s.cookieUIDs[key] = data.UID
		} else {
			s.cookieUIDs[key] = s.cookieUIDs[oldKey]
		}
	}
	return updated, res
}

func (s *signatureRig) start(uid, purpose string, cookies []*http.Cookie) []*http.Cookie {
	s.t.Helper()
	next, res := s.request("POST", "/start", fmt.Sprintf(`{"bilibili_uid":%q,"purpose":%q}`, uid, purpose), cookies, 200)
	var view struct {
		Text string `json:"verification_text"`
	}
	if json.Unmarshal(res.Data, &view) != nil || view.Text == "" {
		s.t.Fatal("missing verification text")
	}
	s.text = "original signature " + view.Text
	return next
}

func (s *signatureRig) register(uid string) []*http.Cookie {
	c := s.start(uid, "register", nil)
	s.request("POST", "/check", `{}`, c, 200)
	c, _ = s.request("POST", "/finish", `{"password":"initial-password"}`, c, 200)
	return c
}

func (s *signatureRig) seedLegacy(uid int64, name string) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.MinCost)
	if err := database.DB.Create(&database.User{BilibiliUID: uid, Username: name, PasswordHash: string(hash), VerifiedAt: s.now, AuthVersion: ""}).Error; err != nil {
		s.t.Fatal(err)
	}
}

func TestSignatureRegistrationPrivateCredentialAndPasswordLogin(t *testing.T) {
	s := newSignatureRig(t)
	cookies := s.start("10001", "register", nil)
	var challenge database.SignatureChallenge
	database.DB.First(&challenge)
	if challenge.SecretHash == challenge.CodeHash || len(challenge.SecretHash) != 64 || len(challenge.CodeHash) != 64 || strings.Contains(challenge.CodeHash, "MFV-") {
		t.Fatal("challenge hashes not independent")
	}
	s.request("POST", "/check", `{}`, cookies, 200)
	_, status := s.request("GET", "/status", "", cookies, 200)
	if !strings.Contains(string(status.Data), strings.TrimPrefix(s.text, "original signature ")) {
		t.Fatal("status did not restore this browser's verified text")
	}
	cookies, _ = s.request("POST", "/finish", `{"password":"initial-password"}`, cookies, 200)
	var u database.User
	database.DB.First(&u)
	if u.SignatureVerifiedAt == nil || u.AuthVersion == "" || u.Username != "10001" || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("initial-password")) != nil {
		t.Fatal("incorrect registered identity")
	}
	s.request("GET", "/strong", "", cookies, 200)
	s.text = "restored signature"
	login, _ := s.request("POST", "/login", `{"bilibili_uid":"10001","password":"initial-password"}`, nil, 200)
	s.request("GET", "/strong", "", login, 200)
	s.request("GET", "/strong", "", nil, 403)
}

func TestSignatureRejectsUnboundWrongAndExpiredProofs(t *testing.T) {
	s := newSignatureRig(t)
	cookies := s.start("10001", "register", nil)
	s.request("POST", "/check", `{}`, nil, 403)
	correct := s.text
	s.text = "old unrelated signature"
	s.request("POST", "/check", `{}`, cookies, 422)
	s.now = s.now.Add(6 * time.Second)
	s.text = correct
	s.wrongUID = true
	s.request("POST", "/check", `{}`, cookies, 422)
	s.now = s.now.Add(6 * time.Second)
	s.wrongUID = false
	s.failRead = true
	s.request("POST", "/check", `{}`, cookies, 502)
	s.failRead = false
	s.now = s.now.Add(11 * time.Minute)
	s.request("POST", "/check", `{}`, cookies, 410)
	s.request("POST", "/finish", `{"password":"initial-password"}`, cookies, 410)
	var count int64
	database.DB.Model(&database.User{}).Count(&count)
	if count != 0 {
		t.Fatal("failed proofs created a user")
	}
}

func TestSignatureMarkerCannotTransferAcrossBrowsersOrUIDs(t *testing.T) {
	s := newSignatureRig(t)
	a := s.start("10001", "register", nil)
	textA := s.text
	b := s.start("10001", "register", nil)
	s.text = textA
	s.request("POST", "/check", `{}`, b, 422)
	s.now = s.now.Add(6 * time.Second)
	c := s.start("10002", "register", nil)
	s.text = textA
	s.request("POST", "/check", `{}`, c, 422)
	s.request("POST", "/check", `{}`, a, 200)
}

func TestSignatureRegistrationIsConsumedExactlyOnce(t *testing.T) {
	s := newSignatureRig(t)
	c := s.start("10001", "register", nil)
	s.request("POST", "/check", `{}`, c, 200)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	finishBody := s.requestBody("/finish", `{"password":"initial-password"}`, c)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, res := performJSONRequest(s.r, "POST", "/finish", finishBody, c)
			codes <- res.Code
		}()
	}
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 200 {
			success++
		} else if code != 409 && code != 410 {
			t.Fatalf("unexpected duplicate response %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("concurrent success count %d", success)
	}
	var count int64
	database.DB.Model(&database.User{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate identity created")
	}
	s.request("POST", "/finish", `{"password":"initial-password"}`, c, 410)
}

func TestLegacyAccountUpgradeRevokesWeakSessionsAndPreservesAdmin(t *testing.T) {
	s := newSignatureRig(t)
	s.seedLegacy(10001, "legacy-name")
	old, _ := s.request("GET", "/fixture-old-cookie", "", nil, 200)
	s.request("GET", "/info", "", old, 408)
	login, res := s.request("POST", "/login", `{"username":"legacy-name","password":"legacy-password"}`, old, 200)
	if strings.Contains(string(res.Data), `"signature_verified":true`) {
		t.Fatal("legacy account automatically upgraded")
	}
	s.request("GET", "/strong", "", login, 403)
	s.seedLegacy(10002, "another")
	s.request("POST", "/start", `{"bilibili_uid":"10002","purpose":"upgrade"}`, login, 403)
	c := s.start("10001", "upgrade", login)
	s.request("POST", "/check", `{}`, c, 200)
	strong, _ := s.request("POST", "/finish", `{}`, c, 200)
	s.request("GET", "/strong", "", strong, 200)
	s.request("GET", "/info", "", login, 408)
	_, admin := s.request("GET", "/fixture-admin-check", "", strong, 200)
	if !strings.Contains(string(admin.Data), `"authed":true`) || !strings.Contains(string(admin.Data), "admin-stamp") {
		t.Fatal("member flow cleared administrator session")
	}
}

func TestRecoveryPasswordChangeAndLogoutOthersRevokeOldSessions(t *testing.T) {
	s := newSignatureRig(t)
	original := s.register("10001")
	recovery := s.start("10001", "recover", nil)
	s.request("POST", "/check", `{}`, recovery, 200)
	recovered, _ := s.request("POST", "/finish", `{"password":"recovered-password"}`, recovery, 200)
	s.request("GET", "/info", "", original, 408)
	s.request("POST", "/login", `{"bilibili_uid":"10001","password":"initial-password"}`, nil, 401)
	other, _ := s.request("POST", "/login", `{"bilibili_uid":"10001","password":"recovered-password"}`, nil, 200)
	s.request("POST", "/password", `{"current_password":"bad-password","password":"new-password"}`, recovered, 401)
	changed, _ := s.request("POST", "/password", `{"current_password":"recovered-password","password":"new-password"}`, recovered, 200)
	s.request("GET", "/info", "", recovered, 408)
	s.request("GET", "/info", "", other, 408)
	latest, _ := s.request("POST", "/logout-others", `{"password":"new-password"}`, changed, 200)
	s.request("GET", "/info", "", changed, 408)
	s.request("GET", "/strong", "", latest, 200)
}

func TestRecoveryCannotUndoCredentialChangesOrDisabledAccount(t *testing.T) {
	s := newSignatureRig(t)
	original := s.register("10001")
	recovery := s.start("10001", "recover", nil)
	changed, _ := s.request("POST", "/password", `{"current_password":"initial-password","password":"new-password"}`, original, 200)
	s.request("POST", "/check", `{}`, recovery, 409)
	recovery = s.start("10001", "recover", nil)
	s.request("POST", "/check", `{}`, recovery, 200)
	s.request("PUT", "/member/10001", `{"is_disabled":true}`, nil, 200)
	s.request("GET", "/info", "", changed, 408)
	s.request("POST", "/finish", `{"password":"hijack-password"}`, recovery, 409)
	s.request("POST", "/start", `{"bilibili_uid":"10001","purpose":"recover"}`, nil, 403)
	s.request("PUT", "/member/10001", `{"is_disabled":false}`, nil, 200)
	s.request("GET", "/info", "", changed, 408)
}

func TestNumericLegacyLoginDoesNotBlockUIDRegistration(t *testing.T) {
	s := newSignatureRig(t)
	s.seedLegacy(999, "10001")
	strong := s.register("10001")
	s.request("GET", "/strong", "", strong, 200)
	_, legacy := s.request("POST", "/login", `{"username":"10001","password":"legacy-password"}`, nil, 200)
	if !strings.Contains(string(legacy.Data), `"bilibili_uid":"999"`) {
		t.Fatal("numeric legacy login confused with UID")
	}
	_, uid := s.request("POST", "/login", `{"bilibili_uid":"10001","password":"initial-password"}`, nil, 200)
	if !strings.Contains(string(uid.Data), `"bilibili_uid":"10001"`) {
		t.Fatal("UID login confused with legacy name")
	}
}

func TestSignatureRequestsRejectCrossOriginFormsAndLargeBodies(t *testing.T) {
	s := newSignatureRig(t)
	for _, tc := range []struct {
		origin, site, content, body string
		want                        int
	}{
		{"https://evil.example", "", "application/json", `{}`, 403},
		{"", "same-site", "application/json", `{}`, 403},
		{"", "", "application/x-www-form-urlencoded", `{}`, 415},
		{"", "", "application/json", `{"password":"` + strings.Repeat("a", 5000) + `"}`, 400},
	} {
		req := httptest.NewRequest("POST", "/start", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.content)
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		rec := httptest.NewRecorder()
		s.r.ServeHTTP(rec, req)
		var response testAPIResponse
		json.Unmarshal(rec.Body.Bytes(), &response)
		if response.Code != tc.want {
			t.Fatalf("want %d got %+v", tc.want, response)
		}
	}
}

func TestSignatureCheckingIsRateLimitedAndHasNoStatusPollingReads(t *testing.T) {
	s := newSignatureRig(t)
	c := s.start("10001", "register", nil)
	s.text = "not yet saved"
	s.request("POST", "/check", `{}`, c, 422)
	s.request("POST", "/check", `{}`, c, 429)
	for n := 0; n < 10; n++ {
		s.request("GET", "/status", "", c, 200)
	}
	var request database.SignatureChallenge
	database.DB.First(&request)
	if request.CheckCount != 1 {
		t.Fatal("status queried upstream or cooldown failed")
	}
	for n := 0; n < 4; n++ {
		s.start("10001", "register", nil)
	}
	s.request("POST", "/start", `{"bilibili_uid":"10001","purpose":"register"}`, nil, 429)
}

func TestIdentityMigrationDoesNotUpgradeLegacyUsersAndIsRepeatable(t *testing.T) {
	s := newSignatureRig(t)
	s.seedLegacy(10001, "legacy")
	var before database.User
	database.DB.First(&before)
	for n := 0; n < 2; n++ {
		if err := database.InitializeMemberIdentity(); err != nil {
			t.Fatal(err)
		}
	}
	var after database.User
	database.DB.First(&after)
	if after.SignatureVerifiedAt != nil || after.PasswordHash != before.PasswordHash || after.Username != before.Username || after.AuthVersion != "" {
		t.Fatal("migration altered or upgraded legacy identity")
	}
}

func TestPasswordThrottleSharesCanonicalUIDAndLegacyName(t *testing.T) {
	s := newSignatureRig(t)
	s.seedLegacy(10001, "legacy-name")
	for n := 0; n < 12; n++ {
		body := fmt.Sprintf(`{"bilibili_uid":%q,"password":"wrong-password"}`, strings.Repeat("0", n)+"10001")
		if n%2 == 1 {
			body = `{"username":"legacy-name","password":"wrong-password"}`
		}
		s.request("POST", "/login", body, nil, 401)
	}
	s.request("POST", "/login", `{"bilibili_uid":"10001","password":"legacy-password"}`, nil, 429)
}

func TestSignatureExpiryIsCheckedAfterUpstreamAndBeforeFinish(t *testing.T) {
	s := newSignatureRig(t)
	c := s.start("10001", "register", nil)
	s.ctl.SignatureReader = signatureReadFunc(func(_ context.Context, uid int64) (bilibili.Profile, error) {
		s.now = s.now.Add(11 * time.Minute)
		return bilibili.Profile{MID: uid, Name: "超时测试", Signature: s.text}, nil
	})
	s.request("POST", "/check", `{}`, c, 410)
	var challenge database.SignatureChallenge
	database.DB.First(&challenge)
	if challenge.Status == database.VerificationVerified {
		t.Fatal("expired upstream result became a proof")
	}

	s = newSignatureRig(t)
	c = s.start("10001", "register", nil)
	s.request("POST", "/check", `{}`, c, 200)
	s.now = s.now.Add(6 * time.Minute)
	s.request("POST", "/finish", `{"password":"initial-password"}`, c, 410)
}

func TestSeparateRecoveryRequestsCannotOverwriteEachOther(t *testing.T) {
	s := newSignatureRig(t)
	s.register("10001")
	a := s.start("10001", "recover", nil)
	textA := s.text
	b := s.start("10001", "recover", nil)
	s.text = textA + " " + s.text
	s.request("POST", "/check", `{}`, a, 200)
	s.request("POST", "/check", `{}`, b, 200)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i, c := range [][]*http.Cookie{a, b} {
		body := s.requestBody("/finish", fmt.Sprintf(`{"password":"new-password-%d"}`, i), c)
		wg.Add(1)
		go func(body string, c []*http.Cookie) {
			defer wg.Done()
			_, res := performJSONRequest(s.r, "POST", "/finish", body, c)
			codes <- res.Code
		}(body, c)
	}
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 200 {
			success++
		} else if code != 409 {
			t.Fatalf("unexpected concurrent recovery response %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("competing recovery requests succeeded %d times", success)
	}
}

func TestSignatureCrossTabChallengeMismatchCannotCheckOrFinish(t *testing.T) {
	s := newSignatureRig(t)
	a := s.start("10001", "register", nil)
	s.request("POST", "/check", `{}`, a, 200)
	aID := s.challengeIDs[signatureCookieKey(a)]
	b := s.start("10002", "register", a)
	bID := s.challengeIDs[signatureCookieKey(b)]
	s.request("POST", "/check", fmt.Sprintf(`{"challenge_id":%q}`, aID), b, 409)
	s.request("POST", "/check", `{}`, b, 200)
	s.request("POST", "/finish", fmt.Sprintf(`{"challenge_id":%q,"password":"wrong-tab-password"}`, aID), b, 409)
	var request database.SignatureChallenge
	database.DB.Where("id = ?", bID).First(&request)
	if request.Status != database.VerificationVerified {
		t.Fatal("wrong tab consumed current challenge")
	}
	s.request("POST", "/finish", `{"password":"correct-tab-password"}`, b, 200)
	var user database.User
	database.DB.First(&user)
	if user.BilibiliUID != 10002 || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("correct-tab-password")) != nil {
		t.Fatal("incorrect cross-tab account mutation")
	}
}

func TestPasswordAndLogoutActionsRequireExpectedUID(t *testing.T) {
	s := newSignatureRig(t)
	b := s.register("10002")
	s.request("POST", "/password", `{"uid":"10001","current_password":"initial-password","password":"wrong-target-password"}`, b, 403)
	s.request("POST", "/logout-others", `{"uid":"10001","password":"initial-password"}`, b, 403)
	s.request("POST", "/logout", `{"uid":"10001"}`, b, 403)
	s.request("GET", "/strong", "", b, 200)
	s.request("POST", "/login", `{"bilibili_uid":"10002","password":"initial-password"}`, nil, 200)
}

func TestSignatureShortTextUsesCurrentBrandAndBoundedSingleLine(t *testing.T) {
	for _, tc := range []struct {
		name, supplied, want string
		useDefault           bool
	}{
		{"mofu", "", "MofuFun", true},
		{"sample-a", "SampleA", "SampleA", false},
		{"sample-b", "SampleB", "SampleB", false},
		{"single-line", "  SampleA\r\n粉丝站\t", "SampleA 粉丝站", false},
		{"bounded", strings.Repeat("长", 40), strings.Repeat("长", signatureBrandMaxRunes), false},
		{"empty", "\x00\n<>\u202e", "本站", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSignatureRig(t)
			brand := tc.supplied
			if !tc.useDefault {
				s.ctl.SignatureBrand = func() string { return brand }
			}
			c := s.start("10001", "register", nil)
			text := strings.TrimPrefix(s.text, "original signature ")
			if !regexp.MustCompile("^"+regexp.QuoteMeta(tc.want)+`验证[a-z2-7]{8}$`).MatchString(text) || utf8.RuneCountInString(text) > signatureBrandMaxRunes+10 || strings.ContainsAny(text, "\r\n\t") {
				t.Fatalf("unexpected short verification text %q", text)
			}
			var record database.SignatureChallenge
			database.DB.First(&record)
			if record.CodeHash != signatureTextHash(text) || record.ExpiresAt.Sub(record.RequestedAt) != 10*time.Minute {
				t.Fatal("short proof hash or lifetime changed")
			}
			s.request("POST", "/check", `{}`, c, 200)
			if !tc.useDefault {
				brand = "UpdatedBrand"
				s.start("10002", "register", nil)
				if !strings.Contains(s.text, "UpdatedBrand验证") {
					t.Fatal("brand callback was cached")
				}
			}
		})
	}
}

func TestSignatureShortTextDoesNotMatchLongerOrUnbrandedCodes(t *testing.T) {
	s := newSignatureRig(t)
	c := s.start("10001", "register", nil)
	text := strings.TrimPrefix(s.text, "original signature ")
	for _, invalid := range []string{text + "a", text + "Z", text + "9", text + "_", text + "-", text[len(text)-8:], "Other验证" + text[len(text)-8:]} {
		s.text = invalid
		s.request("POST", "/check", `{}`, c, 422)
		s.now = s.now.Add(6 * time.Second)
	}
	s.text = "原来的签名" + text + "保留的文字"
	s.request("POST", "/check", `{}`, c, 200)
}

func TestSignatureStatusRestoresPendingAndVerifiedWithoutUpstreamOrRenewal(t *testing.T) {
	s := newSignatureRig(t)
	c := s.start("10001", "register", nil)
	text := strings.TrimPrefix(s.text, "original signature ")
	var original database.SignatureChallenge
	database.DB.First(&original)
	s.now = s.now.Add(time.Minute)
	_, pending := s.request("GET", "/status", "", c, 200)
	if !strings.Contains(string(pending.Data), `"status":"pending"`) || !strings.Contains(string(pending.Data), text) || s.readCalls != 0 {
		t.Fatal("pending status not restored independently")
	}
	s.request("POST", "/check", `{}`, c, 200)
	var verified database.SignatureChallenge
	database.DB.First(&verified)
	s.now = s.now.Add(time.Minute)
	s.failRead = true
	for n := 0; n < 3; n++ {
		_, response := s.request("GET", "/status", "", c, 200)
		if !strings.Contains(string(response.Data), `"status":"verified"`) || !strings.Contains(string(response.Data), text) || !strings.Contains(string(response.Data), `"confirmation_until"`) {
			t.Fatal("verified status not restored")
		}
	}
	var after database.SignatureChallenge
	database.DB.First(&after)
	if s.readCalls != 1 || after.CheckCount != 1 || !after.ExpiresAt.Equal(original.ExpiresAt) || after.ConfirmationUntil == nil || !after.ConfirmationUntil.Equal(*verified.ConfirmationUntil) {
		t.Fatal("status renewed or rechecked proof")
	}
	s.request("POST", "/finish", `{"password":"initial-password"}`, c, 200)
}

func TestSignatureLegacyLongCodeAndVerifiedProgressRemainUsable(t *testing.T) {
	s := newSignatureRig(t)
	c := s.start("10001", "register", nil)
	oldCode := "MFV-" + strings.Repeat("A", 26)
	if err := database.DB.Model(&database.SignatureChallenge{}).Where("id = ?", s.challengeIDs[signatureCookieKey(c)]).Update("code_hash", hashVerificationToken(oldCode)).Error; err != nil {
		t.Fatal(err)
	}
	c, _ = s.request("POST", "/fixture-text", `{}`, c, 200)
	_, pending := s.request("GET", "/status", "", c, 200)
	if strings.Contains(string(pending.Data), "verification_text") || s.readCalls != 0 {
		t.Fatal("legacy pending fabricated text or checked upstream")
	}
	s.text = "原签名 example.com 账号验证 " + oldCode
	s.request("POST", "/check", `{}`, c, 200)
	s.failRead = true
	_, verified := s.request("GET", "/status", "", c, 200)
	if !strings.Contains(string(verified.Data), `"status":"verified"`) || strings.Contains(string(verified.Data), "verification_text") || s.readCalls != 1 {
		t.Fatal("legacy verified progress did not resume")
	}
	s.request("POST", "/finish", `{"password":"initial-password"}`, c, 200)
}

func TestSignatureStatusDoesNotEchoInvalidTextOrWrongSecret(t *testing.T) {
	s := newSignatureRig(t)
	c := s.start("10001", "register", nil)
	text := strings.TrimPrefix(s.text, "original signature ")
	for _, wrong := range []string{"Other验证" + text[len(text)-8:], text + "extra", "\n" + text, strings.Repeat("x", 200)} {
		changed, _ := s.request("POST", "/fixture-text", fmt.Sprintf(`{"text":%q}`, wrong), c, 200)
		_, result := s.request("GET", "/status", "", changed, 200)
		if strings.Contains(string(result.Data), "verification_text") {
			t.Fatal("unbound cookie text was echoed")
		}
	}
	wrongSecret, _ := s.request("POST", "/fixture-text", fmt.Sprintf(`{"text":%q,"secret":%q}`, text, strings.Repeat("0", 64)), c, 200)
	s.request("GET", "/status", "", wrongSecret, 404)
	s.request("GET", "/status", "", nil, 404)
	if s.readCalls != 0 {
		t.Fatal("restoration queried Bilibili")
	}
}

func TestSignatureStatusRejectsChangedAccountAndExpiredOrConsumedText(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		s := newSignatureRig(t)
		c := s.start("10001", "register", nil)
		s.now = s.now.Add(11 * time.Minute)
		_, res := s.request("GET", "/status", "", c, 200)
		if !strings.Contains(string(res.Data), `"status":"expired"`) || strings.Contains(string(res.Data), "verification_text") {
			t.Fatal("expired proof restored as usable")
		}
	})
	t.Run("confirmation-expired", func(t *testing.T) {
		s := newSignatureRig(t)
		c := s.start("10001", "register", nil)
		s.request("POST", "/check", `{}`, c, 200)
		s.now = s.now.Add(6 * time.Minute)
		_, res := s.request("GET", "/status", "", c, 200)
		if !strings.Contains(string(res.Data), `"status":"expired"`) || strings.Contains(string(res.Data), "verification_text") {
			t.Fatal("expired confirmation restored as usable")
		}
	})
	t.Run("consumed", func(t *testing.T) {
		s := newSignatureRig(t)
		c := s.start("10001", "register", nil)
		s.request("POST", "/check", `{}`, c, 200)
		s.request("POST", "/finish", `{"password":"initial-password"}`, c, 200)
		_, res := s.request("GET", "/status", "", c, 200)
		if !strings.Contains(string(res.Data), `"status":"consumed"`) || strings.Contains(string(res.Data), "verification_text") {
			t.Fatal("consumed proof text restored")
		}
	})
	t.Run("register-created-elsewhere", func(t *testing.T) {
		s := newSignatureRig(t)
		c := s.start("10001", "register", nil)
		s.seedLegacy(10001, "created-elsewhere")
		s.request("GET", "/status", "", c, 409)
		if s.readCalls != 0 {
			t.Fatal("invalid registration restoration queried upstream")
		}
	})
	t.Run("upgrade-switched-member", func(t *testing.T) {
		s := newSignatureRig(t)
		s.seedLegacy(10001, "legacy")
		login, _ := s.request("POST", "/login", `{"username":"legacy","password":"legacy-password"}`, nil, 200)
		c := s.start("10001", "upgrade", login)
		s.request("POST", "/check", `{}`, c, 200)
		// Preserve the account and proof stamp, but replace only the member session.
		c, _ = s.request("GET", "/fixture-old-cookie", "", c, 200)
		s.request("GET", "/status", "", c, 409)
		if s.readCalls != 1 {
			t.Fatal("invalid upgrade restoration queried upstream")
		}
	})
	for _, stage := range []string{"pending", "verified"} {
		for _, change := range []string{"disabled", "credentials"} {
			t.Run(stage+"-"+change, func(t *testing.T) {
				s := newSignatureRig(t)
				s.register("10001")
				c := s.start("10001", "recover", nil)
				if stage == "verified" {
					s.request("POST", "/check", `{}`, c, 200)
				}
				if change == "disabled" {
					database.DB.Model(&database.User{}).Where("bilibili_uid = ?", 10001).Update("is_disabled", true)
				} else {
					database.DB.Model(&database.User{}).Where("bilibili_uid = ?", 10001).Update("auth_version", "changed")
				}
				before := s.readCalls
				_, res := s.request("GET", "/status", "", c, 409)
				if strings.Contains(string(res.Data), "verification_text") || s.readCalls != before {
					t.Fatal("changed account resumed proof")
				}
			})
		}
	}
}

func TestSignatureSessionTextClearedOnLoginFinishAndLogoutPreservingAdmin(t *testing.T) {
	for _, action := range []string{"login", "finish", "logout"} {
		t.Run(action, func(t *testing.T) {
			s := newSignatureRig(t)
			if action == "login" {
				s.seedLegacy(10002, "legacy")
			}
			admin, _ := s.request("GET", "/fixture-old-cookie", "", nil, 200)
			c := s.start("10001", "register", admin)
			switch action {
			case "login":
				c, _ = s.request("POST", "/login", `{"username":"legacy","password":"legacy-password"}`, c, 200)
			case "finish":
				s.request("POST", "/check", `{}`, c, 200)
				c, _ = s.request("POST", "/finish", `{"password":"initial-password"}`, c, 200)
			case "logout":
				c, _ = s.request("POST", "/logout", `{}`, c, 200)
			}
			s.request("GET", "/status", "", c, 404)
			_, res := s.request("GET", "/fixture-admin-check", "", c, 200)
			if !strings.Contains(string(res.Data), `"authed":true`) || !strings.Contains(string(res.Data), `"stamp":"admin-stamp"`) {
				t.Fatal("administrator session was cleared")
			}
			var state map[string]any
			if err := json.Unmarshal(res.Data, &state); err != nil {
				t.Fatal(err)
			}
			if state["challenge"] != nil || state["secret"] != nil || state["text"] != nil {
				t.Fatal("signature session fields survived successful account action")
			}
		})
	}
}
