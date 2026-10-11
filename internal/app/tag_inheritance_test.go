package app

import (
	"strings"
	"testing"

	"vpk-manager/internal/parser"
)

func containsInheritedTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func storeInheritanceMod(app *App, path string, tags []string, roots []string) {
	app.vpkCache.Store(path, &VPKFileCache{File: parser.VPKFile{
		Path:                   path,
		Name:                   path,
		SecondaryTags:          tags,
		StructureResourceRoots: roots,
	}})
}

// TestSuiteTagInheritanceOnlyAddsAndRespectsThreshold 覆盖 W4 通道 5：
// 命名空间里 ≥60% 成员都带的标签补给缺它的成员；只加不删；成员太少的命名空间不动。
func TestSuiteTagInheritanceOnlyAddsAndRespectsThreshold(t *testing.T) {
	app := &App{rootDir: t.TempDir()}
	storeInheritanceMod(app, "a.vpk", []string{"AK47"}, []string{"codm/ice"})
	storeInheritanceMod(app, "b.vpk", []string{"AK47", "贴图"}, []string{"codm/ice"})
	storeInheritanceMod(app, "c.vpk", []string{"贴图"}, []string{"codm/ice"})
	// 两个成员的命名空间不满足最小成员数（3），不应继承。
	storeInheritanceMod(app, "d.vpk", []string{"M16"}, []string{"solo/one"})
	storeInheritanceMod(app, "e.vpk", []string{}, []string{"solo/one"})
	// 只有一个成员的命名空间也不动。
	storeInheritanceMod(app, "f.vpk", []string{"武士刀"}, []string{"solo/two"})

	inherited := app.applySuiteTagInheritance()
	// "AK47"（2/3）补给 c；"贴图"（2/3）补给 a。
	if inherited != 2 {
		t.Fatalf("应补 2 个标签，实际 %d", inherited)
	}

	c := app.mustCachedVPKFile(t, "c.vpk")
	if !containsInheritedTag(c.SecondaryTags, "AK47") || !containsInheritedTag(c.SecondaryTags, "贴图") {
		t.Fatalf("c 应同时带 AK47 与原标签：%v", c.SecondaryTags)
	}
	// 继承来的标签要带证据，说明它来自哪个套件命名空间。
	found := false
	for _, item := range c.TagEvidence {
		if item.Tag == "AK47" && item.Rule == "suite:codm/ice" {
			found = true
		}
	}
	if !found {
		t.Fatalf("继承标签应记录 suite 证据：%+v", c.TagEvidence)
	}
	a := app.mustCachedVPKFile(t, "a.vpk")
	if !containsInheritedTag(a.SecondaryTags, "AK47") || !containsInheritedTag(a.SecondaryTags, "贴图") {
		t.Fatalf("a 应保留 AK47 并补上贴图：%v", a.SecondaryTags)
	}
	e := app.mustCachedVPKFile(t, "e.vpk")
	if len(e.SecondaryTags) != 0 {
		t.Fatalf("成员不足的命名空间不应继承：%v", e.SecondaryTags)
	}
	f := app.mustCachedVPKFile(t, "f.vpk")
	if len(f.SecondaryTags) != 1 {
		t.Fatalf("单成员命名空间不应继承：%v", f.SecondaryTags)
	}

	// 幂等：再跑一次不应重复追加。
	if again := app.applySuiteTagInheritance(); again != 0 {
		t.Fatalf("重复执行应为 0，实际 %d", again)
	}
}

// TestSuiteTagInheritanceIsDeterministic 覆盖确定性：命名空间用 map 存，
// 若不排序会让"成员上限"截断产生随机结果（真机表现为两次导出的标签数不同）。
func TestSuiteTagInheritanceIsDeterministic(t *testing.T) {
	build := func() *App {
		app := &App{rootDir: t.TempDir()}
		// 两个命名空间，成员都接近上限，顺序不同会截断出不同标签。
		tags := make([]string, 0, suiteInheritanceTagCap-1)
		for i := 0; i < suiteInheritanceTagCap-1; i++ {
			tags = append(tags, "标签"+string(rune('A'+i)))
		}
		storeInheritanceMod(app, "x1.vpk", append(append([]string{}, tags...), "AK47"), []string{"a/suite"})
		storeInheritanceMod(app, "x2.vpk", append(append([]string{}, tags...), "AK47"), []string{"a/suite"})
		storeInheritanceMod(app, "x3.vpk", tags, []string{"a/suite"})
		storeInheritanceMod(app, "y1.vpk", append(append([]string{}, tags...), "M16"), []string{"b/suite"})
		storeInheritanceMod(app, "y2.vpk", append(append([]string{}, tags...), "M16"), []string{"b/suite"})
		storeInheritanceMod(app, "y3.vpk", tags, []string{"b/suite"})
		return app
	}

	first := build()
	first.applySuiteTagInheritance()
	firstX3 := first.mustCachedVPKFile(t, "x3.vpk")
	firstY3 := first.mustCachedVPKFile(t, "y3.vpk")

	for i := 0; i < 5; i++ {
		app := build()
		app.applySuiteTagInheritance()
		x3 := app.mustCachedVPKFile(t, "x3.vpk")
		y3 := app.mustCachedVPKFile(t, "y3.vpk")
		if strings.Join(x3.SecondaryTags, ",") != strings.Join(firstX3.SecondaryTags, ",") ||
			strings.Join(y3.SecondaryTags, ",") != strings.Join(firstY3.SecondaryTags, ",") {
			t.Fatalf("第 %d 次结果不一致：x3=%v y3=%v", i, x3.SecondaryTags, y3.SecondaryTags)
		}
	}
}

// TestSuiteTagInheritanceCapCountsInheritedAddsNotTotalTags 是 2026-10-12 的回归：
// 旧实现按"成员总标签数 ≥ 上限就跳过"，成员自身标签因识别升级而增长后
// （真机案例：Dead Before Dawn Part 2 / 3583381403 升级为地图分片包，自身到 20+），
// 此前靠继承拿到的标签会被挤掉，违反"只增不减"。现在上限只计"本次为该成员
// 新增的继承标签数"：自身标签再多，缺的特征标签照样补。
func TestSuiteTagInheritanceCapCountsInheritedAddsNotTotalTags(t *testing.T) {
	app := &App{rootDir: t.TempDir()}
	own := make([]string, 0, suiteInheritanceTagCap+4)
	for i := 0; i < suiteInheritanceTagCap+4; i++ {
		own = append(own, "自身"+string(rune('A'+i)))
	}
	storeInheritanceMod(app, "rich.vpk", own, []string{"ns/rich"})
	storeInheritanceMod(app, "rich2.vpk", append(append([]string{}, own...), "AK47"), []string{"ns/rich"})
	storeInheritanceMod(app, "rich3.vpk", append(append([]string{}, own...), "AK47"), []string{"ns/rich"})

	inherited := app.applySuiteTagInheritance()
	if inherited != 1 {
		t.Fatalf("超出旧口径上限的成员也应补 1 个继承标签，实际 %d", inherited)
	}
	rich := app.mustCachedVPKFile(t, "rich.vpk")
	if !containsInheritedTag(rich.SecondaryTags, "AK47") {
		t.Fatalf("自身标签多不应挡住继承补齐：%v", rich.SecondaryTags)
	}
}

// TestSuiteTagInheritancePerMemberRunCapStillApplies 守住上限本身：
// 单个成员单次扫描的新增继承标签不会超过 suiteInheritanceTagCap（防止堆标签）。
func TestSuiteTagInheritancePerMemberRunCapStillApplies(t *testing.T) {
	app := &App{rootDir: t.TempDir()}
	features := make([]string, 0, suiteInheritanceTagCap+8)
	for i := 0; i < suiteInheritanceTagCap+8; i++ {
		features = append(features, "特征"+string(rune('A'+i)))
	}
	// 3 个成员共享 28 个特征标签；第 4 个成员自身没有标签、缺全部特征。
	storeInheritanceMod(app, "m1.vpk", append([]string{}, features...), []string{"ns/cap"})
	storeInheritanceMod(app, "m2.vpk", append([]string{}, features...), []string{"ns/cap"})
	storeInheritanceMod(app, "m3.vpk", append([]string{}, features...), []string{"ns/cap"})
	storeInheritanceMod(app, "empty.vpk", []string{}, []string{"ns/cap"})

	inherited := app.applySuiteTagInheritance()
	if inherited != suiteInheritanceTagCap {
		t.Fatalf("单成员单次新增继承标签应截断为 %d，实际 %d", suiteInheritanceTagCap, inherited)
	}
	empty := app.mustCachedVPKFile(t, "empty.vpk")
	if len(empty.SecondaryTags) != suiteInheritanceTagCap {
		t.Fatalf("截断后应恰好 %d 个标签，实际 %d", suiteInheritanceTagCap, len(empty.SecondaryTags))
	}
}
