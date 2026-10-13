package app

import "testing"

func TestReminderSessionState(t *testing.T) {
	a := &App{}
	if err := a.SetReminderSessionState("singleton-conflict", true, "Replace"); err != nil {
		t.Fatalf("写入会话状态失败: %v", err)
	}
	states := a.GetReminderSessionStates()
	got, ok := states["singleton-conflict"]
	if !ok || !got.Asked || !got.Suppressed || got.Choice != "replace" {
		t.Fatalf("会话状态不对: %+v", got)
	}

	// 非法键拒绝；非白名单 choice 清空。
	if err := a.SetReminderSessionState("bad key!", false, ""); err == nil {
		t.Fatal("非法键应报错")
	}
	if err := a.SetReminderSessionState("vpk-risk", false, "whatever"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if state := a.GetReminderSessionStates()["vpk-risk"]; state.Choice != "" || state.Suppressed {
		t.Fatalf("非法 choice 应清空、suppressed=false: %+v", state)
	}
}

func TestRemindersHiddenRoundTrip(t *testing.T) {
	first := newConfigTestApp(t)
	first.loadConfig()

	config := first.GetAppConfig()
	config.RemindersHidden = map[string]bool{"vpk-risk": true, "Bad Key!": true, "singleton-conflict": false}
	if err := first.SaveAppConfig(config); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	second := configTestAppAtDir(first.configDir)
	second.loadConfig()
	got := second.GetAppConfig().RemindersHidden
	if len(got) != 1 || !got["vpk-risk"] {
		t.Fatalf("重启后应只剩 {vpk-risk:true}，实际 %+v", got)
	}

	// 清空后回落 nil（配置文件里不再出现该键）。
	config = second.GetAppConfig()
	config.RemindersHidden = map[string]bool{}
	if err := second.SaveAppConfig(config); err != nil {
		t.Fatalf("清空保存失败: %v", err)
	}
	third := configTestAppAtDir(second.configDir)
	third.loadConfig()
	if third.GetAppConfig().RemindersHidden != nil {
		t.Fatalf("清空后应为 nil，实际 %+v", third.GetAppConfig().RemindersHidden)
	}
}
