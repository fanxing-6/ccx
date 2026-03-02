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
	fmt.Println("准备工作（需要浏览器操作）：")
	fmt.Println("  1) 创建/查看 Gist: https://gitee.com/dashboard/codes")
	fmt.Println("  2) Gist URL 形如 https://gitee.com/<owner>/codes/<gistID>")
	fmt.Println("     其中 <gistID> 就是下面要输入的 Gist ID")

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
	gistID := internal.PromptInput("Gist ID（例：https://gitee.com/<owner>/codes/<gistID> 中的 <gistID>）", defaultGistID)
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
		return fmt.Errorf(
			"连接 Gitee 失败: %w\n\n排查建议:\n- 打开并核对 Gist 页面: https://gitee.com/%s/codes/%s\n- 确认 Token 有 Gist 读写权限\n- dashboard: https://gitee.com/dashboard/codes",
			err,
			owner,
			gistID,
		)
	}
	fmt.Printf("连接成功，发现 %d 个配置文件\n", len(files))

	// 保存配置
	internal.SaveAppConfig(cfg)
	fmt.Printf("配置已保存到 %s\n", internal.ConfigPath())

	return nil
}
