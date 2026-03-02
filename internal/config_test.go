package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractProfileInfoReasoningEffort(t *testing.T) {
	settings := json.RawMessage(`{
	  "api_format":"openai",
	  "env":{
	    "ANTHROPIC_BASE_URL":"https://api.linkflow.run/v1",
	    "ANTHROPIC_MODEL":"gpt-5.3-codex",
	    "ANTHROPIC_API_KEY":"k",
	    "ANTHROPIC_AUTH_TOKEN":"k",
	    "OPENAI_REASONING_EFFORT":"high"
	  }
	}`)

	info := ExtractProfileInfo(settings)
	if info.APIFormat != "openai" {
		t.Fatalf("APIFormat=%q, want openai", info.APIFormat)
	}
	if info.ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort=%q, want high", info.ReasoningEffort)
	}
}

func TestLoadAppConfigInvalidJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".config", "ccx")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{bad-json"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := LoadAppConfig()
	if err == nil {
		t.Fatal("expected error for invalid config json")
	}
	if !strings.Contains(err.Error(), "解析配置文件失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveAppConfigReturnsErrorWhenEnsureDirsFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configRoot := filepath.Join(home, ".config")
	if err := os.WriteFile(configRoot, []byte("not-a-dir"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := SaveAppConfig(&AppConfig{ClaudeCmd: "claude"})
	if err == nil {
		t.Fatal("expected SaveAppConfig to fail when config dir cannot be created")
	}
	if !strings.Contains(err.Error(), "创建配置目录失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}
