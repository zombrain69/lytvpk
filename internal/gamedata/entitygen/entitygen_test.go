package entitygen

import "testing"

// TestParseScriptReadsAuthoritativeKeys 覆盖"从脚本里读权威字段"这一层：
// playermodel / viewmodel / CharacterViewmodelAddon 必须被正确解析。
func TestParseScriptReadsAuthoritativeKeys(t *testing.T) {
	sample := []byte(`WeaponData
{
	"printname"			"Pump Shotgun"
	"playermodel"		"models/w_models/weapons/w_pumpshotgun_A.mdl"
	"viewmodel"			"models/v_models/v_shotgun_chrome.mdl"
	"CharacterViewmodelAddon"
	{
		"Coach"				"models/weapons/arms/v_arms_coach_new.mdl"
		"Mechanic"			"models/weapons/arms/v_arms_mechanic_new.mdl"
	}
}
`)
	parsed, err := parseScript(sample)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.printName != "Pump Shotgun" {
		t.Fatalf("printname=%q", parsed.printName)
	}
	if parsed.playermodel != "models/w_models/weapons/w_pumpshotgun_a.mdl" {
		t.Fatalf("playermodel=%q", parsed.playermodel)
	}
	if parsed.viewmodel != "models/v_models/v_shotgun_chrome.mdl" {
		t.Fatalf("viewmodel=%q", parsed.viewmodel)
	}
	if parsed.arms["Coach"] != "models/weapons/arms/v_arms_coach_new.mdl" {
		t.Fatalf("CharacterViewmodelAddon 未解析：%v", parsed.arms)
	}
}

// TestScriptSpecsCoverStockScripts 保证维护者名单覆盖了游戏里实际存在的脚本：
// 少了任何一条都会在这里显式暴露（而不是悄悄少标）。
func TestScriptSpecsCoverStockScripts(t *testing.T) {
	required := []string{
		"scripts/weapon_rifle_ak47.txt", "scripts/weapon_shotgun_chrome.txt",
		"scripts/weapon_pumpshotgun.txt", "scripts/weapon_cola_bottles.txt",
		"scripts/weapon_fireworkcrate.txt", "scripts/weapon_rifle_m60.txt",
		"scripts/weapon_hunter_claw.txt", "scripts/weapon_tank_claw.txt",
		"scripts/melee/golfclub.txt", "scripts/melee/pitchfork.txt",
		"scripts/melee/shovel.txt", "scripts/weapon_chainsaw.txt",
	}
	for _, path := range required {
		spec, ok := scriptSpecs[path]
		if !ok {
			t.Fatalf("scriptSpecs 缺少 %q", path)
		}
		if spec.Skip {
			t.Fatalf("%q 不应被跳过", path)
		}
	}
	if !scriptSpecs["scripts/weapon_melee.txt"].Skip || !scriptSpecs["scripts/weapon_manifest.txt"].Skip {
		t.Fatalf("基类/清单脚本必须跳过（它们的 playermodel 是共享占位模型）")
	}
	if len(armsCharacters) == 0 {
		t.Fatalf("手臂角色映射不应为空")
	}
}
