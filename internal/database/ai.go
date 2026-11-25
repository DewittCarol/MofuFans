package database

import (
	"encoding/json"
	"errors"
	"mofufans/internal/ai"
	"mofufans/internal/secret"
	"strings"
)

func (c Config) AIOptions() (ai.Options, error) {
	options := ai.Default()
	if c.AIConfig != "" && json.Unmarshal([]byte(c.AIConfig), &options) != nil {
		return options, errors.New("AI 设置无法读取，请重新配置")
	}
	return options, nil
}
func (c Config) AIKey() (string, error) {
	if c.AIKeyCiphertext == "" {
		return "", nil
	}
	key, err := secret.DecryptCookie(c.AIKeyCiphertext)
	if err != nil {
		return "", errors.New("AI 密钥无法解密，请重新填写")
	}
	return key, nil
}

// Preserve configured installations while retiring the plaintext legacy key.
func InitializeAI() error {
	var config Config
	if err := DB.First(&config).Error; err != nil {
		return err
	}
	if config.AIConfig != "" || strings.TrimSpace(config.DeepSeekAPIKey) == "" {
		return nil
	}
	options := ai.Default()
	options.Enabled = true
	options.Protocol = "openai_chat"
	options.BaseURL = "https://api.deepseek.com/v1"
	options.Model = "deepseek-v4-flash"
	options.ChatTokenField = "max_tokens"
	ciphertext, err := secret.EncryptCookie(config.DeepSeekAPIKey)
	if err != nil {
		return errors.New("cannot migrate AI credentials: encryption key unavailable")
	}
	raw, _ := json.Marshal(options)
	return DB.Model(&config).Updates(map[string]any{"ai_config": string(raw), "ai_key_ciphertext": ciphertext, "deep_seek_api_key": ""}).Error
}
