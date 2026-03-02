package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
)

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

func selfUpdate() error {
	// 获取当前版本
	currentVersion := strings.TrimPrefix(Version, "v")
	if currentVersion == "" || currentVersion == "dev" {
		fmt.Println("当前为开发版本，无法自动更新")
		fmt.Println("请手动重新安装: npm install -g claude-ccx")
		return nil
	}

	fmt.Printf("当前版本: v%s\n", currentVersion)
	fmt.Println("正在检查最新版本...")

	// 获取最新版本
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
	fmt.Printf("发布说明: %s\n", latest.Name)

	// 检查平台支持
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("自动更新仅支持 linux/amd64，当前平台: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// 确定更新方式
	if isInstalledViaNPM() {
		fmt.Println("\n通过 npm 更新:")
		fmt.Printf("  npm install -g claude-ccx@%s\n", latestVersion)
		fmt.Println("\n或者运行:")
		fmt.Println("  npm update -g claude-ccx")
		return nil
	}

	// 直接二进制更新
	return binarySelfUpdate(latest.TagName)
}

func fetchLatestRelease() (*releaseInfo, error) {
	resp, err := http.Get(githubAPIReleases)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API 返回 %d: %s", resp.StatusCode, string(body))
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &release, nil
}

func isInstalledViaNPM() bool {
	// 检查是否在 npm 全局目录中
	execPath, err := os.Executable()
	if err != nil {
		return false
	}

	// npm 全局路径通常包含 'lib/node_modules' 或 'node_modules'
	if strings.Contains(execPath, "node_modules") {
		return true
	}

	// 检查 npm 是否存在
	_, err = exec.LookPath("npm")
	if err != nil {
		return false
	}

	// 尝试通过 npm list 检查
	cmd := exec.Command("npm", "list", "-g", "claude-ccx")
	output, _ := cmd.Output()
	return strings.Contains(string(output), "claude-ccx")
}

func binarySelfUpdate(version string) error {
	// 获取当前二进制路径
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前路径失败: %w", err)
	}

	// 解析真实路径（处理软链）
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realPath = execPath
	}

	downloadURL := fmt.Sprintf("%s/%s/ccx_linux_amd64.tar.gz", githubReleasesURL, version)
	fmt.Printf("\n下载地址: %s\n", downloadURL)
	fmt.Printf("安装路径: %s\n", realPath)

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "ccx-update-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpTar := filepath.Join(tmpDir, "ccx.tar.gz")

	// 下载
	fmt.Println("正在下载...")
	if err := downloadFile(downloadURL, tmpTar); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	// 解压
	fmt.Println("正在解压...")
	cmd := exec.Command("tar", "-xzf", tmpTar, "-C", tmpDir, "ccx")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	// 验证新版本
	newBinary := filepath.Join(tmpDir, "ccx")
	verifyCmd := exec.Command(newBinary, "--version")
	output, err := verifyCmd.Output()
	if err != nil {
		return fmt.Errorf("验证新版本失败: %w", err)
	}
	fmt.Printf("验证成功: %s", output)

	// 替换旧版本
	fmt.Println("正在更新...")

	// 备份旧版本
	backupPath := realPath + ".backup"
	if err := os.Rename(realPath, backupPath); err != nil {
		return fmt.Errorf("备份旧版本失败: %w", err)
	}

	// 移动新版本
	if err := os.Rename(newBinary, realPath); err != nil {
		// 恢复备份
		os.Rename(backupPath, realPath)
		return fmt.Errorf("更新失败: %w", err)
	}

	// 设置权限
	os.Chmod(realPath, 0755)

	// 删除备份
	os.Remove(backupPath)

	fmt.Printf("\n✓ 更新成功: v%s\n", strings.TrimPrefix(version, "v"))
	return nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
