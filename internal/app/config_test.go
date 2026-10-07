package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"vpk-manager/internal/network"
)

func newConfigTestApp(t *testing.T) *App {
	t.Helper()
	return configTestAppAtDir(t.TempDir())
}

// configTestAppAtDir 用指定配置目录建一个测试用 App：排序设置的持久化用例需要
// "第二次启动"读同一个 config.json，所以不能每次都新建临时目录。
func configTestAppAtDir(dir string) *App {
	return &App{
		configDir:                       dir,
		configPath:                      filepath.Join(dir, "config.json"),
		serversPath:                     filepath.Join(dir, "servers.json"),
		workshopWatchLaterPath:          filepath.Join(dir, "workshop_watch_later.json"),
		workshopPreferredIP:             true,
		workshopMetaEnabled:             true,
		workshopBrowserTarget:           "mirror",
		workshopTranslateProvider:       workshopTranslateProviderMicrosoft,
		displayMode:                     "list",
		sortType:                        fileSortTypeLoadOrder,
		sortOrder:                       fileSortOrderAsc,
		filterLayoutMode:                "compact",
		boxSelectionEnabled:             true,
		ctrlClickSelectionEnabled:       true,
		uiScale:                         defaultUIScale,
		unrecordedModLoadOrderPlacement: addonListUnrecordedPlacementEnd,
		savedDirectories:                []SavedDirectory{},
	}
}

func TestConfigDefaultsWithoutFile(t *testing.T) {
	app := newConfigTestApp(t)
	app.loadConfig()

	config := app.GetAppConfig()
	if config.WorkshopPreferredIP == nil || !*config.WorkshopPreferredIP {
		t.Fatalf("expected workshop preferred IP to default to true")
	}
	if config.WorkshopBrowserTarget == nil || *config.WorkshopBrowserTarget != "mirror" {
		t.Fatalf("expected browser target mirror, got %#v", config.WorkshopBrowserTarget)
	}
	if config.WorkshopTranslateProvider == nil || *config.WorkshopTranslateProvider != workshopTranslateProviderMicrosoft {
		t.Fatalf("expected translate provider microsoft, got %#v", config.WorkshopTranslateProvider)
	}
	if config.WorkshopMetaEnabled == nil || !*config.WorkshopMetaEnabled {
		t.Fatalf("expected workshop meta storage to default to true")
	}
	if config.DisplayMode != "list" {
		t.Fatalf("expected display mode list, got %q", config.DisplayMode)
	}
	if config.FilterLayoutMode != "compact" {
		t.Fatalf("expected filter layout compact, got %q", config.FilterLayoutMode)
	}
	if config.MigrationVersion != 0 {
		t.Fatalf("expected migration version 0, got %d", config.MigrationVersion)
	}
	if len(config.SavedDirectories) != 0 {
		t.Fatalf("expected no saved directories, got %d", len(config.SavedDirectories))
	}
	if config.BoxSelectionEnabled == nil || !*config.BoxSelectionEnabled {
		t.Fatalf("expected box selection to default to true")
	}
	if config.CtrlClickSelectionEnabled == nil || !*config.CtrlClickSelectionEnabled {
		t.Fatalf("expected ctrl click selection to default to true")
	}
	if config.UIScale != defaultUIScale {
		t.Fatalf("expected UI scale to default to %v, got %v", defaultUIScale, config.UIScale)
	}
	if config.AddonListGuardEnabled == nil || *config.AddonListGuardEnabled {
		t.Fatalf("expected addonlist guard to default to false, got %#v", config.AddonListGuardEnabled)
	}
	if config.UnrecordedModLoadOrderPlacement == nil || *config.UnrecordedModLoadOrderPlacement != addonListUnrecordedPlacementEnd {
		t.Fatalf("expected unrecorded Mod placement to default to end, got %#v", config.UnrecordedModLoadOrderPlacement)
	}
}

// 排序设置要能写进 config.json 并在重启后恢复（对齐上游 7b0818c）。
func TestFileSortPreferenceRoundTrip(t *testing.T) {
	first := newConfigTestApp(t)
	first.loadConfig()

	config := first.GetAppConfig()
	config.SortType = fileSortTypeModelComplexity
	config.SortOrder = fileSortOrderAsc
	if err := first.SaveAppConfig(config); err != nil {
		t.Fatalf("保存排序设置失败: %v", err)
	}

	// 模拟重启：同一个配置目录再建一个 App 读盘。
	second := configTestAppAtDir(first.configDir)
	second.loadConfig()
	got := second.GetAppConfig()
	if got.SortType != fileSortTypeModelComplexity || got.SortOrder != fileSortOrderAsc {
		t.Fatalf("重启后应恢复排序设置，实际 %q/%q", got.SortType, got.SortOrder)
	}
}

// 非法 / 缺失的排序值必须回落成合法值：旧配置没有这两个字段，手改配置也可能写错。
func TestNormalizeFileSort(t *testing.T) {
	cases := []struct {
		inType, inOrder   string
		wantType, wantOrd string
	}{
		{"", "", fileSortTypeLoadOrder, fileSortOrderAsc},
		{"bogus", "desc", fileSortTypeLoadOrder, fileSortOrderDesc},
		{fileSortTypeDate, "", fileSortTypeDate, fileSortOrderDesc},
		{fileSortTypeName, "", fileSortTypeName, fileSortOrderAsc},
		{fileSortTypeLoadOrder, "DESC", fileSortTypeLoadOrder, fileSortOrderDesc},
		{fileSortTypeSize, "asc", fileSortTypeSize, fileSortOrderAsc},
		{fileSortTypeModelComplexity, " x ", fileSortTypeModelComplexity, fileSortOrderDesc},
	}
	for _, c := range cases {
		gotType, gotOrder := normalizeFileSort(c.inType, c.inOrder)
		if gotType != c.wantType || gotOrder != c.wantOrd {
			t.Fatalf("normalizeFileSort(%q,%q) = %q/%q，期望 %q/%q",
				c.inType, c.inOrder, gotType, gotOrder, c.wantType, c.wantOrd)
		}
	}
}

func TestUIScaleConfigurationRoundTrip(t *testing.T) {
	app := newConfigTestApp(t)
	app.uiScale = 1.25
	if err := app.writeConfigFile(app.snapshotConfig()); err != nil {
		t.Fatalf("write config: %v", err)
	}

	restored := newConfigTestApp(t)
	restored.configDir = app.configDir
	restored.configPath = app.configPath
	restored.serversPath = app.serversPath
	restored.workshopWatchLaterPath = app.workshopWatchLaterPath
	restored.loadConfig()
	if config := restored.GetAppConfig(); config.UIScale != 1.25 {
		t.Fatalf("expected UI scale to round trip, got %v", config.UIScale)
	}

	if err := restored.SaveAppConfig(ConfigFile{UIScale: 3}); err != nil {
		t.Fatalf("save invalid UI scale: %v", err)
	}
	if config := restored.GetAppConfig(); config.UIScale != defaultUIScale {
		t.Fatalf("expected invalid UI scale to reset to %v, got %v", defaultUIScale, config.UIScale)
	}
}

func TestSaveAppConfigPersistsSettingsAndPreservesSecret(t *testing.T) {
	t.Cleanup(func() { network.GlobalIPSelector.SetFixedIP("") })
	app := newConfigTestApp(t)
	app.workshopTranslateCustomAPIKey = "encrypted-existing-key"
	app.modRotationConfig = RotationConfig{EnableCharacters: true}

	preferredIP := false
	fixedIP := "23.55.51.221"
	metaEnabled := false
	updateCheckEnabled := true
	browserTarget := "steam"
	provider := workshopTranslateProviderYandex
	guardEnabled := true
	placement := addonListUnrecordedPlacementStart
	if err := app.SaveAppConfig(ConfigFile{
		ModRotationConfig:               RotationConfig{EnableCharacters: false, EnableWeapons: true},
		WorkshopPreferredIP:             &preferredIP,
		WorkshopFixedIP:                 &fixedIP,
		WorkshopMetaEnabled:             &metaEnabled,
		WorkshopUpdateCheckEnabled:      &updateCheckEnabled,
		WorkshopBrowserTarget:           &browserTarget,
		WorkshopTranslateProvider:       &provider,
		WorkshopTranslateCustomBaseURL:  "https://translate.example/v1",
		WorkshopTranslateCustomModelId:  "model-a",
		AddonListGuardEnabled:           &guardEnabled,
		UnrecordedModLoadOrderPlacement: &placement,
		DisplayMode:                     "card",
		FilterLayoutMode:                "classic",
		UIScale:                         1.2,
	}); err != nil {
		t.Fatalf("save complete app config: %v", err)
	}

	config := app.GetAppConfig()
	if config.WorkshopPreferredIP == nil || *config.WorkshopPreferredIP {
		t.Fatalf("preferred IP = %#v, want false", config.WorkshopPreferredIP)
	}
	if config.WorkshopFixedIP == nil || *config.WorkshopFixedIP != fixedIP {
		t.Fatalf("fixed IP = %#v, want %q", config.WorkshopFixedIP, fixedIP)
	}
	if config.WorkshopMetaEnabled == nil || *config.WorkshopMetaEnabled {
		t.Fatalf("meta enabled = %#v, want false", config.WorkshopMetaEnabled)
	}
	if config.WorkshopUpdateCheckEnabled == nil || !*config.WorkshopUpdateCheckEnabled {
		t.Fatalf("update check enabled = %#v, want true", config.WorkshopUpdateCheckEnabled)
	}
	if config.WorkshopBrowserTarget == nil || *config.WorkshopBrowserTarget != browserTarget {
		t.Fatalf("browser target = %#v, want %q", config.WorkshopBrowserTarget, browserTarget)
	}
	if config.WorkshopTranslateProvider == nil || *config.WorkshopTranslateProvider != provider {
		t.Fatalf("translate provider = %#v, want %q", config.WorkshopTranslateProvider, provider)
	}
	if config.ModRotationConfig != (RotationConfig{EnableWeapons: true}) {
		t.Fatalf("rotation config = %#v", config.ModRotationConfig)
	}
	if config.AddonListGuardEnabled == nil || !*config.AddonListGuardEnabled {
		t.Fatalf("addonlist guard = %#v, want true", config.AddonListGuardEnabled)
	}
	if config.UnrecordedModLoadOrderPlacement == nil || *config.UnrecordedModLoadOrderPlacement != placement {
		t.Fatalf("unrecorded Mod placement = %#v, want %q", config.UnrecordedModLoadOrderPlacement, placement)
	}
	if config.WorkshopTranslateCustomAPIKey != "encrypted-existing-key" {
		t.Fatalf("custom API key changed unexpectedly: %q", config.WorkshopTranslateCustomAPIKey)
	}

	partial := ConfigFile{UIScale: 1.3}
	if err := app.SaveAppConfig(partial); err != nil {
		t.Fatalf("save partial app config: %v", err)
	}
	config = app.GetAppConfig()
	if config.WorkshopPreferredIP == nil || *config.WorkshopPreferredIP {
		t.Fatalf("partial save lost preferred IP: %#v", config.WorkshopPreferredIP)
	}
	if config.WorkshopBrowserTarget == nil || *config.WorkshopBrowserTarget != browserTarget {
		t.Fatalf("partial save lost browser target: %#v", config.WorkshopBrowserTarget)
	}
	if config.AddonListGuardEnabled == nil || !*config.AddonListGuardEnabled {
		t.Fatalf("partial save lost addonlist guard: %#v", config.AddonListGuardEnabled)
	}
	if config.UnrecordedModLoadOrderPlacement == nil || *config.UnrecordedModLoadOrderPlacement != placement {
		t.Fatalf("partial save lost unrecorded Mod placement: %#v", config.UnrecordedModLoadOrderPlacement)
	}
}

func TestUnrecordedModLoadOrderPlacementConfigurationRoundTrip(t *testing.T) {
	app := newConfigTestApp(t)
	app.unrecordedModLoadOrderPlacement = addonListUnrecordedPlacementAfterEnabled
	if err := app.writeConfigFile(app.snapshotConfig()); err != nil {
		t.Fatalf("write config: %v", err)
	}

	restored := newConfigTestApp(t)
	restored.configDir = app.configDir
	restored.configPath = app.configPath
	restored.serversPath = app.serversPath
	restored.workshopWatchLaterPath = app.workshopWatchLaterPath
	restored.loadConfig()
	if config := restored.GetAppConfig(); config.UnrecordedModLoadOrderPlacement == nil || *config.UnrecordedModLoadOrderPlacement != addonListUnrecordedPlacementAfterEnabled {
		t.Fatalf("unrecorded Mod placement did not round trip: %#v", config.UnrecordedModLoadOrderPlacement)
	}

	invalid := "unsupported"
	if err := restored.SaveAppConfig(ConfigFile{UnrecordedModLoadOrderPlacement: &invalid}); err != nil {
		t.Fatalf("save invalid placement: %v", err)
	}
	if config := restored.GetAppConfig(); config.UnrecordedModLoadOrderPlacement == nil || *config.UnrecordedModLoadOrderPlacement != addonListUnrecordedPlacementEnd {
		t.Fatalf("invalid placement should normalize to end: %#v", config.UnrecordedModLoadOrderPlacement)
	}
}

func TestAddonListGuardConfigurationRoundTrip(t *testing.T) {
	app := newConfigTestApp(t)
	app.addonListGuardEnabled = true
	if err := app.writeConfigFile(app.snapshotConfig()); err != nil {
		t.Fatalf("write config: %v", err)
	}

	restored := newConfigTestApp(t)
	restored.configDir = app.configDir
	restored.configPath = app.configPath
	restored.serversPath = app.serversPath
	restored.workshopWatchLaterPath = app.workshopWatchLaterPath
	restored.loadConfig()

	config := restored.GetAppConfig()
	if config.AddonListGuardEnabled == nil || !*config.AddonListGuardEnabled {
		t.Fatalf("expected addonlist guard to round trip, got %#v", config.AddonListGuardEnabled)
	}
}

func TestAddonListGuardExplicitSelectionPersistsAndCanBeDisabled(t *testing.T) {
	app := newConfigTestApp(t)
	addons := filepath.Join(t.TempDir(), "left4dead2", "addons")
	if err := os.MkdirAll(addons, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(addons), "addonlist.txt"), []byte("\"AddonList\"\n{\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app.rootDir = addons
	t.Cleanup(app.stopAddonListMonitor)

	if _, err := app.SetAddonListGuardEnabled(true); err != nil {
		t.Fatalf("explicitly enable guard: %v", err)
	}
	if app.addonListMonitorStop == nil {
		t.Fatal("explicitly enabled guard did not start the monitor")
	}

	restored := newConfigTestApp(t)
	restored.configDir = app.configDir
	restored.configPath = app.configPath
	restored.serversPath = app.serversPath
	restored.workshopWatchLaterPath = app.workshopWatchLaterPath
	restored.loadConfig()
	if config := restored.GetAppConfig(); config.AddonListGuardEnabled == nil || !*config.AddonListGuardEnabled {
		t.Fatalf("explicitly enabled guard was not persisted: %#v", config.AddonListGuardEnabled)
	}

	if _, err := app.SetAddonListGuardEnabled(false); err != nil {
		t.Fatalf("explicitly disable guard: %v", err)
	}
	if app.addonListMonitorStop != nil {
		t.Fatal("explicitly disabled guard left the monitor running")
	}

	restored.loadConfig()
	if config := restored.GetAppConfig(); config.AddonListGuardEnabled == nil || *config.AddonListGuardEnabled {
		t.Fatalf("explicitly disabled guard was not persisted: %#v", config.AddonListGuardEnabled)
	}
}

func TestMigrateLocalStorageConfigCreatesMissingTargets(t *testing.T) {
	app := newConfigTestApp(t)

	err := app.MigrateLocalStorageConfig(legacyMigrationPayload())
	if err != nil {
		t.Fatalf("migrate local storage config: %v", err)
	}

	config := app.GetAppConfig()
	if config.MigrationVersion != configMigrationVersion {
		t.Fatalf("expected migration version %d, got %d", configMigrationVersion, config.MigrationVersion)
	}
	if config.WorkshopPreferredIP == nil || *config.WorkshopPreferredIP {
		t.Fatalf("expected migrated workshop preferred IP false")
	}
	if config.DisplayMode != "card" || config.FilterLayoutMode != "classic" {
		t.Fatalf("expected migrated display/filter modes, got %q/%q", config.DisplayMode, config.FilterLayoutMode)
	}
	if config.BoxSelectionEnabled == nil || !*config.BoxSelectionEnabled || config.CtrlClickSelectionEnabled == nil || !*config.CtrlClickSelectionEnabled {
		t.Fatalf("expected migrated selection settings enabled")
	}
	if config.Theme != "dark" || config.IgnoredVersion != "1.2.3" || config.LastUpdateCheckTime != "1779169113000" {
		t.Fatalf("expected migrated theme/update fields, got theme=%q ignored=%q last=%q", config.Theme, config.IgnoredVersion, config.LastUpdateCheckTime)
	}
	if !config.ModRotationConfig.EnableCharacters || config.ModRotationConfig.EnableWeapons {
		t.Fatalf("expected migrated rotation config, got %#v", config.ModRotationConfig)
	}

	servers := app.GetServerStorage()
	if len(servers.Servers) != 1 || servers.Servers[0].Address != "127.0.0.1:27015" {
		t.Fatalf("expected migrated server storage, got %#v", servers)
	}
	if len(servers.RecentServers) != 1 || servers.RecentServers[0].LastConnectedAt != 1779169113000 {
		t.Fatalf("expected migrated recent server storage, got %#v", servers.RecentServers)
	}

	watchLater := app.GetWorkshopWatchLaterStorage()
	if len(watchLater.Items) != 1 || watchLater.Items[0].PublishedFileID != "123" {
		t.Fatalf("expected migrated watch later storage, got %#v", watchLater)
	}
}

func TestMigrateLocalStorageConfigExistingTargetsSkipLegacyData(t *testing.T) {
	app := newConfigTestApp(t)
	writeConfigFixture(t, app.configPath, ConfigFile{
		DefaultDirectory:    "D:/Current",
		DisplayMode:         "card",
		FilterLayoutMode:    "compact",
		Theme:               "current-theme",
		MigrationVersion:    1,
		SavedDirectories:    []SavedDirectory{{Path: "D:/Current", LastUsed: "2026-05-20T00:00:00.000Z"}},
		LastUpdateCheckTime: "111",
	})
	if err := app.SaveServerStorage(ServerStorage{
		Servers:       []SavedServer{{Name: "Current", Address: "10.0.0.1:27015", Weight: 9}},
		RecentServers: []RecentServer{{Name: "Current Recent", Address: "10.0.0.3:27015", LastConnectedAt: 111}},
	}); err != nil {
		t.Fatalf("save server storage: %v", err)
	}
	if err := app.SaveWorkshopWatchLaterStorage(WorkshopWatchLaterStorage{
		Items: []WorkshopWatchLaterItem{{PublishedFileID: "current", Title: "Current Item"}},
	}); err != nil {
		t.Fatalf("save watch later storage: %v", err)
	}
	app.loadConfig()

	if err := app.MigrateLocalStorageConfig(legacyMigrationPayload()); err != nil {
		t.Fatalf("migrate local storage config: %v", err)
	}

	config := app.GetAppConfig()
	if config.MigrationVersion != configMigrationVersion {
		t.Fatalf("expected migration version %d, got %d", configMigrationVersion, config.MigrationVersion)
	}
	if config.DefaultDirectory != "D:/Current" || config.DisplayMode != "card" || config.Theme != "current-theme" || config.LastUpdateCheckTime != "111" {
		t.Fatalf("expected existing config fields to remain, got %#v", config)
	}

	servers := app.GetServerStorage()
	if len(servers.Servers) != 1 || servers.Servers[0].Address != "10.0.0.1:27015" {
		t.Fatalf("expected existing server storage to remain, got %#v", servers)
	}
	if len(servers.RecentServers) != 1 || servers.RecentServers[0].Address != "10.0.0.3:27015" {
		t.Fatalf("expected existing recent server storage to remain, got %#v", servers.RecentServers)
	}

	watchLater := app.GetWorkshopWatchLaterStorage()
	if len(watchLater.Items) != 1 || watchLater.Items[0].PublishedFileID != "current" {
		t.Fatalf("expected existing watch later storage to remain, got %#v", watchLater)
	}
}

func TestMigrateLocalStorageConfigMigratesOnlyMissingServers(t *testing.T) {
	app := newConfigTestApp(t)
	writeConfigFixture(t, app.configPath, ConfigFile{
		DefaultDirectory: "D:/Current",
		DisplayMode:      "card",
		MigrationVersion: 1,
	})
	if err := app.SaveWorkshopWatchLaterStorage(WorkshopWatchLaterStorage{
		Items: []WorkshopWatchLaterItem{{PublishedFileID: "current", Title: "Current Item"}},
	}); err != nil {
		t.Fatalf("save watch later storage: %v", err)
	}
	app.loadConfig()

	if err := app.MigrateLocalStorageConfig(legacyMigrationPayload()); err != nil {
		t.Fatalf("migrate local storage config: %v", err)
	}

	config := app.GetAppConfig()
	if config.MigrationVersion != configMigrationVersion || config.DefaultDirectory != "D:/Current" || config.DisplayMode != "card" {
		t.Fatalf("expected existing config to remain with migrated version, got %#v", config)
	}

	servers := app.GetServerStorage()
	if len(servers.Servers) != 1 || servers.Servers[0].Address != "127.0.0.1:27015" {
		t.Fatalf("expected legacy servers to be migrated, got %#v", servers)
	}
	if len(servers.RecentServers) != 1 || servers.RecentServers[0].LastConnectedAt != 1779169113000 {
		t.Fatalf("expected legacy recent servers to be migrated, got %#v", servers.RecentServers)
	}

	watchLater := app.GetWorkshopWatchLaterStorage()
	if len(watchLater.Items) != 1 || watchLater.Items[0].PublishedFileID != "current" {
		t.Fatalf("expected existing watch later storage to remain, got %#v", watchLater)
	}
}

func TestMigrateLocalStorageConfigMigratesOnlyMissingWatchLater(t *testing.T) {
	app := newConfigTestApp(t)
	writeConfigFixture(t, app.configPath, ConfigFile{
		DefaultDirectory: "D:/Current",
		DisplayMode:      "card",
		MigrationVersion: 1,
	})
	if err := app.SaveServerStorage(ServerStorage{
		Servers:       []SavedServer{{Name: "Current", Address: "10.0.0.1:27015", Weight: 9}},
		RecentServers: []RecentServer{{Name: "Current Recent", Address: "10.0.0.3:27015", LastConnectedAt: 111}},
	}); err != nil {
		t.Fatalf("save server storage: %v", err)
	}
	app.loadConfig()

	if err := app.MigrateLocalStorageConfig(legacyMigrationPayload()); err != nil {
		t.Fatalf("migrate local storage config: %v", err)
	}

	config := app.GetAppConfig()
	if config.MigrationVersion != configMigrationVersion || config.DefaultDirectory != "D:/Current" || config.DisplayMode != "card" {
		t.Fatalf("expected existing config to remain with migrated version, got %#v", config)
	}

	servers := app.GetServerStorage()
	if len(servers.Servers) != 1 || servers.Servers[0].Address != "10.0.0.1:27015" {
		t.Fatalf("expected existing server storage to remain, got %#v", servers)
	}
	if len(servers.RecentServers) != 1 || servers.RecentServers[0].Address != "10.0.0.3:27015" {
		t.Fatalf("expected existing recent server storage to remain, got %#v", servers.RecentServers)
	}

	watchLater := app.GetWorkshopWatchLaterStorage()
	if len(watchLater.Items) != 1 || watchLater.Items[0].PublishedFileID != "123" {
		t.Fatalf("expected legacy watch later storage to be migrated, got %#v", watchLater)
	}
}

func TestMigrateLocalStorageConfigVersionTwoDoesNotOverwrite(t *testing.T) {
	app := newConfigTestApp(t)
	app.defaultDirectory = "D:/Current"
	app.displayMode = "card"
	app.migrationVersion = configMigrationVersion
	app.saveConfig()
	if err := app.SaveServerStorage(ServerStorage{
		Servers: []SavedServer{{Name: "Current", Address: "10.0.0.1:27015", Weight: 9}},
	}); err != nil {
		t.Fatalf("save server storage: %v", err)
	}

	err := app.MigrateLocalStorageConfig(LocalStorageMigrationPayload{
		Config:  `{"defaultDirectory":"D:/Legacy","displayMode":"list"}`,
		Servers: `[{"name":"Legacy","address":"10.0.0.2:27015","weight":1}]`,
	})
	if err != nil {
		t.Fatalf("migrate local storage config: %v", err)
	}

	config := app.GetAppConfig()
	if config.DefaultDirectory != "D:/Current" || config.DisplayMode != "card" {
		t.Fatalf("expected config to remain unchanged, got %#v", config)
	}
	servers := app.GetServerStorage()
	if len(servers.Servers) != 1 || servers.Servers[0].Name != "Current" {
		t.Fatalf("expected server storage to remain unchanged, got %#v", servers.Servers)
	}
}

func TestDamagedSidecarJSONFallsBackToEmptyStorage(t *testing.T) {
	app := newConfigTestApp(t)
	if err := os.WriteFile(app.serversPath, []byte("{bad json"), 0644); err != nil {
		t.Fatalf("write damaged servers json: %v", err)
	}
	if err := os.WriteFile(app.workshopWatchLaterPath, []byte("{bad json"), 0644); err != nil {
		t.Fatalf("write damaged watch later json: %v", err)
	}

	servers := app.GetServerStorage()
	if len(servers.Servers) != 0 || len(servers.RecentServers) != 0 {
		t.Fatalf("expected empty server fallback, got %#v", servers)
	}
	watchLater := app.GetWorkshopWatchLaterStorage()
	if len(watchLater.Items) != 0 {
		t.Fatalf("expected empty watch later fallback, got %#v", watchLater)
	}
}

func TestServerPanelPasswordIsEncryptedAndHidden(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI 加密仅在 Windows 上验证")
	}

	app := newConfigTestApp(t)
	err := app.SaveServerStorage(ServerStorage{
		Servers: []SavedServer{{
			ID:            "srv_test",
			Name:          "Panel",
			Address:       "127.0.0.1:27015",
			Weight:        1,
			PanelURL:      "http://127.0.0.1:27020",
			PanelPassword: "secret-password",
		}},
	})
	if err != nil {
		t.Fatalf("save server storage: %v", err)
	}

	raw, err := os.ReadFile(app.serversPath)
	if err != nil {
		t.Fatalf("read servers file: %v", err)
	}
	if strings.Contains(string(raw), "secret-password") {
		t.Fatalf("expected encrypted storage to hide plaintext password: %s", raw)
	}
	if !strings.Contains(string(raw), "panelPasswordEncrypted") {
		t.Fatalf("expected encrypted password field, got %s", raw)
	}

	storage := app.GetServerStorage()
	if len(storage.Servers) != 1 {
		t.Fatalf("expected one server, got %#v", storage.Servers)
	}
	server := storage.Servers[0]
	if !server.PanelPasswordSet {
		t.Fatalf("expected panelPasswordSet")
	}
	if server.PanelPassword != "" || server.PanelPasswordEncrypted != "" {
		t.Fatalf("frontend storage leaked password fields: %#v", server)
	}
}

func TestServerPanelPasswordPreserveAndClear(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI 加密仅在 Windows 上验证")
	}

	app := newConfigTestApp(t)
	if err := app.SaveServerStorage(ServerStorage{
		Servers: []SavedServer{{
			ID:            "srv_test",
			Name:          "Panel",
			Address:       "127.0.0.1:27015",
			PanelURL:      "http://127.0.0.1:27020",
			PanelPassword: "secret-password",
		}},
	}); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	var stored ServerStorage
	if err := readJSONFile(app.serversPath, &stored); err != nil {
		t.Fatalf("read stored config: %v", err)
	}
	encrypted := stored.Servers[0].PanelPasswordEncrypted
	if encrypted == "" {
		t.Fatalf("expected encrypted password")
	}

	if err := app.SaveServerStorage(ServerStorage{
		Servers: []SavedServer{{
			ID:       "srv_test",
			Name:     "Panel Renamed",
			Address:  "127.0.0.1:27015",
			PanelURL: "http://127.0.0.1:27020",
		}},
	}); err != nil {
		t.Fatalf("save without password: %v", err)
	}
	if err := readJSONFile(app.serversPath, &stored); err != nil {
		t.Fatalf("read preserved config: %v", err)
	}
	if stored.Servers[0].PanelPasswordEncrypted != encrypted {
		t.Fatalf("expected existing encrypted password to be preserved")
	}

	if err := app.SaveServerStorage(ServerStorage{
		Servers: []SavedServer{{
			ID:                 "srv_test",
			Name:               "Panel",
			Address:            "127.0.0.1:27015",
			PanelURL:           "http://127.0.0.1:27020",
			ClearPanelPassword: true,
		}},
	}); err != nil {
		t.Fatalf("clear password: %v", err)
	}
	stored = ServerStorage{}
	if err := readJSONFile(app.serversPath, &stored); err != nil {
		t.Fatalf("read cleared config: %v", err)
	}
	if stored.Servers[0].PanelPasswordEncrypted != "" {
		t.Fatalf("expected encrypted password to be cleared")
	}
}

func TestPanelProxyRequests(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI 加密仅在 Windows 上验证")
	}

	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer panel-secret" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		seen = append(seen, r.URL.Path)

		switch r.URL.Path {
		case "/panel/rcon/getstatus":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Hostname":"Test Host","Map":"c1m1_hotel","Players":"1/8","Difficulty":"普通","GameMode":"合作","Users":[{"Name":"Alice","Id":3,"SteamId":"STEAM_1:1:1","Ip":"127.0.0.1:27005","Location":"本地","Status":"active","Delay":20,"Loss":0,"Duration":"00:10","LinkRate":60000}]}`))
		case "/panel/rcon/changemap":
			if got := r.FormValue("mapName"); got != "c2m1_highway" {
				t.Fatalf("unexpected mapName: %q", got)
			}
			w.Write([]byte("地图切换成功"))
		case "/panel/maps/hot-reload/status":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"using_default":true}`))
		case "/panel/maps/hot-reload":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","message":"地图热重载指令已发送"}`))
		case "/panel/rcon/changedifficulty":
			if got := r.FormValue("difficulty"); got != "高级" {
				t.Fatalf("unexpected difficulty: %q", got)
			}
			w.Write([]byte("难度切换成功"))
		case "/panel/rcon":
			if got := r.FormValue("cmd"); got != "status" {
				t.Fatalf("unexpected rcon cmd: %q", got)
			}
			w.Write([]byte("hostname: Test Host"))
		case "/panel/clear":
			w.Write([]byte("清空成功！"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := newConfigTestApp(t)
	if err := app.SaveServerStorage(ServerStorage{
		Servers: []SavedServer{{
			ID:            "srv_panel",
			Name:          "Panel",
			Address:       "127.0.0.1:27015",
			PanelURL:      server.URL + "/panel",
			PanelPassword: "panel-secret",
		}},
	}); err != nil {
		t.Fatalf("save panel config: %v", err)
	}

	status, err := app.FetchPanelServerStatus("srv_panel")
	if err != nil {
		t.Fatalf("fetch panel status: %v", err)
	}
	if status.Hostname != "Test Host" || status.Users[0].Name != "Alice" {
		t.Fatalf("unexpected status: %#v", status)
	}

	if text, err := app.ChangePanelMap("srv_panel", "c2m1_highway"); err != nil || text != "地图切换成功" {
		t.Fatalf("change map = %q, %v", text, err)
	}
	hotReloadStatus, err := app.FetchPanelMapHotReloadStatus("srv_panel")
	if err != nil || !hotReloadStatus.UsingDefault {
		t.Fatalf("fetch map hot reload status = %#v, %v", hotReloadStatus, err)
	}
	hotReloadResult, err := app.HotReloadPanelMaps("srv_panel")
	if err != nil || hotReloadResult.Status != "ok" || hotReloadResult.Message != "地图热重载指令已发送" {
		t.Fatalf("hot reload maps = %#v, %v", hotReloadResult, err)
	}
	if text, err := app.ChangePanelDifficulty("srv_panel", "高级"); err != nil || text != "难度切换成功" {
		t.Fatalf("change difficulty = %q, %v", text, err)
	}
	if text, err := app.SendPanelRconCommand("srv_panel", "status"); err != nil || text != "hostname: Test Host" {
		t.Fatalf("send rcon = %q, %v", text, err)
	}
	if text, err := app.ClearPanelMaps("srv_panel"); err != nil || text != "清空成功！" {
		t.Fatalf("clear maps = %q, %v", text, err)
	}

	expected := []string{"/panel/rcon/getstatus", "/panel/rcon/changemap", "/panel/maps/hot-reload/status", "/panel/maps/hot-reload", "/panel/rcon/changedifficulty", "/panel/rcon", "/panel/clear"}
	if strings.Join(seen, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected request paths: %#v", seen)
	}
}

func writeConfigFixture(t *testing.T, path string, config ConfigFile) {
	t.Helper()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal config fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
}

func legacyMigrationPayload() LocalStorageMigrationPayload {
	return LocalStorageMigrationPayload{
		Config: `{
			"defaultDirectory":"D:/Games/left4dead2/addons",
			"savedDirectories":[{"path":"D:/Games/left4dead2/addons","lastUsed":"2026-05-19T00:00:00.000Z"}],
			"lastActiveDirectory":"D:/Games/left4dead2/addons",
			"displayMode":"card",
			"filterLayoutMode":"classic",
			"boxSelectionEnabled":true,
			"ctrlClickSelectionEnabled":true,
			"workshopPreferredIP":false,
			"modRotationConfig":{"enableCharacters":true,"enableWeapons":false},
			"ignoredVersion":"1.2.3"
		}`,
		Theme:               "dark",
		LastUpdateCheckTime: "1779169113000",
		Servers:             `[{"name":"Test","address":"127.0.0.1:27015","weight":5}]`,
		RecentServers:       `[{"name":"Test","address":"127.0.0.1:27015","lastConnectedAt":1779169113000}]`,
		WatchLaterItems:     `[{"publishedfileid":"123","title":"Item","preview_url":"https://example.test/a.jpg","views":10,"subscriptions":20,"favorited":30,"file_type":0,"addedAt":"2026-05-19T00:00:00.000Z"}]`,
	}
}

// 「策略组管理」窗口的浮动偏好要能落盘：前端把它放进 config.json，
// Go 端结构体缺字段时会被静默丢弃（真实缺陷：停靠过的窗口重启后又变回浮动）。
func TestSaveAppConfigPersistsStrategyGroupFloating(t *testing.T) {
	app := newConfigTestApp(t)
	app.loadConfig()

	docked := false
	if err := app.SaveAppConfig(ConfigFile{StrategyGroupFloating: &docked}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if got := app.GetAppConfig().StrategyGroupFloating; got == nil || *got {
		t.Fatalf("停靠偏好应落盘为 false: %#v", got)
	}

	// 重新读盘（模拟重启）后仍然保持停靠。
	restored := newConfigTestApp(t)
	restored.configDir = app.configDir
	restored.configPath = app.configPath
	restored.loadConfig()
	if got := restored.GetAppConfig().StrategyGroupFloating; got == nil || *got {
		t.Fatalf("重启后停靠偏好应保持 false: %#v", got)
	}
}

// 主窗口几何（宽 / 高 / 最大化）与上面同一类缺陷：前端在 config.json 里带了这三个字段，
// 但 SaveAppConfig 没把它们写回内存，于是 GetAppConfig 永远返回 null ——
// 启动时的"恢复上次窗口大小"拿到 null 直接跳过，功能整体失效。
func TestSaveAppConfigPersistsMainWindowGeometry(t *testing.T) {
	app := newConfigTestApp(t)
	app.loadConfig()

	width, height := 1280, 760
	maximised := true
	if err := app.SaveAppConfig(ConfigFile{
		MainWindowWidth:     &width,
		MainWindowHeight:    &height,
		MainWindowMaximised: &maximised,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	config := app.GetAppConfig()
	if config.MainWindowWidth == nil || *config.MainWindowWidth != width {
		t.Fatalf("主窗口宽度应落盘为 %d: %#v", width, config.MainWindowWidth)
	}
	if config.MainWindowHeight == nil || *config.MainWindowHeight != height {
		t.Fatalf("主窗口高度应落盘为 %d: %#v", height, config.MainWindowHeight)
	}
	if config.MainWindowMaximised == nil || !*config.MainWindowMaximised {
		t.Fatalf("最大化状态应落盘为 true: %#v", config.MainWindowMaximised)
	}

	// 重新读盘（模拟重启）后仍然记得几何 —— 这才是"窗口大小记得住"的必要条件。
	restored := newConfigTestApp(t)
	restored.configDir = app.configDir
	restored.configPath = app.configPath
	restored.loadConfig()
	reloaded := restored.GetAppConfig()
	if reloaded.MainWindowWidth == nil || *reloaded.MainWindowWidth != width {
		t.Fatalf("重启后主窗口宽度应保持 %d: %#v", width, reloaded.MainWindowWidth)
	}
	if reloaded.MainWindowHeight == nil || *reloaded.MainWindowHeight != height {
		t.Fatalf("重启后主窗口高度应保持 %d: %#v", height, reloaded.MainWindowHeight)
	}
	if reloaded.MainWindowMaximised == nil || !*reloaded.MainWindowMaximised {
		t.Fatalf("重启后最大化状态应保持 true: %#v", reloaded.MainWindowMaximised)
	}
}

// 前端允许的最小主窗口尺寸是 900×600（core/main-window-geometry.mjs），后端必须用同一套下限，
// 否则用户把窗口拉成 1000×700 之后，高度会被判成非法、宽高一起被丢弃（记忆静默失效）。
func TestMainWindowGeometryAcceptsFrontendMinimumSize(t *testing.T) {
	app := newConfigTestApp(t)
	app.loadConfig()

	width, height := 900, 600
	if err := app.SaveAppConfig(ConfigFile{
		MainWindowWidth:  &width,
		MainWindowHeight: &height,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	restored := newConfigTestApp(t)
	restored.configDir = app.configDir
	restored.configPath = app.configPath
	restored.loadConfig()
	reloaded := restored.GetAppConfig()
	if reloaded.MainWindowWidth == nil || *reloaded.MainWindowWidth != width {
		t.Fatalf("900 宽应被接受: %#v", reloaded.MainWindowWidth)
	}
	if reloaded.MainWindowHeight == nil || *reloaded.MainWindowHeight != height {
		t.Fatalf("600 高应被接受: %#v", reloaded.MainWindowHeight)
	}
}

// 非法几何（0 / 负数 / 超出上限 / 手改配置写坏）一律丢弃，绝不能写出 0×0 或超大窗口。
func TestMainWindowGeometryDropsInvalidSizes(t *testing.T) {
	app := newConfigTestApp(t)
	app.loadConfig()

	zeroWidth, zeroHeight := 0, 0
	if err := app.SaveAppConfig(ConfigFile{
		MainWindowWidth:  &zeroWidth,
		MainWindowHeight: &zeroHeight,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if config := app.GetAppConfig(); config.MainWindowWidth != nil || config.MainWindowHeight != nil {
		t.Fatalf("0×0 必须被丢弃: %#v / %#v", config.MainWindowWidth, config.MainWindowHeight)
	}

	huge := 99999
	if err := app.SaveAppConfig(ConfigFile{
		MainWindowWidth:  &huge,
		MainWindowHeight: &huge,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if config := app.GetAppConfig(); config.MainWindowWidth != nil || config.MainWindowHeight != nil {
		t.Fatalf("超大尺寸必须被丢弃: %#v / %#v", config.MainWindowWidth, config.MainWindowHeight)
	}
}

// TestGetServerStorageBackfillsMissingServerIDs 覆盖真机复现：
// servers.json 里没有 id 的服务器（老配置 / 上游迁移过来的文件）会让面板控制报
// 「未找到面板配置对应的服务器」—— 因为读出来的 id 是每次新生成的临时值，
// 面板按 id 回文件里查根本查不到。
func TestGetServerStorageBackfillsMissingServerIDs(t *testing.T) {
	app := newConfigTestApp(t)
	legacy := ServerStorage{
		Servers: []SavedServer{
			{
				Name:                   "Legacy",
				Address:                "127.0.0.1:27015",
				PanelURL:               "http://127.0.0.1:8732",
				PanelPasswordEncrypted: "encrypted-placeholder",
			},
		},
	}
	if err := writeJSONFile(app.configDir, app.serversPath, legacy); err != nil {
		t.Fatalf("写入老配置: %v", err)
	}

	first := app.GetServerStorage()
	if len(first.Servers) != 1 {
		t.Fatalf("服务器数量 = %d", len(first.Servers))
	}
	id := first.Servers[0].ID
	if strings.TrimSpace(id) == "" {
		t.Fatal("读取老配置时应补上 id")
	}

	second := app.GetServerStorage()
	if second.Servers[0].ID != id {
		t.Fatalf("id 必须稳定：%q → %q", id, second.Servers[0].ID)
	}

	var stored ServerStorage
	if err := readJSONFile(app.serversPath, &stored); err != nil {
		t.Fatalf("重新读取磁盘配置: %v", err)
	}
	if stored.Servers[0].ID != id {
		t.Fatalf("磁盘上的 id = %q，期望 %q（应写回）", stored.Servers[0].ID, id)
	}
}

// 「分类侧边栏」的展开偏好要能落盘：前端把它放进 config.json，
// Go 端结构体缺字段时会被静默丢弃（与策略组浮动那次是同一类缺陷）。
func TestSaveAppConfigPersistsCategorySidebarVisible(t *testing.T) {
	app := newConfigTestApp(t)
	app.loadConfig()

	// 没设置过 → 保持 nil（前端据此走默认：收起，不占列表空间）。
	if got := app.GetAppConfig().CategorySidebarVisible; got != nil {
		t.Fatalf("没设置过时应保持 nil: %#v", got)
	}

	visible := true
	if err := app.SaveAppConfig(ConfigFile{CategorySidebarVisible: &visible}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if got := app.GetAppConfig().CategorySidebarVisible; got == nil || !*got {
		t.Fatalf("展开偏好应落盘为 true: %#v", got)
	}

	// 重新读盘（模拟重启）后仍然记得：清掉内存状态再 loadConfig。
	app.categorySidebarVisible = nil
	app.loadConfig()
	if got := app.GetAppConfig().CategorySidebarVisible; got == nil || !*got {
		t.Fatalf("重启后应读回 true: %#v", got)
	}

	// 收起也要能记住：false 不能被当成"没设置过"。
	hidden := false
	if err := app.SaveAppConfig(ConfigFile{CategorySidebarVisible: &hidden}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	app.categorySidebarVisible = nil
	app.loadConfig()
	if got := app.GetAppConfig().CategorySidebarVisible; got == nil || *got {
		t.Fatalf("重启后应读回 false: %#v", got)
	}
}
