// Package siteconfig keeps browser-managed settings separate from deployment
// configuration. Viper remains read-only after startup; live reads use snapshots.
package siteconfig

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/viper"
)

type OSS struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
}
type Options struct {
	StorageType  string `json:"storage_type"`
	OSS          OSS    `json:"oss"`
	TestUID      string `json:"test_recipient_uid"`
	SecureCookie bool   `json:"secure_cookie"`
}
type Update struct {
	Options
	ClearCredentials     bool `json:"clear_credentials"`
	ConfirmStorageChange bool `json:"confirm_storage_change"`
}
type Store struct {
	mu            sync.RWMutex
	directory     string
	saved, active Options
}

var installed atomic.Pointer[Store]

func Use(s *Store)    { installed.Store(s) }
func Current() *Store { return installed.Load() }

func FromDeployment() Options {
	return Options{StorageType: viper.GetString("storage_type"), OSS: OSS{viper.GetString("oss.endpoint"), viper.GetString("oss.region"), viper.GetString("oss.bucket"), viper.GetString("oss.prefix"), viper.GetString("oss.access_key"), viper.GetString("oss.secret_key")}, TestUID: viper.GetString("bilibili.test_recipient_uid"), SecureCookie: viper.GetBool("security.secure_cookie")}
}
func Active() Options {
	if s := Current(); s != nil {
		return s.Active()
	}
	return FromDeployment()
}
func (s *Store) Active() Options { s.mu.RLock(); defer s.mu.RUnlock(); return s.active }
func (s *Store) Saved() Options  { s.mu.RLock(); defer s.mu.RUnlock(); return s.saved }
func environment(o Options) (Options, []string) {
	locked := []string{}
	for _, field := range []struct {
		key   string
		value *string
	}{
		{"storage_type", &o.StorageType}, {"oss.endpoint", &o.OSS.Endpoint}, {"oss.region", &o.OSS.Region}, {"oss.bucket", &o.OSS.Bucket}, {"oss.prefix", &o.OSS.Prefix}, {"oss.access_key", &o.OSS.AccessKey}, {"oss.secret_key", &o.OSS.SecretKey}, {"bilibili.test_recipient_uid", &o.TestUID},
	} {
		if value, ok := os.LookupEnv("MOFUFANS_" + strings.ToUpper(strings.ReplaceAll(field.key, ".", "_"))); ok && value != "" {
			*field.value = value
			locked = append(locked, field.key)
		}
	}
	if value := os.Getenv("MOFUFANS_SECURITY_SECURE_COOKIE"); value != "" {
		o.SecureCookie, _ = strconv.ParseBool(value)
		locked = append(locked, "security.secure_cookie")
	}
	return o, locked
}
func (s *Store) View() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, locked := environment(s.saved)
	return map[string]any{"storage_type": o.StorageType, "oss": map[string]any{"endpoint": o.OSS.Endpoint, "region": o.OSS.Region, "bucket": o.OSS.Bucket, "prefix": o.OSS.Prefix, "has_access_key": o.OSS.AccessKey != "", "has_secret_key": o.OSS.SecretKey != ""}, "test_recipient_uid": o.TestUID, "secure_cookie": o.SecureCookie, "locked": locked, "restart_required": StorageChanged(o, s.active), "active_storage_type": s.active.StorageType, "database_type": viper.GetString("db_type")}
}
func StorageChanged(a, b Options) bool {
	return a.StorageType != b.StorageType || (a.StorageType == "oss" && !reflect.DeepEqual(a.OSS, b.OSS))
}

var uid = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
var segment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func Validate(o Options) error {
	if o.TestUID != "" && !uid.MatchString(o.TestUID) {
		return errors.New("测试 UID 请填写不以 0 开头、最多 19 位的数字，或留空稍后配置")
	}
	if o.StorageType == "local" {
		return nil
	}
	if o.StorageType != "oss" {
		return errors.New("请选择本地存储或阿里云 OSS")
	}
	u, err := url.Parse(o.OSS.Endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || !strings.HasSuffix(u.Hostname(), ".aliyuncs.com") {
		return errors.New("OSS Endpoint 请填写对应地域的 HTTPS aliyuncs.com 地址，不带路径或端口")
	}
	if o.OSS.Region == "" || o.OSS.Bucket == "" || o.OSS.AccessKey == "" || o.OSS.SecretKey == "" {
		return errors.New("请填写 OSS 地域、Bucket、AccessKey ID 和 AccessKey Secret")
	}
	for _, p := range strings.Split(strings.Trim(o.OSS.Prefix, "/"), "/") {
		if p != "" && (!segment.MatchString(p) || p == "." || p == "..") {
			return errors.New("OSS 前缀格式不正确")
		}
	}
	return nil
}
func Open(directory string) (*Store, error) {
	s := &Store{directory: directory, saved: FromDeployment()}
	raw, err := os.ReadFile(filepath.Join(directory, "settings.json"))
	if err == nil {
		if json.Unmarshal(raw, &s.saved) != nil {
			return nil, errors.New("cannot read saved application settings")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	s.active, _ = environment(s.saved)
	if err = Validate(s.active); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) Propose(r Update) (Options, error) {
	o := r.Options
	o.TestUID = strings.TrimSpace(o.TestUID)
	o.OSS.Endpoint = strings.TrimRight(strings.TrimSpace(o.OSS.Endpoint), "/")
	o.OSS.Region = strings.TrimSpace(o.OSS.Region)
	o.OSS.Bucket = strings.TrimSpace(o.OSS.Bucket)
	o.OSS.Prefix = strings.Trim(o.OSS.Prefix, " /")
	old := s.Saved()
	if o.OSS.AccessKey == "" && !r.ClearCredentials {
		o.OSS.AccessKey = old.OSS.AccessKey
	}
	if o.OSS.SecretKey == "" && !r.ClearCredentials {
		o.OSS.SecretKey = old.OSS.SecretKey
	}
	// Environment values are authoritative, including credentials omitted by the UI.
	o, _ = environment(o)
	return o, Validate(o)
}
func (s *Store) Save(o Options, activateStorage bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := WritePrivate(filepath.Join(s.directory, "settings.json"), o); err != nil {
		return errors.New("设置保存失败，请检查 data 目录写入权限")
	}
	s.saved = o
	if activateStorage {
		s.active = o
	} else {
		s.active.TestUID = o.TestUID
		s.active.SecureCookie = o.SecureCookie
	}
	return nil
}
func Random() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func WritePrivate(file string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), file)
}
func (s *Store) SetupToken() (string, error) {
	file := filepath.Join(s.directory, "setup-token")
	if raw, err := os.ReadFile(file); err == nil {
		if len(strings.TrimSpace(string(raw))) != 64 {
			return "", errors.New("invalid setup-token file")
		}
		return strings.TrimSpace(string(raw)), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	token, err := Random()
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(token)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return token, err
}
func (s *Store) RemoveSetupToken() { _ = os.Remove(filepath.Join(s.directory, "setup-token")) }
