package app

import (
	"fmt"
	"strings"
)

// 「提醒窗口」体系：会话级选择 + 设置默认。
//
// 语义（与前端 core/reminders.mjs 同一套键）：
//   - 设置默认：config.json 的 RemindersHidden[key]=true = 每次启动应用后默认不显示该提醒；
//   - 会话级：每个提醒在"本次运行第一次弹出"时问一次（「本次运行期间不再显示此提醒」），
//     选择只保留到进程结束（本文件内存态，重开应用即重置，回落到设置默认）；
//   - Choice：会话级选择可附带一个动作（如同类冲突的 replace / coexist），
//     供"静音后按当时选择自动处理"使用；其它提醒为空。
//
// 当前键：singleton-conflict（同类互斥）/ vpk-risk（VPK 风险提示）。

type ReminderSessionState struct {
	Asked      bool   `json:"asked"`
	Suppressed bool   `json:"suppressed"`
	Choice     string `json:"choice,omitempty"`
}

// sanitizeReminderKey 校验提醒键：小写、仅 [a-z0-9-]、≤64 字符。
func sanitizeReminderKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || len(key) > 64 {
		return ""
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return ""
		}
	}
	return key
}

// sanitizeRemindersHidden 只保留值为 true 的合法键（false/非法键视为"显示"并丢弃）。
func sanitizeRemindersHidden(input map[string]bool) map[string]bool {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]bool, len(input))
	for key, hidden := range input {
		if !hidden {
			continue
		}
		if sanitized := sanitizeReminderKey(key); sanitized != "" {
			out[sanitized] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetReminderSessionStates 返回本进程内所有提醒的会话状态（前端启动/按需拉取；重开应用清空）。
func (a *App) GetReminderSessionStates() map[string]ReminderSessionState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]ReminderSessionState, len(a.reminderSession))
	for key, state := range a.reminderSession {
		out[key] = state
	}
	return out
}

// SetReminderSessionState 写入一次会话级选择（界面在提醒弹窗关闭时调用）。
// choice 只接受 replace / coexist（其余清空）；状态只保留到进程结束。
func (a *App) SetReminderSessionState(key string, suppressed bool, choice string) error {
	sanitized := sanitizeReminderKey(key)
	if sanitized == "" {
		return fmt.Errorf("无效的提醒键：%q", key)
	}
	choice = strings.ToLower(strings.TrimSpace(choice))
	if choice != "replace" && choice != "coexist" {
		choice = ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.reminderSession == nil {
		a.reminderSession = make(map[string]ReminderSessionState, 2)
	}
	a.reminderSession[sanitized] = ReminderSessionState{Asked: true, Suppressed: suppressed, Choice: choice}
	return nil
}
