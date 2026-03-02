package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestParseChecksumForFile(t *testing.T) {
	content := `abc123def456  ccx_linux_amd64.tar.gz
789abcdef012  ccx_darwin_arm64.tar.gz
`
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "existing linux asset", filename: "ccx_linux_amd64.tar.gz", want: "abc123def456"},
		{name: "existing darwin asset", filename: "ccx_darwin_arm64.tar.gz", want: "789abcdef012"},
		{name: "non-existing asset", filename: "ccx_windows_amd64.zip", want: ""},
		{name: "empty content", filename: "ccx_linux_amd64.tar.gz", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := content
			if tc.name == "empty content" {
				input = ""
			}
			got := parseChecksumForFile(input, tc.filename)
			if got != tc.want {
				t.Fatalf("parseChecksumForFile(%q)=%q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	data := []byte("hello world\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}

	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])
	if got != want {
		t.Fatalf("sha256File=%q, want %q", got, want)
	}
}

func TestSha256FileNotExist(t *testing.T) {
	_, err := sha256File("/nonexistent/path/file")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestVerifyChecksumSuccess(t *testing.T) {
	dir := t.TempDir()

	// 创建目标文件
	targetData := []byte("binary content here")
	targetPath := filepath.Join(dir, "ccx_linux_amd64.tar.gz")
	if err := os.WriteFile(targetPath, targetData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// 计算真实 hash
	h := sha256.Sum256(targetData)
	realHash := hex.EncodeToString(h[:])

	// 创建 checksums.txt
	checksumContent := realHash + "  ccx_linux_amd64.tar.gz\n"
	checksumPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumPath, []byte(checksumContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := verifyChecksum(targetPath, checksumPath, "ccx_linux_amd64.tar.gz"); err != nil {
		t.Fatalf("verifyChecksum should succeed: %v", err)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	dir := t.TempDir()

	targetPath := filepath.Join(dir, "ccx_linux_amd64.tar.gz")
	if err := os.WriteFile(targetPath, []byte("actual content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	checksumContent := "0000000000000000000000000000000000000000000000000000000000000000  ccx_linux_amd64.tar.gz\n"
	checksumPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumPath, []byte(checksumContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := verifyChecksum(targetPath, checksumPath, "ccx_linux_amd64.tar.gz")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestVerifyChecksumMissingEntry(t *testing.T) {
	dir := t.TempDir()

	targetPath := filepath.Join(dir, "ccx_linux_amd64.tar.gz")
	if err := os.WriteFile(targetPath, []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	checksumContent := "abcdef  some_other_file.tar.gz\n"
	checksumPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumPath, []byte(checksumContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := verifyChecksum(targetPath, checksumPath, "ccx_linux_amd64.tar.gz")
	if err == nil {
		t.Fatal("expected missing entry error")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	srcData := []byte("some binary data")
	if err := os.WriteFile(src, srcData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := copyFile(src, dst, 0755); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	dstData, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile dst: %v", err)
	}
	if string(dstData) != string(srcData) {
		t.Fatalf("copied content mismatch")
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat dst: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("perm=%o, want 0755", info.Mode().Perm())
	}
}

func TestCopyFileSrcNotExist(t *testing.T) {
	dir := t.TempDir()
	err := copyFile("/nonexistent", filepath.Join(dir, "dst"), 0755)
	if err == nil {
		t.Fatal("expected error for non-existent src")
	}
}

func TestAssetName(t *testing.T) {
	got := assetName()
	// 在测试环境中应该是 linux_amd64
	want := "ccx_linux_amd64.tar.gz"
	if got != want {
		t.Fatalf("assetName()=%q, want %q", got, want)
	}
}
