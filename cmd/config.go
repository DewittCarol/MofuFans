package main

import (
	"encoding/json"
	"errors"
	"github.com/spf13/viper"
	"mofufans/internal/siteconfig"
	"mofufans/internal/storage"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

func loadConfig() error {
	file := os.Getenv("MOFUFANS_CONFIG")
	explicit := file != ""
	if file == "" {
		file = "config/config.json"
	}
	viper.SetConfigFile(file)
	viper.SetEnvPrefix("MOFUFANS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	defaults := map[string]any{"db_type": "sqlite", "sqlite": "./data/mofufans.db", "state_directory": "./data", "storage_type": "local", "server.host": "127.0.0.1", "server.port": 8080, "server.trusted_proxies": []string{"127.0.0.1", "::1"}, "oss.endpoint": "https://oss-cn-hangzhou.aliyuncs.com", "oss.region": "cn-hangzhou", "oss.prefix": "mofufans"}
	for key, value := range defaults {
		viper.SetDefault(key, value)
	}
	if err := viper.ReadInConfig(); err != nil && (explicit || !os.IsNotExist(err)) {
		return errors.New("cannot read deployment configuration")
	}
	directory := viper.GetString("state_directory")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	instanceFile := filepath.Join(directory, "instance.json")
	instance := map[string]string{}
	if raw, err := os.ReadFile(instanceFile); err == nil {
		if json.Unmarshal(raw, &instance) != nil {
			return errors.New("cannot read instance keys; restore data/instance.json from backup")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	changed := false
	for _, key := range []string{"security.session_secret", "security.bilibili_cookie_key"} {
		value := viper.GetString(key)
		if value == "" {
			value = instance[key]
			if value == "" {
				if info, err := os.Stat(strings.Split(viper.GetString("sqlite"), "?")[0]); err == nil && info.Size() > 0 {
					return errors.New("existing database has no instance keys; restore the original configuration or data/instance.json")
				}
				var err error
				value, err = siteconfig.Random()
				if err != nil {
					return err
				}
				instance[key] = value
				changed = true
			}
			viper.Set(key, value)
		}
		if len(value) < 32 || strings.HasPrefix(value, "replace-") {
			return errors.New(key + " must contain at least 32 random characters")
		}
	}
	if changed {
		if err := siteconfig.WritePrivate(instanceFile, instance); err != nil {
			return err
		}
	}
	if viper.GetString("security.session_secret") == viper.GetString("security.bilibili_cookie_key") {
		return errors.New("session and encryption secrets must be independent")
	}
	if viper.GetString("db_type") != "sqlite" && viper.GetString("db_type") != "mysql" {
		return errors.New("db_type must be sqlite or mysql")
	}
	if viper.GetString("db_type") == "sqlite" {
		filename := strings.Split(viper.GetString("sqlite"), "?")[0]
		if filename == "" {
			return errors.New("sqlite path is required")
		}
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return err
		}
	}
	for _, origin := range viper.GetStringSlice("server.allowed_origins") {
		if strings.Contains(origin, "*") {
			return errors.New("allowed_origins must contain exact origins, without wildcards")
		}
	}
	for _, proxy := range viper.GetStringSlice("server.trusted_proxies") {
		if prefix, err := netip.ParsePrefix(proxy); err == nil {
			if prefix.Bits() == 0 {
				return errors.New("trusted_proxies cannot trust every address")
			}
		} else if _, err = netip.ParseAddr(proxy); err != nil {
			return errors.New("trusted_proxies must contain explicit IPs or CIDRs")
		}
	}
	s, err := siteconfig.Open(directory)
	if err != nil {
		return err
	}
	siteconfig.Use(s)
	return storage.Validate()
}
