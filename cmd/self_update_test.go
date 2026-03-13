package cmd

import (
	"errors"
	"reflect"
	"testing"
)

func TestAutoUpdateBeforeStartupSkipsForDevVersion(t *testing.T) {
	originalVersion := Version
	originalFetch := fetchLatestReleaseFunc
	defer func() {
		Version = originalVersion
		fetchLatestReleaseFunc = originalFetch
	}()

	Version = "dev"
	fetchCalled := false
	fetchLatestReleaseFunc = func() (*releaseInfo, error) {
		fetchCalled = true
		return nil, nil
	}

	if err := autoUpdateBeforeStartup([]string{"list"}); err != nil {
		t.Fatalf("autoUpdateBeforeStartup returned error: %v", err)
	}
	if fetchCalled {
		t.Fatal("expected dev version to skip auto update check")
	}
}

func TestAutoUpdateBeforeStartupSkipsWhenAlreadyLatest(t *testing.T) {
	originalVersion := Version
	originalFetch := fetchLatestReleaseFunc
	originalBinary := binarySelfUpdateFunc
	originalReexec := reexecCurrentProcessFunc
	defer func() {
		Version = originalVersion
		fetchLatestReleaseFunc = originalFetch
		binarySelfUpdateFunc = originalBinary
		reexecCurrentProcessFunc = originalReexec
	}()

	Version = "0.1.0"
	binaryCalled := false
	reexecCalled := false
	fetchLatestReleaseFunc = func() (*releaseInfo, error) {
		return &releaseInfo{TagName: "v0.1.0", Name: "same version"}, nil
	}
	binarySelfUpdateFunc = func(version string) error {
		binaryCalled = true
		return nil
	}
	reexecCurrentProcessFunc = func(args []string) error {
		reexecCalled = true
		return nil
	}

	if err := autoUpdateBeforeStartup([]string{"list"}); err != nil {
		t.Fatalf("autoUpdateBeforeStartup returned error: %v", err)
	}
	if binaryCalled {
		t.Fatal("did not expect binary update when already latest")
	}
	if reexecCalled {
		t.Fatal("did not expect reexec when already latest")
	}
}

func TestAutoUpdateBeforeStartupUpdatesAndReexecs(t *testing.T) {
	originalVersion := Version
	originalFetch := fetchLatestReleaseFunc
	originalBinary := binarySelfUpdateFunc
	originalReexec := reexecCurrentProcessFunc
	defer func() {
		Version = originalVersion
		fetchLatestReleaseFunc = originalFetch
		binarySelfUpdateFunc = originalBinary
		reexecCurrentProcessFunc = originalReexec
	}()

	Version = "0.1.0"
	var updatedVersion string
	var gotArgs []string
	fetchLatestReleaseFunc = func() (*releaseInfo, error) {
		return &releaseInfo{TagName: "v0.1.1", Name: "new release"}, nil
	}
	binarySelfUpdateFunc = func(version string) error {
		updatedVersion = version
		return nil
	}
	reexecCurrentProcessFunc = func(args []string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}

	args := []string{"auth", "status"}
	if err := autoUpdateBeforeStartup(args); err != nil {
		t.Fatalf("autoUpdateBeforeStartup returned error: %v", err)
	}
	if updatedVersion != "v0.1.1" {
		t.Fatalf("updated version=%q, want %q", updatedVersion, "v0.1.1")
	}
	if !reflect.DeepEqual(gotArgs, args) {
		t.Fatalf("reexec args=%v, want %v", gotArgs, args)
	}
}

func TestAutoUpdateBeforeStartupContinuesOnCheckFailure(t *testing.T) {
	originalVersion := Version
	originalFetch := fetchLatestReleaseFunc
	originalBinary := binarySelfUpdateFunc
	originalReexec := reexecCurrentProcessFunc
	defer func() {
		Version = originalVersion
		fetchLatestReleaseFunc = originalFetch
		binarySelfUpdateFunc = originalBinary
		reexecCurrentProcessFunc = originalReexec
	}()

	Version = "0.1.0"
	binaryCalled := false
	reexecCalled := false
	fetchLatestReleaseFunc = func() (*releaseInfo, error) {
		return nil, errors.New("network down")
	}
	binarySelfUpdateFunc = func(version string) error {
		binaryCalled = true
		return nil
	}
	reexecCurrentProcessFunc = func(args []string) error {
		reexecCalled = true
		return nil
	}

	if err := autoUpdateBeforeStartup([]string{"list"}); err != nil {
		t.Fatalf("autoUpdateBeforeStartup returned error: %v", err)
	}
	if binaryCalled {
		t.Fatal("did not expect update after check failure")
	}
	if reexecCalled {
		t.Fatal("did not expect reexec after check failure")
	}
}

func TestAutoUpdateBeforeStartupContinuesOnUpdateFailure(t *testing.T) {
	originalVersion := Version
	originalFetch := fetchLatestReleaseFunc
	originalBinary := binarySelfUpdateFunc
	originalReexec := reexecCurrentProcessFunc
	defer func() {
		Version = originalVersion
		fetchLatestReleaseFunc = originalFetch
		binarySelfUpdateFunc = originalBinary
		reexecCurrentProcessFunc = originalReexec
	}()

	Version = "0.1.0"
	reexecCalled := false
	fetchLatestReleaseFunc = func() (*releaseInfo, error) {
		return &releaseInfo{TagName: "v0.1.1", Name: "new release"}, nil
	}
	binarySelfUpdateFunc = func(version string) error {
		return errors.New("permission denied")
	}
	reexecCurrentProcessFunc = func(args []string) error {
		reexecCalled = true
		return nil
	}

	if err := autoUpdateBeforeStartup([]string{"list"}); err != nil {
		t.Fatalf("autoUpdateBeforeStartup returned error: %v", err)
	}
	if reexecCalled {
		t.Fatal("did not expect reexec after update failure")
	}
}
