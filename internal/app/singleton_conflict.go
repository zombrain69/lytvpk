package app

import (
	"fmt"
	"sort"
	"strings"
)

// 「同类互斥」提示（借鉴 xavier-cai/L4D2ModManager 的 SingletonResource 思路）。
//
// 背景：同一角色的皮肤/模型、同一把武器的替换包，在游戏里其实**只会生效一个**
// （后加载覆盖前者，或者引擎随机挑一个）。我们的事后冲突分析能看出"文件重叠"，
// 但用户点"启用"的那一刻并不知情，于是经常出现"启用了却没生效"。
//
// 这里的口径刻意保守：只对"明确是同一个槽位"的分类报警——
//   - 角色：Bill/Francis/Louis/Zoey/Coach/Ellis/Nick/Rochelle + 特感（Hunter 等）；
//   - 武器：具体型号（AK47 / M16 / 大狙…），不包含"步枪 / 霰弹枪"这种类别标签，
//     因为不同型号的步枪并不互斥。
// 宁可不报，也不要用"类别"标签把所有枪都算成互斥。

// singletonCharacterTags 是"同一角色只能有一个替换生效"的标签。
var singletonCharacterTags = map[string]string{
	"Bill": "Bill", "Francis": "Francis", "Louis": "Louis", "Zoey": "Zoey",
	"Coach": "Coach", "Ellis": "Ellis", "Nick": "Nick", "Rochelle": "Rochelle",
	"Hunter": "Hunter", "Smoker": "Smoker", "Boomer": "Boomer", "Tank": "Tank",
	"Charger": "Charger", "Jockey": "Jockey", "Spitter": "Spitter", "Witch": "Witch",
}

// singletonWeaponTags 是"同一把武器/近战只能有一个替换生效"的具体型号标签。
var singletonWeaponTags = map[string]string{
	"AK47": "AK47", "M16": "M16", "sg552": "SG552", "三连发": "三连发", "M60": "M60",
	"乌兹": "乌兹", "消音": "消音", "MP5": "MP5",
	"大狙": "大狙", "军狙": "军狙", "猎枪": "猎枪", "鸟狙": "鸟狙",
	"木喷": "木喷", "铁喷": "铁喷", "一代连喷": "一代连喷", "二代连喷": "二代连喷",
	"马格南": "马格南", "小手枪": "小手枪", "榴弹发射器": "榴弹发射器",
	"砍刀": "砍刀", "武士刀": "武士刀", "棒球棍": "棒球棍", "匕首": "匕首", "电锯": "电锯",
	"撬棍": "撬棍", "消防斧": "消防斧", "平底锅": "平底锅", "吉他": "吉他",
	"板球拍": "板球拍", "警棍": "警棍", "高尔夫球杆": "高尔夫球杆", "铁铲": "铁铲",
	"草叉": "草叉", "防爆盾": "防爆盾",
	"一代固定机枪": "一代固定机枪", "二代固定机枪": "二代固定机枪",
}

type ModEnableConflictItem struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

type ModEnableConflict struct {
	// Key 形如 "人物:Zoey" / "武器:AK47"，与前端提示文案共用。
	Key       string                  `json:"key"`
	Label     string                  `json:"label"`
	Conflicts []ModEnableConflictItem `json:"conflicts"`
}

// CheckModEnableConflict 检查"游戏内启用这个 Mod"时会不会和已启用的同类 Mod 撞车。
// 没找到文件或没有同类已启用项时返回空结构 + nil（调用方按"没有冲突"处理）。
func (a *App) CheckModEnableConflict(filePath string) (ModEnableConflict, error) {
	target := strings.TrimSpace(filePath)
	if target == "" {
		return ModEnableConflict{}, fmt.Errorf("缺少 Mod 路径")
	}
	cached, ok := a.vpkCache.Load(target)
	if !ok {
		return ModEnableConflict{}, nil
	}
	cache, ok := cached.(*VPKFileCache)
	if !ok || cache == nil {
		return ModEnableConflict{}, nil
	}
	targetTags := singletonTagsFor(cache.File)
	if len(targetTags) == 0 {
		return ModEnableConflict{}, nil
	}

	type conflictBucket struct {
		key   string
		label string
		items []ModEnableConflictItem
	}
	buckets := make(map[string]*conflictBucket, len(targetTags))
	for _, key := range targetTags {
		buckets[key] = &conflictBucket{key: key, label: SingletonConflictLabel(key)}
	}
	a.vpkCache.Range(func(key, value interface{}) bool {
		other, ok := value.(*VPKFileCache)
		if !ok || other == nil || other.File.Path == target {
			return true
		}
		file := other.File
		// 只有"真的会被游戏加载"的同类 Mod 才算互斥：必须是游戏内开启状态。
		if !file.Enabled || !file.GameEnabled {
			return true
		}
		for _, tag := range singletonTagsFor(file) {
			bucket, exists := buckets[tag]
			if !exists {
				continue
			}
			bucket.items = append(bucket.items, ModEnableConflictItem{
				Path: file.Path,
				Name: file.Name,
				Tag:  tag,
			})
		}
		return true
	})
	if len(buckets) == 0 {
		return ModEnableConflict{}, nil
	}

	// 命中多个分类时，报告"冲突项最多"的那一条；界面一次只处理一个提示。
	list := make([]*conflictBucket, 0, len(buckets))
	for _, bucket := range buckets {
		if len(bucket.items) == 0 {
			continue
		}
		sort.Slice(bucket.items, func(i, j int) bool { return bucket.items[i].Name < bucket.items[j].Name })
		list = append(list, bucket)
	}
	if len(list) == 0 {
		return ModEnableConflict{}, nil
	}
	sort.Slice(list, func(i, j int) bool {
		if len(list[i].items) != len(list[j].items) {
			return len(list[i].items) > len(list[j].items)
		}
		return list[i].key < list[j].key
	})
	best := list[0]
	return ModEnableConflict{
		Key:       best.key,
		Label:     best.label,
		Conflicts: best.items,
	}, nil
}

// singletonTagsFor 返回一个 Mod 命中的互斥分类（形如 "人物:Zoey" 的键 → 展示标签）。
func singletonTagsFor(file VPKFile) []string {
	if len(file.SecondaryTags) == 0 {
		return nil
	}
	result := make([]string, 0, 2)
	for _, tag := range file.SecondaryTags {
		if _, ok := singletonCharacterTags[tag]; ok {
			result = append(result, "人物:"+tag)
			continue
		}
		if _, ok := singletonWeaponTags[tag]; ok {
			result = append(result, "武器:"+tag)
		}
	}
	return result
}

// SingletonConflictLabel 把互斥键转成给用户看的一句话。
func SingletonConflictLabel(key string) string {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return key
	}
	switch parts[0] {
	case "人物":
		return fmt.Sprintf("角色 %s 的替换（同一角色只会生效一个）", parts[1])
	case "武器":
		return fmt.Sprintf("%s 的替换（同一把武器只会生效一个）", parts[1])
	default:
		return key
	}
}
