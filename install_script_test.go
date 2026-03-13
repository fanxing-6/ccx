package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptInstallsLatestAndSpecificVersion(t *testing.T) {
	t.Parallel()

	latestArchive := makeTestArchive(t, "0.0.1-test")
	specificArchive := makeTestArchive(t, "0.0.2-test")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest/download/ccx_linux_amd64.tar.gz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(latestArchive)
		case "/releases/download/v0.0.2-test/ccx_linux_amd64.tar.gz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(specificArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "install.sh")

	latestOutput := runInstallScript(t, scriptPath, installDir, server.URL, "")
	if !strings.Contains(latestOutput, "安装完成") {
		t.Fatalf("latest install output missing success message: %s", latestOutput)
	}
	assertInstalledVersion(t, installDir, "0.0.1-test")

	specificOutput := runInstallScript(t, scriptPath, installDir, server.URL, "0.0.2-test")
	if !strings.Contains(specificOutput, "0.0.2-test") {
		t.Fatalf("specific install output missing version: %s", specificOutput)
	}
	assertInstalledVersion(t, installDir, "0.0.2-test")
}

func TestInstallScriptSupportsDownloadURLAndPipeInvocation(t *testing.T) {
	t.Parallel()

	archive := makeTestArchive(t, "0.0.3-test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/direct/ccx.tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	repoRoot := mustGetwd(t)
	scriptPath := filepath.Join(repoRoot, "install.sh")
	bashPath := mustLookPath(t, "bash")

	cmd := exec.Command(bashPath, "-c", "cat \"$1\" | bash", "bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"CCX_INSTALL_DIR="+installDir,
		"CCX_DOWNLOAD_URL="+server.URL+"/direct/ccx.tar.gz",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pipe install failed: %v\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "安装完成") {
		t.Fatalf("pipe install output missing success message: %s", string(output))
	}
	assertInstalledVersion(t, installDir, "0.0.3-test")
}

func TestInstallScriptFailsOnUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	fakeBin := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "uname"), "#!/bin/sh\nif [ \"$1\" = \"-s\" ]; then\n  echo Darwin\nelse\n  echo x86_64\nfi\n")

	output, err := runInstallScriptExpectError(t, runInstallOptions{
		pathEnv: fakeBin,
	})
	if err == nil {
		t.Fatal("expected unsupported platform error")
	}
	if !strings.Contains(output, "当前仅支持 Linux") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestInstallScriptFailsWhenTarMissing(t *testing.T) {
	t.Parallel()

	fakeBin := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "uname"), "#!/bin/sh\nif [ \"$1\" = \"-s\" ]; then\n  echo Linux\nelse\n  echo x86_64\nfi\n")

	output, err := runInstallScriptExpectError(t, runInstallOptions{
		pathEnv: fakeBin,
	})
	if err == nil {
		t.Fatal("expected missing tar error")
	}
	if !strings.Contains(output, "缺少依赖命令: tar") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func makeTestArchive(t *testing.T, version string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	content := []byte(fmt.Sprintf("#!/bin/sh\necho ccx version %s\n", version))
	header := &tar.Header{
		Name: "ccx",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close failed: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}

	return buf.Bytes()
}

type runInstallOptions struct {
	installDir string
	serverURL  string
	version    string
	pathEnv    string
}

func runInstallScript(t *testing.T, scriptPath, installDir, serverURL, version string) string {
	t.Helper()

	output, err := runInstallScriptCommand(scriptPath, runInstallOptions{
		installDir: installDir,
		serverURL:  serverURL,
		version:    version,
	})
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}
	return output
}

func runInstallScriptExpectError(t *testing.T, opts runInstallOptions) (string, error) {
	t.Helper()

	repoRoot := mustGetwd(t)
	scriptPath := filepath.Join(repoRoot, "install.sh")
	return runInstallScriptCommand(scriptPath, opts)
}

func runInstallScriptCommand(scriptPath string, opts runInstallOptions) (string, error) {
	bashPath := mustLookPath(nil, "bash")
	cmd := exec.Command(bashPath, scriptPath)
	cmd.Env = buildInstallEnv(opts)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func buildInstallEnv(opts runInstallOptions) []string {
	env := append([]string{}, os.Environ()...)
	if opts.installDir != "" {
		env = append(env, "CCX_INSTALL_DIR="+opts.installDir)
	}
	if opts.serverURL != "" {
		env = append(env, "CCX_RELEASE_BASE_URL="+opts.serverURL+"/releases")
	}
	if opts.version != "" {
		env = append(env, "CCX_VERSION="+opts.version)
	}
	if opts.pathEnv != "" {
		env = append(withoutEnv(env, "PATH"), "PATH="+opts.pathEnv)
	}
	return env
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	return wd
}

func mustLookPath(t *testing.T, name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		if t != nil {
			t.Fatalf("LookPath(%s) failed: %v", name, err)
		}
		panic(err)
	}
	return path
}

func assertInstalledVersion(t *testing.T, installDir, version string) {
	t.Helper()

	binaryPath := filepath.Join(installDir, "ccx")
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
	output, err := exec.Command(binaryPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running installed binary failed: %v\n%s", err, string(output))
	}
	if !strings.Contains(string(output), version) {
		t.Fatalf("installed version mismatch: got %q want %q", string(output), version)
	}
}

func TestInstallScriptHandlesInvalidDownload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	repoRoot := mustGetwd(t)
	scriptPath := filepath.Join(repoRoot, "install.sh")
	output, err := runInstallScriptCommand(scriptPath, runInstallOptions{
		installDir: installDir,
		serverURL:  server.URL,
	})
	if err == nil {
		t.Fatal("expected download failure")
	}
	if !strings.Contains(output, "下载安装包") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "404") {
		t.Fatalf("expected 404 in output, got: %s", output)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "ccx")); !os.IsNotExist(statErr) {
		if statErr == nil {
			t.Fatal("binary should not exist after failed install")
		}
		t.Fatalf("unexpected stat error: %v", statErr)
	}
}

func TestInstallScriptLeavesNoTempDirLeakInOutput(t *testing.T) {
	t.Parallel()

	archive := makeTestArchive(t, "0.0.4-test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = io.Copy(w, bytes.NewReader(archive))
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	repoRoot := mustGetwd(t)
	scriptPath := filepath.Join(repoRoot, "install.sh")
	output, err := runInstallScriptCommand(scriptPath, runInstallOptions{
		installDir: installDir,
		serverURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}
	if strings.Contains(output, "unbound variable") {
		t.Fatalf("unexpected shell cleanup error: %s", output)
	}
}
