package cmd

import (
	"ccx/internal"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化 ccx 配置（设置 Gitee Gist 信息）",
	RunE: func(cmd *cobra.Command, args []string) error {
		return initRun()
	},
}

func initRun() error {
	fmt.Println("=== CCX 初始化 ===")

	// 加载已有配置（如果存在）
	existing, _ := internal.LoadAppConfig()

	defaultToken := ""
	defaultOwner := ""
	defaultGistID := ""
	defaultCmd := "claude"
	if existing != nil {
		defaultToken = existing.GiteeToken
		defaultOwner = existing.GistOwner
		defaultGistID = existing.GistID
		defaultCmd = existing.ClaudeCmd
	}

	token := internal.PromptPassword("Gitee Personal Access Token")
	if token == "" {
		token = defaultToken
	}

	owner := internal.PromptInput("Gitee 用户名", defaultOwner)
	gistID := internal.PromptInput("Gist ID（codes 路径中的 ID）", defaultGistID)
	claudeCmd := internal.PromptInput("Claude 命令名", defaultCmd)

	cfg := &internal.AppConfig{
		GiteeToken: token,
		GistID:     gistID,
		GistOwner:  owner,
		ClaudeCmd:  claudeCmd,
	}

	// 测试连接
	fmt.Println("\n正在测试 Gitee 连接...")
	client := internal.NewGistClientFromConfig(cfg)
	files, err := client.ListSettingsFiles()
	if err != nil {
		return fmt.Errorf("连接 Gitee 失败: %w", err)
	}
	fmt.Printf("连接成功，发现 %d 个配置文件\n", len(files))

	// 保存配置
	internal.SaveAppConfig(cfg)
	fmt.Printf("配置已保存到 %s\n", internal.ConfigPath())

	return nil
}
