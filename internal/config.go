package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AppConfig 是 ccx 的全局配置
type AppConfig struct {
	GiteeToken     string `json:"gitee_token"`
	GistID         string `json:"gist_id"`
	GistOwner      string `json:"gist_owner"`
	ClaudeCmd      string `json:"claude_command,omitempty"`
	DefaultProfile string `json:"default_profile,omitempty"`
}

// Profile 表示一个 Claude Code 配置
type Profile struct {
	Name     string
	Settings json.RawMessage // 原始 JSON，直接传给 claude --settings
}

// ProfileInfo 从 settings JSON 中提取的展示信息
type ProfileInfo struct {
	BaseURL         string
	Model           string
	APIFormat       string
	APIKey          string
	AuthToken       string
	ReasoningEffort string
}

// ConfigDir 返回配置目录路径 ~/.config/ccx
func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ccx")
}

// ConfigPath 返回主配置文件路径
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

// EnsureDirs 确保配置目录存在
func EnsureDirs() {
	os.MkdirAll(ConfigDir(), 0755)
}

// LoadAppConfig 加载主配置文件
func LoadAppConfig() (*AppConfig, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, fmt.Errorf("未找到配置文件，请先运行 ccx init: %w", err)
	}
	var cfg AppConfig
	json.Unmarshal(data, &cfg)
	if cfg.ClaudeCmd == "" {
		cfg.ClaudeCmd = "claude"
	}
	return &cfg, nil
}

// SaveAppConfig 保存主配置文件
func SaveAppConfig(cfg *AppConfig) {
	EnsureDirs()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(ConfigPath(), data, 0600)
}

// ConfigExists 检查配置文件是否存在
func ConfigExists() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

// ExtractProfileInfo 从 settings JSON 中提取展示信息
func ExtractProfileInfo(settings json.RawMessage) ProfileInfo {
	var parsed struct {
		APIFormat string            `json:"api_format"`
		Env       map[string]string `json:"env"`
	}
	json.Unmarshal(settings, &parsed)

	info := ProfileInfo{
		APIFormat: "anthropic",
	}
	if parsed.APIFormat != "" {
		info.APIFormat = parsed.APIFormat
	}
	if parsed.Env != nil {
		info.BaseURL = parsed.Env["ANTHROPIC_BASE_URL"]
		info.Model = parsed.Env["ANTHROPIC_MODEL"]
		info.APIKey = parsed.Env["ANTHROPIC_API_KEY"]
		info.AuthToken = parsed.Env["ANTHROPIC_AUTH_TOKEN"]
		info.ReasoningEffort = parsed.Env["OPENAI_REASONING_EFFORT"]
	}
	return info
}
