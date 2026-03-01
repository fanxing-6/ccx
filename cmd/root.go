package cmd

import (
	"ccx/internal"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

// Version 编译时注入，go build -ldflags "-X ccx/cmd.Version=x.x.x"
var Version = "dev"

func init() {
	rootCmd.Flags().BoolP("dangerous", "d", false, "危险模式：跳过权限确认 (--dangerously-skip-permissions)")
	rootCmd.Version = Version
}

var rootCmd = &cobra.Command{
	Use:   "ccx [profile]",
	Short: "CCX - Claude Code eXecutor",
	Long:  "快速选择配置并启动 Claude Code，所有配置存储在 Gitee Gist 云端",
	Example: `  ccx                  # 交互式选择配置并启动
  ccx volc             # 直接使用 volc 配置启动
  ccx -d volc          # 危险模式启动（跳过权限确认）
  ccx list             # 列出所有配置
  ccx info volc        # 查看 volc 配置详情`,
	Args: cobra.ArbitraryArgs,
	FParseErrWhitelist: cobra.FParseErrWhitelist{
		UnknownFlags: true,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if !internal.ConfigExists() {
			fmt.Println("首次使用，请先初始化配置：")
			return initRun()
		}

		cfg, err := internal.LoadAppConfig()
		if err != nil {
			return err
		}
		client := internal.NewGistClientFromConfig(cfg)

		var profileName string

		if len(args) > 0 {
			// ccx <name> 直接启动
			profileName = args[0]
		} else {
			// ccx 交互式选择（含配置管理入口），循环直到选中 profile
			for {
				fmt.Println("正在从 Gitee Gist 获取配置列表...")
				// 每次循环重新拉取，确保配置管理操作后列表是最新的
				profiles, err := client.FetchAllProfiles()
				if err != nil {
					return err
				}
				if len(profiles) == 0 {
					return fmt.Errorf("Gitee Gist 中没有配置，运行 ccx add <name> 创建")
				}

				// 重新加载 cfg 以获取最新的 default_profile
				cfg, _ = internal.LoadAppConfig()

				selected, err := selectProfileOrConfig(profiles, cfg.DefaultProfile)
				if err != nil {
					return fmt.Errorf("已取消")
				}

				if selected == "__config__" {
					if err := configMenu(); err != nil {
						fmt.Printf("操作出错: %v\n\n", err)
					}
					continue
				}
				profileName = selected
				break
			}
		}

		var extraArgs []string
		if len(args) > 1 {
			extraArgs = args[1:]
		}
		dangerous, _ := cmd.Flags().GetBool("dangerous")
		return launchClaude(profileName, extraArgs, dangerous, client, cfg)
	},
}

// selectProfileOrConfig 主选择界面：profile 列表 + "配置管理"入口
func selectProfileOrConfig(profiles []*internal.Profile, defaultProfile string) (string, error) {
	// 构建 profile 列表项
	items := make([]internal.ProfileItem, 0, len(profiles)+1)
	defaultIdx := 0
	for i, p := range profiles {
		info := internal.ExtractProfileInfo(p.Settings)
		item := internal.ProfileItem{
			Name:    p.Name,
			BaseURL: internal.ShortenURL(info.BaseURL),
			Model:   info.Model,
		}
		items = append(items, item)
		if p.Name == defaultProfile {
			defaultIdx = i
		}
	}

	// 追加"配置管理"选项
	items = append(items, internal.ProfileItem{
		Name:    "⚙ 配置管理",
		BaseURL: "",
		Model:   "",
	})

	selected, err := internal.SelectProfileEx(items, defaultIdx)
	if err != nil {
		return "", err
	}

	if selected == "⚙ 配置管理" {
		return "__config__", nil
	}
	return selected, nil
}

// launchClaude 使用指定 profile 启动 claude
func launchClaude(profileName string, extraArgs []string, dangerous bool, client *internal.GistClient, cfg *internal.AppConfig) error {
	profile, err := client.FetchProfile(profileName)
	if err != nil {
		return err
	}

	// 直接使用 profile settings
	settings := profile.Settings

	// 校验合并后的 settings JSON 合法性
	if !json.Valid(settings) {
		return fmt.Errorf("配置 %q 的 settings JSON 格式损坏，请运行 ccx edit %s 修复", profileName, profileName)
	}

	claudePath, err := exec.LookPath(cfg.ClaudeCmd)
	if err != nil {
		return fmt.Errorf("未找到 %s 命令，请确认已安装 Claude Code", cfg.ClaudeCmd)
	}

	cmdArgs := []string{cfg.ClaudeCmd, "--settings", string(settings)}
	if dangerous {
		cmdArgs = append(cmdArgs, "--dangerously-skip-permissions")
	}
	cmdArgs = append(cmdArgs, extraArgs...)

	// 打印完整执行命令摘要
	info := internal.ExtractProfileInfo(profile.Settings)
	fmt.Printf("\n=> %s --settings '{...}' ", cfg.ClaudeCmd)
	if dangerous {
		fmt.Print("--dangerously-skip-permissions ")
	}
	for _, a := range extraArgs {
		fmt.Printf("%s ", a)
	}
	fmt.Printf("\n   profile: %s | %s", profileName, info.BaseURL)
	if info.Model != "" {
		fmt.Printf(" [%s]", info.Model)
	}
	fmt.Println()
	fmt.Println()

	// 用 syscall.Exec 替换当前进程
	return syscall.Exec(claudePath, cmdArgs, os.Environ())
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
