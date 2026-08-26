package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/semver"
)

func withUpdateGlobals(t *testing.T, version, repo, pending string) {
	t.Helper()
	oldVersion, oldRepo, oldPending := AppVersion, UpdateRepo, pendingUpdateURL
	AppVersion, UpdateRepo, pendingUpdateURL = version, repo, pending
	t.Cleanup(func() {
		AppVersion, UpdateRepo, pendingUpdateURL = oldVersion, oldRepo, oldPending
	})
}

func withUpdateReleases(t *testing.T, releases []GithubRelease) {
	t.Helper()
	oldFetch := fetchReleasesForUpdate
	fetchReleasesForUpdate = func(string) ([]GithubRelease, error) {
		return releases, nil
	}
	t.Cleanup(func() {
		fetchReleasesForUpdate = oldFetch
	})
}

func TestCheckUpdateWithoutForkSourceNeverUsesUpstream(t *testing.T) {
	withUpdateGlobals(t, "1.2.3", "", "https://example.invalid/stale.zip")

	info := (&App{}).CheckUpdate()
	if info.Error != unconfiguredUpdateSourceMessage {
		t.Fatalf("unexpected error: %q", info.Error)
	}
	if info.CurrentVer != "1.2.3" {
		t.Fatalf("current version = %q, want 1.2.3", info.CurrentVer)
	}
	if info.HasUpdate || pendingUpdateURL != "" {
		t.Fatalf("unconfigured source must not retain an update: info=%+v pending=%q", info, pendingUpdateURL)
	}
}

func TestConfiguredUpdateRepoRejectsUpstream(t *testing.T) {
	withUpdateGlobals(t, "1.2.3", UpstreamGithubRepo, "")

	_, err := configuredUpdateRepo()
	if err == nil {
		t.Fatal("expected upstream repository to be rejected")
	}
}

func TestDefaultUpdateRepoUsesCommunityFork(t *testing.T) {
	repo, err := configuredUpdateRepo()
	if err != nil {
		t.Fatalf("default update source should be configured: %v", err)
	}
	if repo != "zombrain69/lytvpk" {
		t.Fatalf("default update repo = %q", repo)
	}
}

func TestGetForkInfoUsesOnlyConfiguredFork(t *testing.T) {
	withUpdateGlobals(t, "1.2.3", "ExampleOwner/lytvpk-community", "")

	info := (&App{}).GetForkInfo()
	if !info.Configured {
		t.Fatalf("fork source unexpectedly unconfigured: %+v", info)
	}
	if info.UpdateRepo != "ExampleOwner/lytvpk-community" {
		t.Fatalf("update repo = %q", info.UpdateRepo)
	}
	if info.SourceURL != "https://github.com/ExampleOwner/lytvpk-community" {
		t.Fatalf("source URL = %q", info.SourceURL)
	}
	if info.UpstreamRepo != UpstreamGithubRepo {
		t.Fatalf("upstream repo = %q", info.UpstreamRepo)
	}
}

func TestSelectWindowsAMD64ZipAssetUsesReleaseAssetOnly(t *testing.T) {
	release := GithubRelease{Assets: []GithubReleaseAsset{
		{Name: "source-code.zip", BrowserDownloadURL: "https://example.invalid/source.zip"},
		{Name: "lytvpk-community_v1.2.3_windows_amd64.zip", BrowserDownloadURL: "https://example.invalid/windows.zip"},
		{Name: "lytvpk-community_v1.2.3_windows_arm64.zip", BrowserDownloadURL: "https://example.invalid/arm.zip"},
	}}

	if got := selectWindowsAMD64ZipAsset(release); got != "https://example.invalid/windows.zip" {
		t.Fatalf("asset URL = %q", got)
	}
}

func TestPublishedCommunityReleaseNamingIsUpdaterCompatible(t *testing.T) {
	version, err := semver.ParseTolerant("v2.5.14-community.1")
	if err != nil {
		t.Fatalf("release tag should parse: %v", err)
	}
	if version.String() != "2.5.14-community.1" {
		t.Fatalf("parsed version = %q", version.String())
	}

	release := GithubRelease{Assets: []GithubReleaseAsset{{
		Name:               "lytvpk_2.5.14-community.1_windows_amd64.zip",
		BrowserDownloadURL: "https://github.com/zombrain69/lytvpk/releases/download/v2.5.14-community.1/lytvpk_2.5.14-community.1_windows_amd64.zip",
	}}}
	if got := selectWindowsAMD64ZipAsset(release); got != release.Assets[0].BrowserDownloadURL {
		t.Fatalf("published asset was not selected: %q", got)
	}
}

func TestCheckUpdateReturnsCurrentReleaseNotesWithoutUpdate(t *testing.T) {
	withUpdateGlobals(t, "2.5.14-community.6", "zombrain69/lytvpk", "https://example.invalid/stale.zip")
	withUpdateReleases(t, []GithubRelease{
		{TagName: "v2.5.14-community.5", Body: "fix: older release"},
		{TagName: "v2.5.14-community.6", Body: "feat: current release notes"},
	})

	info := (&App{}).CheckUpdate()
	if info.Error != "" {
		t.Fatalf("unexpected error: %q", info.Error)
	}
	if info.HasUpdate {
		t.Fatalf("current release must not be reported as an update: %+v", info)
	}
	if info.CurrentVer != "2.5.14-community.6" || info.LatestVer != "2.5.14-community.6" {
		t.Fatalf("unexpected versions: %+v", info)
	}
	if !strings.Contains(info.ReleaseNote, "【v2.5.14-community.6】") || !strings.Contains(info.ReleaseNote, "current release notes") {
		t.Fatalf("current release note missing: %q", info.ReleaseNote)
	}
	if info.DownloadURL != "" || pendingUpdateURL != "" {
		t.Fatalf("read-only changelog must not retain an update URL: info=%+v pending=%q", info, pendingUpdateURL)
	}
}

func createUpdateZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(t.TempDir(), "update.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return zipPath
}

func TestInstallUpdateUsesCanonicalExecutable(t *testing.T) {
	dir := t.TempDir()
	currentExe := filepath.Join(dir, "legacy-lytvpk.exe")
	if err := os.WriteFile(currentExe, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	zipPath := createUpdateZip(t, map[string]string{
		"tools/other.exe":                        "wrong",
		"release/" + CommunityForkExecutableName: "new",
	})

	if err := installUpdate(zipPath, currentExe); err != nil {
		t.Fatalf("install update: %v", err)
	}
	got, err := os.ReadFile(currentExe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("updated executable = %q", got)
	}
}

func TestInstallUpdateRejectsZipWithoutCanonicalExecutable(t *testing.T) {
	dir := t.TempDir()
	currentExe := filepath.Join(dir, "lytvpk.exe")
	if err := os.WriteFile(currentExe, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	zipPath := createUpdateZip(t, map[string]string{"lytvpk.exe": "old-name"})

	if err := installUpdate(zipPath, currentExe); err == nil {
		t.Fatal("expected canonical executable validation failure")
	}
	got, err := os.ReadFile(currentExe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("current executable changed after rejected update: %q", got)
	}
}
