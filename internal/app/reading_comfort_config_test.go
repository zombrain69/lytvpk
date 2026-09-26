package app

import "testing"

// 阅读舒适度的两个档位要能落盘、能读回，非法值必须回落而不是透传给界面。
func TestReadingComfortConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := &App{configDir: dir}

	// 全新配置：默认标准档。
	config := a.GetAppConfig()
	if config.TextSize != "standard" || config.ReadingComfort != "standard" {
		t.Fatalf("默认档位应为 standard：%+v", config)
	}

	// 改成大字号 + 高对比，落盘后重新加载。
	config.TextSize = "xlarge"
	config.ReadingComfort = "contrast"
	if err := a.SaveAppConfig(config); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	reloaded := &App{configDir: dir}
	reloaded.loadConfig()
	got := reloaded.GetAppConfig()
	if got.TextSize != "xlarge" || got.ReadingComfort != "contrast" {
		t.Fatalf("重新加载后档位丢失：%+v", got)
	}
}

func TestReadingComfortConfigNormalizesIllegalValues(t *testing.T) {
	dir := t.TempDir()
	a := &App{configDir: dir}

	config := a.GetAppConfig()
	config.TextSize = "巨无霸"
	config.ReadingComfort = "dark-magic"
	if err := a.SaveAppConfig(config); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if got := a.GetAppConfig(); got.TextSize != "standard" || got.ReadingComfort != "standard" {
		t.Fatalf("非法档位应回落为 standard：%+v", got)
	}

	// 大小写与空白要被容忍（手改 config.json 的常见情况）。
	config = a.GetAppConfig()
	config.TextSize = " Large "
	config.ReadingComfort = "SOFT"
	if err := a.SaveAppConfig(config); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if got := a.GetAppConfig(); got.TextSize != "large" || got.ReadingComfort != "soft" {
		t.Fatalf("应大小写不敏感并去空白：%+v", got)
	}
}
