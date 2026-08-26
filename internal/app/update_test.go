package app

import "testing"

func withUpdateGlobals(t *testing.T, version, repo, pending string) {
	t.Helper()
	oldVersion, oldRepo, oldPending := AppVersion, UpdateRepo, pendingUpdateURL
	AppVersion, UpdateRepo, pendingUpdateURL = version, repo, pending
	t.Cleanup(func() {
		AppVersion, UpdateRepo, pendingUpdateURL = oldVersion, oldRepo, oldPending
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
