package cmd

import (
	"ccx/internal"
	"ccx/internal/proxy"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
  ccx auth status      # 透传到 Claude CLI（非 ccx 命令）
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
			profileName = args[0]
		} else {
			for {
				fmt.Println("正在从 Gitee Gist 获取配置列表...")
				profiles, err := client.FetchAllProfiles()
				if err != nil {
					return err
				}
				if len(profiles) == 0 {
					return fmt.Errorf("Gitee Gist 中没有配置，运行 ccx add <name> 创建")
				}

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

func selectProfileOrConfig(profiles []*internal.Profile, defaultProfile string) (string, error) {
	items := make([]internal.ProfileItem, 0, len(profiles)+1)
	defaultIdx := 0
	for i, p := range profiles {
		info := internal.ExtractProfileInfo(p.Settings)
		name := p.Name
		if strings.EqualFold(info.APIFormat, "openai") {
			name = p.Name + " [openai]"
		}
		item := internal.ProfileItem{
			Name:    name,
			BaseURL: internal.ShortenURL(info.BaseURL),
			Model:   info.Model,
		}
		items = append(items, item)
		if p.Name == defaultProfile {
			defaultIdx = i
		}
	}

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
	if strings.HasSuffix(selected, " [openai]") {
		return strings.TrimSuffix(selected, " [openai]"), nil
	}
	return selected, nil
}

func launchClaude(profileName string, extraArgs []string, dangerous bool, client *internal.GistClient, cfg *internal.AppConfig) error {
	profile, err := client.FetchProfile(profileName)
	if err != nil {
		return err
	}

	if !json.Valid(profile.Settings) {
		return fmt.Errorf("配置 %q 的 settings JSON 格式损坏，请运行 ccx edit %s 修复", profileName, profileName)
	}

	info := internal.ExtractProfileInfo(profile.Settings)
	apiFormat := strings.ToLower(strings.TrimSpace(info.APIFormat))
	if apiFormat == "" {
		apiFormat = "anthropic"
	}

	token, err := resolveProfileToken(info)
	if err != nil {
		return fmt.Errorf("配置 %q 鉴权冲突: %w", profileName, err)
	}

	claudePath, err := exec.LookPath(cfg.ClaudeCmd)
	if err != nil {
		return fmt.Errorf("未找到 %s 命令，请确认已安装 Claude Code", cfg.ClaudeCmd)
	}

	if apiFormat != "openai" {
		settingsForClaude, err := buildClaudeSettings(profile.Settings, token, "")
		if err != nil {
			return err
		}

		cmdArgs := []string{cfg.ClaudeCmd, "--settings", string(settingsForClaude)}
		if dangerous {
			cmdArgs = append(cmdArgs, "--dangerously-skip-permissions")
		}
		cmdArgs = append(cmdArgs, extraArgs...)

		printLaunchSummary(cfg.ClaudeCmd, profileName, info, dangerous, extraArgs, "", token)
		return syscall.Exec(claudePath, cmdArgs, os.Environ())
	}

	if info.BaseURL == "" {
		return fmt.Errorf("openai 模式需要设置 ANTHROPIC_BASE_URL")
	}
	if token == "" {
		return fmt.Errorf("openai 模式需要设置 ANTHROPIC_API_KEY 或 ANTHROPIC_AUTH_TOKEN")
	}

	port, shutdown, err := proxy.StartProxy(info.BaseURL, token, proxy.ProxyOptions{
		ReasoningEffort: info.ReasoningEffort,
	})
	if err != nil {
		return err
	}
	proxyURL := proxy.ProxyURL(port)

	settingsForClaude, err := buildClaudeSettings(profile.Settings, token, proxyURL)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(ctx)
		return err
	}

	args := []string{"--settings", string(settingsForClaude)}
	if dangerous {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, extraArgs...)

	cmd := exec.Command(claudePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	printLaunchSummary(cfg.ClaudeCmd, profileName, info, dangerous, extraArgs, proxyURL, token)

	if err := cmd.Start(); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(ctx)
		return fmt.Errorf("启动 Claude 子进程失败: %w", err)
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		for sig := range sigCh {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		}
	}()

	waitErr := cmd.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := shutdown(ctx)

	if waitErr != nil {
		return waitErr
	}
	if shutdownErr != nil {
		return fmt.Errorf("代理关闭失败: %w", shutdownErr)
	}
	return nil
}

func resolveProfileToken(info internal.ProfileInfo) (string, error) {
	apiKey := strings.TrimSpace(info.APIKey)
	authToken := strings.TrimSpace(info.AuthToken)

	if apiKey != "" && authToken != "" && apiKey != authToken {
		return "", fmt.Errorf("ANTHROPIC_API_KEY 与 ANTHROPIC_AUTH_TOKEN 不一致")
	}
	if apiKey != "" {
		return apiKey, nil
	}
	if authToken != "" {
		return authToken, nil
	}
	return "", nil
}

func buildClaudeSettings(original json.RawMessage, token, forceBaseURL string) ([]byte, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(original, &obj); err != nil {
		return nil, fmt.Errorf("解析 settings 失败: %w", err)
	}

	env := map[string]interface{}{}
	if existingEnv, ok := obj["env"]; ok {
		typed, ok := existingEnv.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("settings.env 必须是对象")
		}
		env = typed
	}

	if token != "" {
		env["ANTHROPIC_API_KEY"] = token
	}
	delete(env, "ANTHROPIC_AUTH_TOKEN")
	if forceBaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = forceBaseURL
	}

	obj["env"] = env
	// api_format 是 ccx 内部字段，不传递给 Claude Code
	delete(obj, "api_format")

	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("生成 settings 失败: %w", err)
	}
	return out, nil
}

func printLaunchSummary(claudeCmd, profileName string, info internal.ProfileInfo, dangerous bool, extraArgs []string, proxyURL, token string) {
	fmt.Printf("\n=> %s --settings '{...}' ", claudeCmd)
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
	if strings.EqualFold(info.APIFormat, "openai") {
		fmt.Printf("\n   proxy: %s", proxyURL)
		fmt.Printf("\n   token: %s", proxy.MaskToken(token))
		if strings.TrimSpace(info.ReasoningEffort) != "" {
			fmt.Printf("\n   reasoning: %s", strings.TrimSpace(info.ReasoningEffort))
		}
	}
	fmt.Println()
	fmt.Println()
}

func Execute() {
	if passArgs, dangerous, ok := decideRawPassthrough(os.Args[1:]); ok {
		if err := launchClaudePassthrough(resolvePassthroughClaudeCmd(), passArgs, dangerous); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
