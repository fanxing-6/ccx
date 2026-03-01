package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

const giteeAPIBase = "https://gitee.com/api/v5"

// GistClient 封装 Gitee Gist API 操作
type GistClient struct {
	Token  string
	GistID string
	Owner  string
	client *http.Client
}

// GistFile 表示 Gist 中的一个文件
type GistFile struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	RawURL   string `json:"raw_url"`
}

// NewGistClient 创建 Gitee Gist 客户端
func NewGistClient(token, owner, gistID string) *GistClient {
	return &GistClient{
		Token:  token,
		GistID: gistID,
		Owner:  owner,
		client: &http.Client{},
	}
}

// NewGistClientFromConfig 从 AppConfig 创建客户端
func NewGistClientFromConfig(cfg *AppConfig) *GistClient {
	return NewGistClient(cfg.GiteeToken, cfg.GistOwner, cfg.GistID)
}

// ListSettingsFiles 获取 Gist 中所有 settings-*.json 文件
func (g *GistClient) ListSettingsFiles() (map[string]GistFile, error) {
	url := fmt.Sprintf("%s/gists/%s?access_token=%s", giteeAPIBase, g.GistID, g.Token)
	resp, err := g.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求 Gitee API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitee API 返回 %d: %s", resp.StatusCode, string(body))
	}

	var gist struct {
		Files map[string]GistFile `json:"files"`
	}
	json.NewDecoder(resp.Body).Decode(&gist)

	// 过滤出 settings-*.json 文件
	result := make(map[string]GistFile)
	for name, file := range gist.Files {
		if strings.HasPrefix(name, "settings-") && strings.HasSuffix(name, ".json") {
			result[name] = file
		}
	}
	return result, nil
}

// FetchAllProfiles 从 Gist 拉取所有 profile，返回按名称排序的列表
func (g *GistClient) FetchAllProfiles() ([]*Profile, error) {
	files, err := g.ListSettingsFiles()
	if err != nil {
		return nil, err
	}

	var profiles []*Profile
	for filename, file := range files {
		name := GistFileToProfileName(filename)
		var content []byte
		if file.Content != "" {
			content = []byte(file.Content)
		} else {
			content, err = g.DownloadFile(filename)
			if err != nil {
				return nil, fmt.Errorf("下载 %s 失败: %w", name, err)
			}
		}
		profiles = append(profiles, &Profile{
			Name:     name,
			Settings: json.RawMessage(content),
		})
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

// FetchProfile 从 Gist 拉取单个 profile
func (g *GistClient) FetchProfile(name string) (*Profile, error) {
	filename := ProfileNameToGistFile(name)
	content, err := g.DownloadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("配置 %q 不存在或下载失败: %w", name, err)
	}
	return &Profile{
		Name:     name,
		Settings: json.RawMessage(content),
	}, nil
}

// DownloadFile 通过 raw URL 下载单个文件内容
func (g *GistClient) DownloadFile(filename string) ([]byte, error) {
	url := fmt.Sprintf("https://gitee.com/%s/codes/%s/raw?blob_name=%s",
		g.Owner, g.GistID, filename)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "token "+g.Token)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载文件 %s 失败: %w", filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("下载 %s 返回 %d: %s", filename, resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// UploadFile 上传或更新 Gist 中的文件
func (g *GistClient) UploadFile(filename string, content string) error {
	url := fmt.Sprintf("%s/gists/%s", giteeAPIBase, g.GistID)
	payload := map[string]interface{}{
		"access_token": g.Token,
		"files": map[string]interface{}{
			filename: map[string]string{
				"content": content,
			},
		},
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("上传文件 %s 失败: %w", filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("上传 %s 返回 %d: %s", filename, resp.StatusCode, string(respBody))
	}
	return nil
}

// DeleteFile 从 Gist 中删除文件
func (g *GistClient) DeleteFile(filename string) error {
	url := fmt.Sprintf("%s/gists/%s", giteeAPIBase, g.GistID)
	payload := map[string]interface{}{
		"access_token": g.Token,
		"files": map[string]interface{}{
			filename: nil,
		},
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("删除文件 %s 失败: %w", filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除 %s 返回 %d: %s", filename, resp.StatusCode, string(respBody))
	}
	return nil
}

// ProfileExists 检查 Gist 中是否存在指定 profile
func (g *GistClient) ProfileExists(name string) (bool, error) {
	files, err := g.ListSettingsFiles()
	if err != nil {
		return false, err
	}
	filename := ProfileNameToGistFile(name)
	_, exists := files[filename]
	return exists, nil
}

// GistFileToProfileName 从 Gist 文件名提取 profile 名
func GistFileToProfileName(filename string) string {
	name := strings.TrimPrefix(filename, "settings-")
	return strings.TrimSuffix(name, ".json")
}

// ProfileNameToGistFile 从 profile 名生成 Gist 文件名
func ProfileNameToGistFile(name string) string {
	return "settings-" + name + ".json"
}
