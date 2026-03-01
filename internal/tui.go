package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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

// ModelInfo 模型信息
type ModelInfo struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

// ModelsResponse /v1/models 接口响应
type ModelsResponse struct {
	Data   []ModelInfo `json:"data"`
	Object string      `json:"object"`
}

// FetchModels 从 API 获取模型列表
func FetchModels(baseURL, apiKey string) ([]ModelInfo, error) {
	url := strings.TrimSuffix(baseURL, "/") + "/models"

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("认证失败 (401)，请检查 API Key 是否正确")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("权限不足 (403)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回错误状态: %d", resp.StatusCode)
	}

	var result ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("API 未返回任何模型")
	}

	return result.Data, nil
}

// SelectModelInteractive 交互式选择模型
// 支持分页、搜索、手动输入
func SelectModelInteractive(models []ModelInfo, defaultModel string) (string, error) {
	// 提取模型 ID 列表
	modelIDs := make([]string, len(models))
	for i, m := range models {
		modelIDs[i] = m.ID
	}

	// 查找默认模型索引
	defaultIdx := 0
	for i, id := range modelIDs {
		if id == defaultModel {
			defaultIdx = i
			break
		}
	}

	pageSize := 10
	currentPage := defaultIdx / pageSize
	totalPages := (len(modelIDs) + pageSize - 1) / pageSize

	// 搜索模式
	filtered := modelIDs
	isSearchMode := false

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Println("\n选择模型：")
		fmt.Println("─────────────────────")

		if isSearchMode {
			fmt.Printf("[搜索模式] 输入关键字过滤，或直接回车退出搜索\n\n")
		}

		// 计算当前页显示范围
		start := currentPage * pageSize
		end := start + pageSize
		if end > len(filtered) {
			end = len(filtered)
		}

		// 显示当前页模型
		for i := start; i < end; i++ {
			marker := "  "
			if filtered[i] == defaultModel {
				marker = "* "
			}
			// 查找 display_name
			displayName := ""
			for _, m := range models {
				if m.ID == filtered[i] && m.DisplayName != "" {
					displayName = fmt.Sprintf("  (%s)", m.DisplayName)
					break
				}
			}
			fmt.Printf("%s%d) %s%s\n", marker, i-start+1, filtered[i], displayName)
		}

		// 显示分页信息
		if totalPages > 1 || isSearchMode {
			fmt.Printf("\n页码: %d/%d (共 %d 个模型)\n", currentPage+1, totalPages, len(filtered))
		} else {
			fmt.Printf("\n共 %d 个模型\n", len(filtered))
		}

		// 显示操作提示
		fmt.Println("─────────────────────")
		fmt.Print("操作: 数字选择")
		if len(filtered) > pageSize && !isSearchMode {
			fmt.Print(" | n-下一页 | p-上一页")
		}
		if !isSearchMode {
			fmt.Print(" | /-搜索")
		}
		fmt.Print(" | m-手动输入 | 回车-默认)\n")
		fmt.Print("> ")

		if !scanner.Scan() {
			return defaultModel, nil
		}

		input := strings.TrimSpace(scanner.Text())

		// 空输入：使用默认值
		if input == "" {
			return defaultModel, nil
		}

		// 搜索模式
		if input == "/" && !isSearchMode {
			isSearchMode = true
			fmt.Print("输入搜索关键字: ")
			if !scanner.Scan() {
				isSearchMode = false
				continue
			}
			keyword := strings.TrimSpace(scanner.Text())
			if keyword == "" {
				isSearchMode = false
				filtered = modelIDs
				currentPage = 0
				totalPages = (len(filtered) + pageSize - 1) / pageSize
				continue
			}
			// 过滤
			filtered = []string{}
			for _, id := range modelIDs {
				if strings.Contains(strings.ToLower(id), strings.ToLower(keyword)) {
					filtered = append(filtered, id)
				}
			}
			currentPage = 0
			totalPages = (len(filtered) + pageSize - 1) / pageSize
			if len(filtered) == 0 {
				fmt.Printf("未找到包含 '%s' 的模型，显示全部\n", keyword)
				filtered = modelIDs
				totalPages = (len(filtered) + pageSize - 1) / pageSize
			}
			continue
		}

		// 退出搜索模式
		if isSearchMode && input == "" {
			isSearchMode = false
			filtered = modelIDs
			currentPage = defaultIdx / pageSize
			totalPages = (len(filtered) + pageSize - 1) / pageSize
			continue
		}

		// 下一页
		if input == "n" && !isSearchMode {
			if currentPage < totalPages-1 {
				currentPage++
			}
			continue
		}

		// 上一页
		if input == "p" && !isSearchMode {
			if currentPage > 0 {
				currentPage--
			}
			continue
		}

		// 手动输入
		if input == "m" {
			fmt.Print("输入模型名称: ")
			if !scanner.Scan() {
				continue
			}
			manual := strings.TrimSpace(scanner.Text())
			if manual != "" {
				return manual, nil
			}
			continue
		}

		// 数字选择
		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > (end-start) {
			fmt.Println("无效输入，请重试")
			continue
		}

		return filtered[start+idx-1], nil
	}
}
