package parser

// StockLookup 是「游戏本体文件索引」在解析器侧的最小接口。
//
// 由 internal/gamedata.StockIndex 实现（结构化接口，不需要在 parser 里 import 它，
// 从而避免 parser ↔ gamedata 的包循环）。解析器只把它当作**可选证据来源**：
// nil 或实现不完整时，一切依赖本体索引的判定自动跳过，既有标签产出不受影响
// —— 这是"可以多标、不能少标"约束在异常路径上的体现。
type StockLookup interface {
	// Exists 判断某个归档内路径是否属于游戏本体（大小写不敏感、"/" 分隔）。
	Exists(path string) bool
	// Glob 返回匹配 glob 的本体路径（支持 `**` 跨目录、`*` 不跨目录、`?` 单字符）。
	Glob(pattern string) []string
}
