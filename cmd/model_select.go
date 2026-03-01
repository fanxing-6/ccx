package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"ccx/internal"

	"github.com/tidwall/gjson"
)

const (
	modelFetchTimeout   = 25 * time.Second
	modelPageSize       = 10
	modelManualKey      = "__manual__"
	modelSkipKey        = "__skip__"
	modelNextKey        = "__next__"
	modelPrevKey        = "__prev__"
	modelSearchKey      = "__search__"
	modelClearSearchKey = "__clear_search__"
)

type modelInfo struct {
	ID          string
	DisplayName string
}

func selectOrInputModel(baseURL, token string) string {
	models, err := fetchModels(baseURL, token)
	if err != nil {
		fmt.Printf("获取模型列表失败: %v\n", err)
		return internal.PromptInput("ANTHROPIC_MODEL（留空不设置）", "")
	}
	if len(models) == 0 {
		fmt.Println("API 未返回任何模型，将改为手动输入")
		return internal.PromptInput("ANTHROPIC_MODEL（留空不设置）", "")
	}

	fmt.Printf("已获取 %d 个模型，请选择：\n", len(models))
	selected, err := selectModelInteractive(models)
	if err != nil {
		fmt.Printf("模型选择失败: %v\n", err)
		return internal.PromptInput("ANTHROPIC_MODEL（留空不设置）", "")
	}
	switch selected {
	case modelManualKey:
		return internal.PromptInput("ANTHROPIC_MODEL（留空不设置）", "")
	case modelSkipKey:
		return ""
	default:
		return selected
	}
}

func fetchModels(baseURL, apiKey string) ([]modelInfo, error) {
	candidates := modelEndpointCandidates(baseURL)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("Base URL 无效")
	}

	client := &http.Client{Timeout: modelFetchTimeout}
	var lastErr error

	for _, endpoint := range candidates {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = fmt.Errorf("%s 请求构造失败: %w", endpoint, err)
			continue
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("X-Api-Key", apiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s 请求失败: %w", endpoint, err)
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("%s 读取响应失败: %w", endpoint, readErr)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			msg := extractModelFetchError(body)
			lastErr = fmt.Errorf("%s 返回 %d: %s", endpoint, resp.StatusCode, msg)
			continue
		}

		models, parseErr := parseModelList(body)
		if parseErr != nil {
			lastErr = fmt.Errorf("%s 响应解析失败: %w", endpoint, parseErr)
			continue
		}
		if len(models) == 0 {
			lastErr = fmt.Errorf("%s 未返回任何模型", endpoint)
			continue
		}
		return models, nil
	}

	if lastErr == nil {
		return nil, fmt.Errorf("模型接口不可用")
	}
	return nil, lastErr
}

func parseModelList(body []byte) ([]modelInfo, error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("非 JSON 响应")
	}

	data := gjson.GetBytes(body, "data")
	if !data.Exists() || !data.IsArray() {
		return nil, fmt.Errorf("缺少 data 数组")
	}

	seen := make(map[string]struct{})
	models := make([]modelInfo, 0)
	data.ForEach(func(_, item gjson.Result) bool {
		id := strings.TrimSpace(item.Get("id").String())
		if id == "" {
			return true
		}
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
		display := strings.TrimSpace(item.Get("display_name").String())
		models = append(models, modelInfo{
			ID:          id,
			DisplayName: display,
		})
		return true
	})

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

func modelEndpointCandidates(baseURL string) []string {
	base := strings.TrimSpace(baseURL)
	base = strings.TrimSuffix(base, "/")
	if base == "" {
		return nil
	}

	candidates := make([]string, 0, 2)
	seen := make(map[string]struct{})
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		candidates = append(candidates, v)
	}

	add(base + "/models")
	if !strings.HasSuffix(base, "/v1") {
		add(base + "/v1/models")
	}
	return candidates
}

func extractModelFetchError(body []byte) string {
	if len(body) == 0 {
		return "空响应"
	}
	if !json.Valid(body) {
		raw := strings.TrimSpace(string(body))
		if len(raw) > 220 {
			raw = raw[:220] + "..."
		}
		if raw == "" {
			return "非 JSON 响应"
		}
		return raw
	}

	if msg := gjson.GetBytes(body, "error.message"); msg.Exists() && strings.TrimSpace(msg.String()) != "" {
		return msg.String()
	}
	if msg := gjson.GetBytes(body, "message"); msg.Exists() && strings.TrimSpace(msg.String()) != "" {
		return msg.String()
	}

	raw := strings.TrimSpace(string(body))
	if len(raw) > 220 {
		raw = raw[:220] + "..."
	}
	if raw == "" {
		return "未知错误"
	}
	return raw
}

func selectModelInteractive(models []modelInfo) (string, error) {
	if len(models) == 0 {
		return modelManualKey, nil
	}

	all := models
	filtered := models
	page := 0
	keyword := ""

	for {
		totalPages := (len(filtered) + modelPageSize - 1) / modelPageSize
		if totalPages == 0 {
			totalPages = 1
		}
		if page >= totalPages {
			page = totalPages - 1
		}
		if page < 0 {
			page = 0
		}

		start := page * modelPageSize
		end := start + modelPageSize
		if end > len(filtered) {
			end = len(filtered)
		}

		items := make([]internal.ActionItem, 0, (end-start)+6)
		if start < end {
			for _, model := range filtered[start:end] {
				items = append(items, internal.ActionItem{
					Label: formatModelLabel(model),
					Key:   "model:" + model.ID,
				})
			}
		}

		if totalPages > 1 && page > 0 {
			items = append(items, internal.ActionItem{Label: "上一页", Key: modelPrevKey})
		}
		if totalPages > 1 && page < totalPages-1 {
			items = append(items, internal.ActionItem{Label: "下一页", Key: modelNextKey})
		}
		items = append(items, internal.ActionItem{Label: "搜索模型", Key: modelSearchKey})
		if keyword != "" {
			items = append(items, internal.ActionItem{Label: "清除搜索", Key: modelClearSearchKey})
		}
		items = append(items, internal.ActionItem{Label: "手动输入模型名", Key: modelManualKey})
		items = append(items, internal.ActionItem{Label: "留空不设置模型", Key: modelSkipKey})

		title := fmt.Sprintf("选择模型 (%d/%d 页，当前 %d 个，共 %d 个)", page+1, totalPages, len(filtered), len(all))
		selected, err := internal.SelectAction(title, items)
		if err != nil {
			return "", err
		}

		switch selected {
		case modelPrevKey:
			page--
		case modelNextKey:
			page++
		case modelSearchKey:
			q := strings.TrimSpace(internal.PromptInput("输入搜索关键词（匹配模型 ID/显示名）", keyword))
			keyword = q
			filtered = filterModels(all, keyword)
			if len(filtered) == 0 {
				fmt.Printf("未匹配到模型：%s，已恢复显示全部模型\n", keyword)
				filtered = all
				keyword = ""
			}
			page = 0
		case modelClearSearchKey:
			filtered = all
			keyword = ""
			page = 0
		case modelManualKey, modelSkipKey:
			return selected, nil
		default:
			if strings.HasPrefix(selected, "model:") {
				return strings.TrimPrefix(selected, "model:"), nil
			}
		}
	}
}

func filterModels(models []modelInfo, keyword string) []modelInfo {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return models
	}
	out := make([]modelInfo, 0, len(models))
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID), keyword) || strings.Contains(strings.ToLower(m.DisplayName), keyword) {
			out = append(out, m)
		}
	}
	return out
}

func formatModelLabel(model modelInfo) string {
	display := strings.TrimSpace(model.DisplayName)
	if display == "" || strings.EqualFold(display, model.ID) {
		return model.ID
	}
	return fmt.Sprintf("%s  (%s)", model.ID, display)
}
