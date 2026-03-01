package cmd

import (
	"ccx/internal"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(removeCmd)

	addCmd.Flags().Bool("editor", false, "使用编辑器模式（而非交互式引导）")
}

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "添加新配置（直接创建到 Gitee Gist）",
	Example: `  ccx add myapi            # 交互式引导创建
  ccx add myapi --editor   # 使用编辑器创建`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := internal.LoadAppConfig()
		if err != nil {
			return err
		}
		client := internal.NewGistClientFromConfig(cfg)

		exists, err := client.ProfileExists(name)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("配置 %q 已存在，使用 ccx edit %s 修改", name, name)
		}

		useEditor, _ := cmd.Flags().GetBool("editor")
		var content []byte

		if useEditor {
			template := map[string]interface{}{
				"api_format": "anthropic",
				"env": map[string]string{
					"ANTHROPIC_API_KEY":    "",
					"ANTHROPIC_AUTH_TOKEN": "",
					"ANTHROPIC_BASE_URL":   "",
					"API_TIMEOUT_MS":       "600000",
				},
			}
			tmpl, _ := json.MarshalIndent(template, "", "  ")
			edited, err := openEditor(tmpl)
			if err != nil {
				return err
			}
			if !json.Valid(edited) {
				return fmt.Errorf("JSON 格式无效")
			}
			content = edited
		} else {
			result, err := interactiveAddProfile()
			if err != nil {
				return err
			}
			content = result
		}

		// 直接上传到 Gist
		filename := internal.ProfileNameToGistFile(name)
		err = client.UploadFile(filename, string(content))
		if err != nil {
			return fmt.Errorf("上传到 Gitee Gist 失败: %w", err)
		}
		fmt.Printf("配置 %s 已创建到 Gitee Gist\n", name)
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "编辑已有配置（从 Gist 下载 → 编辑 → 上传）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		return editProfile(name)
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "删除配置（从 Gitee Gist 删除）",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		return removeProfile(name)
	},
}

func promptAPIFormat() (string, error) {
	choices := []internal.ActionItem{
		{Label: "Anthropic（默认）", Key: "anthropic"},
		{Label: "OpenAI Responses API", Key: "openai"},
	}
	selected, err := internal.SelectAction("选择 API 格式", choices)
	if err != nil {
		return "", fmt.Errorf("选择 API 格式失败: %w", err)
	}
	return selected, nil
}

// interactiveAddProfile 交互式引导创建 profile
func interactiveAddProfile() ([]byte, error) {
	fmt.Println("创建新配置（交互式引导）")
	fmt.Println("─────────────────────")

	apiFormat, err := promptAPIFormat()
	if err != nil {
		return nil, err
	}

	token := internal.PromptPassword("API Token（将写入 ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN）")
	if token == "" {
		return nil, fmt.Errorf("Token 不能为空")
	}

	baseURL := internal.PromptInput("ANTHROPIC_BASE_URL", "")
	if baseURL == "" {
		return nil, fmt.Errorf("Base URL 不能为空")
	}

	model := internal.PromptInput("ANTHROPIC_MODEL（留空不设置）", "")
	haikuModel := internal.PromptInput("ANTHROPIC_DEFAULT_HAIKU_MODEL（留空不设置）", "")
	sonnetModel := internal.PromptInput("ANTHROPIC_DEFAULT_SONNET_MODEL（留空不设置）", "")
	opusModel := internal.PromptInput("ANTHROPIC_DEFAULT_OPUS_MODEL（留空不设置）", "")
	timeout := internal.PromptInput("API_TIMEOUT_MS", "600000")

	env := map[string]string{
		"ANTHROPIC_API_KEY":    token,
		"ANTHROPIC_AUTH_TOKEN": token,
		"ANTHROPIC_BASE_URL":   baseURL,
		"API_TIMEOUT_MS":       timeout,
	}
	if model != "" {
		env["ANTHROPIC_MODEL"] = model
	}
	if haikuModel != "" {
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haikuModel
	}
	if sonnetModel != "" {
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = sonnetModel
	}
	if opusModel != "" {
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = opusModel
	}

	settings := map[string]interface{}{
		"env": env,
	}
	if apiFormat == "openai" {
		settings["api_format"] = "openai"
	}

	content, _ := json.MarshalIndent(settings, "", "  ")
	fmt.Println("\n生成的配置：")
	fmt.Println(string(content))
	return content, nil
}

// editProfile 从 Gist 下载 → 编辑器编辑 → 上传回 Gist
func editProfile(name string) error {
	cfg, err := internal.LoadAppConfig()
	if err != nil {
		return err
	}
	client := internal.NewGistClientFromConfig(cfg)

	profile, err := client.FetchProfile(name)
	if err != nil {
		return err
	}

	formatted, _ := json.MarshalIndent(json.RawMessage(profile.Settings), "", "  ")
	edited, err := openEditor(formatted)
	if err != nil {
		return err
	}

	if !json.Valid(edited) {
		return fmt.Errorf("JSON 格式无效")
	}

	// 上传回 Gist
	filename := internal.ProfileNameToGistFile(name)
	err = client.UploadFile(filename, string(edited))
	if err != nil {
		return fmt.Errorf("上传到 Gitee Gist 失败: %w", err)
	}
	fmt.Printf("配置 %s 已更新到 Gitee Gist\n", name)
	return nil
}

// removeProfile 从 Gist 删除指定 profile
func removeProfile(name string) error {
	cfg, err := internal.LoadAppConfig()
	if err != nil {
		return err
	}
	client := internal.NewGistClientFromConfig(cfg)

	exists, err := client.ProfileExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("配置 %q 在 Gitee Gist 中不存在", name)
	}

	if !internal.ConfirmAction(fmt.Sprintf("确认从 Gitee Gist 删除配置 %q", name)) {
		fmt.Println("已取消")
		return nil
	}

	filename := internal.ProfileNameToGistFile(name)
	err = client.DeleteFile(filename)
	if err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	fmt.Printf("配置 %s 已从 Gitee Gist 删除\n", name)
	return nil
}

// configMenu 配置管理二级菜单（从主界面进入），循环直到用户选择返回
func configMenu() error {
	for {
		items := []internal.ActionItem{
			{Label: "新建配置", Key: "add"},
			{Label: "修改配置", Key: "edit"},
			{Label: "删除配置", Key: "remove"},
			{Label: "设置默认", Key: "default"},
			{Label: "← 返回", Key: "__back__"},
		}

		selected, err := internal.SelectAction("配置管理", items)
		if err != nil || selected == "__back__" {
			return nil
		}

		var opErr error
		switch selected {
		case "add":
			opErr = configMenuAdd()
		case "edit":
			opErr = configMenuEdit()
		case "remove":
			opErr = configMenuRemove()
		case "default":
			opErr = configMenuDefault()
		}
		if opErr != nil {
			fmt.Printf("操作出错: %v\n\n", opErr)
		}
		fmt.Println()
	}
}

// configMenuAdd 交互式新建配置
func configMenuAdd() error {
	name := internal.PromptInput("配置名称", "")
	if name == "" {
		return fmt.Errorf("配置名称不能为空")
	}

	cfg, err := internal.LoadAppConfig()
	if err != nil {
		return err
	}
	client := internal.NewGistClientFromConfig(cfg)

	exists, err := client.ProfileExists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("配置 %q 已存在，请选择「修改配置」", name)
	}

	content, err := interactiveAddProfile()
	if err != nil {
		return err
	}

	filename := internal.ProfileNameToGistFile(name)
	err = client.UploadFile(filename, string(content))
	if err != nil {
		return fmt.Errorf("上传到 Gitee Gist 失败: %w", err)
	}
	fmt.Printf("配置 %s 已创建到 Gitee Gist\n", name)
	return nil
}

// configMenuEdit 交互式选择并编辑配置
func configMenuEdit() error {
	cfg, err := internal.LoadAppConfig()
	if err != nil {
		return err
	}
	client := internal.NewGistClientFromConfig(cfg)

	profiles, err := client.FetchAllProfiles()
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Println("没有可编辑的配置")
		return nil
	}

	items := make([]internal.ActionItem, 0, len(profiles)+1)
	for _, p := range profiles {
		items = append(items, internal.ActionItem{Label: p.Name, Key: p.Name})
	}
	items = append(items, internal.ActionItem{Label: "← 返回", Key: "__back__"})

	selected, err := internal.SelectAction("选择要编辑的配置", items)
	if err != nil || selected == "__back__" {
		return nil
	}

	return editProfile(selected)
}

// configMenuRemove 交互式选择并删除配置
func configMenuRemove() error {
	cfg, err := internal.LoadAppConfig()
	if err != nil {
		return err
	}
	client := internal.NewGistClientFromConfig(cfg)

	profiles, err := client.FetchAllProfiles()
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Println("没有可删除的配置")
		return nil
	}

	items := make([]internal.ActionItem, 0, len(profiles)+1)
	for _, p := range profiles {
		items = append(items, internal.ActionItem{Label: p.Name, Key: p.Name})
	}
	items = append(items, internal.ActionItem{Label: "← 返回", Key: "__back__"})

	selected, err := internal.SelectAction("选择要删除的配置", items)
	if err != nil || selected == "__back__" {
		return nil
	}

	return removeProfile(selected)
}

// configMenuDefault 交互式设置默认配置
func configMenuDefault() error {
	cfg, err := internal.LoadAppConfig()
	if err != nil {
		return err
	}
	client := internal.NewGistClientFromConfig(cfg)

	profiles, err := client.FetchAllProfiles()
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Println("没有可用的配置")
		return nil
	}

	currentDefault := cfg.DefaultProfile
	if currentDefault != "" {
		fmt.Printf("当前默认配置: %s\n\n", currentDefault)
	}

	items := make([]internal.ActionItem, 0, len(profiles)+1)
	for _, p := range profiles {
		label := p.Name
		if p.Name == currentDefault {
			label = p.Name + " (当前默认)"
		}
		items = append(items, internal.ActionItem{Label: label, Key: p.Name})
	}
	items = append(items, internal.ActionItem{Label: "← 返回", Key: "__back__"})

	selected, err := internal.SelectAction("选择默认配置", items)
	if err != nil || selected == "__back__" {
		return nil
	}

	cfg.DefaultProfile = selected
	internal.SaveAppConfig(cfg)
	fmt.Printf("默认配置已设置为: %s\n", selected)
	return nil
}

// openEditor 打开编辑器编辑内容
func openEditor(content []byte) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "ccx-*.json")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	tmpFile.Write(content)
	tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	err = editorCmd.Run()
	if err != nil {
		return nil, fmt.Errorf("编辑器退出异常: %w", err)
	}

	return os.ReadFile(tmpPath)
}
