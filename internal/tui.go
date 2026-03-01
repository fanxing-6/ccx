package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/manifoldco/promptui"
	"golang.org/x/term"
)

// ProfileItem 用于 TUI 展示的 profile 条目
type ProfileItem struct {
	Name    string
	BaseURL string
	Model   string
}

// ttuiSelect 正常 TTY 环境下的交互选择
func ttuiSelect(items []ProfileItem, defaultIdx int) (string, error) {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Name | cyan }}{{ if .BaseURL }}  {{ .BaseURL | faint }}{{ end }}{{ if .Model }}  [{{ .Model | yellow }}]{{ end }}",
		Inactive: "  {{ .Name }}{{ if .BaseURL }}  {{ .BaseURL | faint }}{{ end }}{{ if .Model }}  [{{ .Model }}]{{ end }}",
		Selected: "✓ {{ .Name | green }}",
	}

	searcher := func(input string, index int) bool {
		return strings.Contains(strings.ToLower(items[index].Name), strings.ToLower(input))
	}

	prompt := promptui.Select{
		Label:     "选择 Claude Code 配置",
		Items:     items,
		Templates: templates,
		Size:      10,
		Searcher:  searcher,
		CursorPos: defaultIdx,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return "", err
	}
	return items[idx].Name, nil
}

// fallbackSelect 非 TTY 环境的数字选择回退
func fallbackSelect(items []ProfileItem, defaultIdx int) (string, error) {
	fmt.Println("\n选择 Claude Code 配置：")
	fmt.Println("─────────────────────")
	for i, item := range items {
		marker := "  "
		if i == defaultIdx {
			marker = "* "
		}
		model := ""
		if item.Model != "" {
			model = fmt.Sprintf("  [%s]", item.Model)
		}
		fmt.Printf("%s%d) %-22s %s%s\n", marker, i+1, item.Name, item.BaseURL, model)
	}
	fmt.Printf("\n输入编号 [默认 %d]: ", defaultIdx+1)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return items[defaultIdx].Name, nil
	}

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(items) {
		return "", fmt.Errorf("无效的选择: %s", input)
	}
	return items[idx-1].Name, nil
}

// ConfirmAction 确认操作
func ConfirmAction(message string) bool {
	prompt := promptui.Prompt{
		Label:     message,
		IsConfirm: true,
	}
	result, err := prompt.Run()
	if err != nil {
		return false
	}
	return strings.ToLower(result) == "y"
}

// PromptInput 获取用户输入
func PromptInput(label string, defaultVal string) string {
	prompt := promptui.Prompt{
		Label:   label,
		Default: defaultVal,
	}
	result, _ := prompt.Run()
	result = strings.TrimSpace(result)
	if result == "" {
		return defaultVal
	}
	return result
}

// PromptPassword 获取密码输入（隐藏）
func PromptPassword(label string) string {
	prompt := promptui.Prompt{
		Label: label,
		Mask:  '*',
	}
	result, _ := prompt.Run()
	return result
}

// ActionItem 用于二级菜单的选项
type ActionItem struct {
	Label string
	Key   string
}

// SelectAction 通用二级菜单选择器
func SelectAction(title string, items []ActionItem) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fallbackSelectAction(title, items)
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Label | cyan }}",
		Inactive: "  {{ .Label }}",
		Selected: "✓ {{ .Label | green }}",
	}

	prompt := promptui.Select{
		Label:     title,
		Items:     items,
		Templates: templates,
		Size:      10,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return "", err
	}
	return items[idx].Key, nil
}

// fallbackSelectAction 非 TTY 环境的菜单回退
func fallbackSelectAction(title string, items []ActionItem) (string, error) {
	fmt.Printf("\n%s：\n", title)
	fmt.Println("─────────────────────")
	for i, item := range items {
		fmt.Printf("  %d) %s\n", i+1, item.Label)
	}
	fmt.Print("\n输入编号: ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(items) {
		return "", fmt.Errorf("无效的选择: %s", input)
	}
	return items[idx-1].Key, nil
}

// ReadMultilineJSON 提示用户粘贴 JSON，以空行结束输入，验证后返回
func ReadMultilineJSON(prompt string) ([]byte, error) {
	fmt.Println(prompt)
	fmt.Println("（输入完成后按两次回车或 Ctrl+D 结束）")
	fmt.Println("─────────────────────")

	scanner := bufio.NewScanner(os.Stdin)
	// 增大缓冲区以支持长行粘贴
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var lines []string
	emptyCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			emptyCount++
			if emptyCount >= 1 && len(lines) > 0 {
				break
			}
			continue
		}
		emptyCount = 0
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("未输入任何内容")
	}

	content := []byte(strings.Join(lines, "\n"))
	if !json.Valid(content) {
		return nil, fmt.Errorf("JSON 格式无效")
	}
	return content, nil
}

// SelectProfileEx 交互式选择（接受预构建的 items 列表，含"配置管理"等额外选项）
func SelectProfileEx(items []ProfileItem, defaultIdx int) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fallbackSelect(items, defaultIdx)
	}
	return ttuiSelect(items, defaultIdx)
}

// ShortenURL 缩短 URL 展示（导出版本）
func ShortenURL(u string) string {
	return shortenURL(u)
}

// shortenURL 缩短 URL 展示
func shortenURL(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimSuffix(u, "/")
	if len(u) > 45 {
		return u[:42] + "..."
	}
	return u
}
