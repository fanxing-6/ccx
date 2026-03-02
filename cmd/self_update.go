package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
}

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "检查并更新 ccx 到最新版本",
	RunE: func(cmd *cobra.Command, args []string) error {
		return selfUpdate()
	},
}

const (
	githubAPIReleases = "https://api.github.com/repos/fanxing-6/ccx/releases/latest"
	githubReleasesURL = "https://github.com/fanxing-6/ccx/releases/download"
	updateHTTPTimeout = 60 * time.Second
	// 下载文件最大 100MB，防止异常响应耗尽内存
	maxDownloadBytes = 100 << 20
)

// updateHTTPClient 是 self-update 专用的带超时 HTTP 客户端
var updateHTTPClient = &http.Client{Timeout: updateHTTPTimeout}

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

func selfUpdate() error {
	currentVersion := strings.TrimPrefix(Version, "v")
	if currentVersion == "" || currentVersion == "dev" {
		return fmt.Errorf("当前为开发版本（%s），无法自动更新；请手动安装: npm install -g claude-ccx", Version)
	}

	fmt.Printf("当前版本: v%s\n", currentVersion)
	fmt.Println("正在检查最新版本...")

	latest, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("获取最新版本失败: %w", err)
	}

	latestVersion := strings.TrimPrefix(latest.TagName, "v")
	if latestVersion == currentVersion {
		fmt.Println("已是最新版本")
		return nil
	}

	fmt.Printf("发现新版本: v%s\n", latestVersion)
	if latest.Name != "" {
		fmt.Printf("发布说明: %s\n", latest.Name)
	}

	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("自动更新仅支持 linux/amd64，当前平台: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	if isInstalledViaNPM() {
		fmt.Println("\n检测到 npm 安装方式，请通过 npm 更新:")
		fmt.Printf("  npm install -g claude-ccx@%s\n", latestVersion)
		fmt.Println("  npm update -g claude-ccx")
		return nil
	}

	return binarySelfUpdate(latest.TagName)
}

func fetchLatestRelease() (*releaseInfo, error) {
	resp, err := updateHTTPClient.Get(githubAPIReleases)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub API 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub API 响应失败: %w", err)
	}
	if release.TagName == "" {
		return nil, fmt.Errorf("GitHub API 返回的 tag_name 为空")
	}
	return &release, nil
}

func isInstalledViaNPM() bool {
	execPath, err := os.Executable()
	if err != nil {
		return false
	}
	if strings.Contains(execPath, "node_modules") {
		return true
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return false
	}
	output, err := exec.Command("npm", "list", "-g", "claude-ccx").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "claude-ccx")
}

// assetName 根据 GOOS/GOARCH 计算发布资产文件名
func assetName() string {
	return fmt.Sprintf("ccx_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
}

func binarySelfUpdate(tag string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前二进制路径失败: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("解析二进制真实路径失败: %w", err)
	}

	asset := assetName()
	archiveURL := fmt.Sprintf("%s/%s/%s", githubReleasesURL, tag, asset)
	checksumURL := fmt.Sprintf("%s/%s/checksums.txt", githubReleasesURL, tag)

	fmt.Printf("\n下载地址: %s\n", archiveURL)
	fmt.Printf("安装路径: %s\n", realPath)

	tmpDir, err := os.MkdirTemp("", "ccx-update-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. 下载 checksums.txt
	fmt.Println("正在下载校验文件...")
	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(checksumURL, checksumPath); err != nil {
		return fmt.Errorf("下载 checksums.txt 失败: %w", err)
	}

	// 2. 下载压缩包
	fmt.Println("正在下载更新包...")
	archivePath := filepath.Join(tmpDir, asset)
	if err := downloadFile(archiveURL, archivePath); err != nil {
		return fmt.Errorf("下载更新包失败: %w", err)
	}

	// 3. 校验 SHA256
	fmt.Println("正在校验完整性...")
	if err := verifyChecksum(archivePath, checksumPath, asset); err != nil {
		return fmt.Errorf("完整性校验失败: %w", err)
	}

	// 4. 解压
	fmt.Println("正在解压...")
	extractCmd := exec.Command("tar", "-xzf", archivePath, "-C", tmpDir, "ccx")
	if output, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("解压失败: %w\n%s", err, strings.TrimSpace(string(output)))
	}

	// 5. 验证新版本可执行
	newBinary := filepath.Join(tmpDir, "ccx")
	verifyCmd := exec.Command(newBinary, "--version")
	verifyOutput, err := verifyCmd.Output()
	if err != nil {
		return fmt.Errorf("验证新二进制失败: %w", err)
	}
	fmt.Printf("验证成功: %s", verifyOutput)

	// 6. 原子替换：backup → move new → rollback on failure
	fmt.Println("正在更新...")
	backupPath := realPath + ".backup"
	if err := os.Rename(realPath, backupPath); err != nil {
		return fmt.Errorf("备份旧版本失败（%s → %s）: %w", realPath, backupPath, err)
	}

	if err := copyFile(newBinary, realPath, 0755); err != nil {
		// 回滚
		if rbErr := os.Rename(backupPath, realPath); rbErr != nil {
			return fmt.Errorf("更新失败且回滚也失败: 更新错误=%v, 回滚错误=%v（备份文件在 %s）", err, rbErr, backupPath)
		}
		return fmt.Errorf("更新失败（已回滚）: %w", err)
	}

	if err := os.Remove(backupPath); err != nil {
		fmt.Printf("警告: 清理备份文件失败（%s）: %v\n", backupPath, err)
	}

	fmt.Printf("\n更新成功: v%s\n", strings.TrimPrefix(tag, "v"))
	return nil
}

// verifyChecksum 从 checksums.txt 中查找目标文件的 SHA256 并与本地文件比对
func verifyChecksum(filePath, checksumFile, targetName string) error {
	data, err := os.ReadFile(checksumFile)
	if err != nil {
		return fmt.Errorf("读取 checksums.txt 失败: %w", err)
	}

	expectedHash := parseChecksumForFile(string(data), targetName)
	if expectedHash == "" {
		return fmt.Errorf("checksums.txt 中未找到 %s 的校验值", targetName)
	}

	actualHash, err := sha256File(filePath)
	if err != nil {
		return fmt.Errorf("计算文件 SHA256 失败: %w", err)
	}

	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("SHA256 不匹配: 期望 %s, 实际 %s", expectedHash, actualHash)
	}
	return nil
}

// parseChecksumForFile 解析 goreleaser 格式的 checksums.txt（每行: <hash>  <filename>）
func parseChecksumForFile(content, filename string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// goreleaser 格式: "<sha256>  <filename>" （两个空格分隔）
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			return parts[0]
		}
	}
	return ""
}

// sha256File 计算文件的 SHA256 哈希
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile 通过读写复制文件（避免跨文件系统 os.Rename 失败）
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

func downloadFile(url, dest string) error {
	resp, err := updateHTTPClient.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}

	// 限制最大下载体积
	if _, err := io.Copy(out, io.LimitReader(resp.Body, maxDownloadBytes)); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("下载写入失败: %w", err)
	}
	return out.Close()
}
