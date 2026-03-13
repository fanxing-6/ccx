package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
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

func runInstallScript(t *testing.T, scriptPath, installDir, serverURL, version string) string {
	t.Helper()

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"CCX_INSTALL_DIR="+installDir,
		"CCX_RELEASE_BASE_URL="+serverURL+"/releases",
	)
	if version != "" {
		cmd.Env = append(cmd.Env, "CCX_VERSION="+version)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, string(output))
	}
	return string(output)
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
