package app

import (
	"log"
	"os"
)

// addonlist.txt 的"事务化提交"（对齐 FireAxe `AddonRoot.Push()` 的语义：
// 先把目标状态算完整，再一次性落盘；任一步失败都回到写之前的一致状态）。
//
// 为什么需要：`writeAddonList*` 本身已经是"临时文件 + 原子替换"，写失败不会写坏文件；
// 但"写盘成功、受保护快照同步失败"会留下**文件已改、快照还是旧**的半新半旧状态 ——
// 之后运行时监控恢复就会用旧快照把用户刚做的修改覆盖掉。
// 这里把「写盘 → 派生步骤（刷新缓存）→ 快照同步」包成一个事务：
// 快照同步失败就用写前内容回滚文件，并让派生步骤按回滚后的内容重跑一次。
//
// 调用方必须已经持有 `addonListGuardMu`（与现有写盘路径一致）。
func runAddonListTransaction(path string, write func() error, afterWrite func(), sync func() error) error {
	original, readErr := os.ReadFile(path)
	hadOriginal := readErr == nil

	if err := write(); err != nil {
		return err
	}
	if afterWrite != nil {
		afterWrite()
	}
	if err := sync(); err != nil {
		if hadOriginal {
			if restoreErr := os.WriteFile(path, original, 0o644); restoreErr != nil {
				log.Printf("addonlist.txt 事务回滚失败（文件保持新内容）: %v", restoreErr)
			} else if afterWrite != nil {
				// 回滚后让派生状态（内存里的游戏开关）重新对齐到旧内容。
				afterWrite()
			}
		}
		return err
	}
	return nil
}

// commitAddonListDocumentLocked 事务化提交"文档式"写入（保编码 / BOM）。
func (a *App) commitAddonListDocumentLocked(doc addonListDocument, content string, afterWrite func()) error {
	return runAddonListTransaction(
		doc.path,
		func() error { return a.writeAddonListDocument(doc, content) },
		afterWrite,
		func() error { return a.syncManagedAddonListSnapshotLocked(doc.path) },
	)
}

// commitAddonListItemsLocked 事务化提交"列表式"写入。
func (a *App) commitAddonListItemsLocked(path string, items []AddonListItem, afterWrite func()) error {
	return runAddonListTransaction(
		path,
		func() error { return a.writeAddonList(path, items) },
		afterWrite,
		func() error { return a.syncManagedAddonListSnapshotLocked(path) },
	)
}
