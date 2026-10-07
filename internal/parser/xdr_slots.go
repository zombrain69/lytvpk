package parser

// XDR（xdReanimsBase）的官方槽位建议表。
//
// 来源：基础包作者 xdshot 在工坊讨论区给出的 "Slots designations"
// （https://steamcommunity.com/workshop/filedetails/discussion/2121557118/2290590708546536185/）。
// 原文是一串 `$definevariable SLOT_xxx NNN`，这里原样保留槽位号与名字，只补一层中文归类。
//
// 注意：这是**建议**用途，不是事实。真实库里有作者把蹲姿动作放进 SLOT_Rescue（031）这类例子，
// 所以界面上只能写成"官方建议：xxx"，不能用它反推"这个 Mod 到底替换了什么动作"。
type XDRSlotDesignation struct {
	Slot  int
	Name  string
	Group string
}

var xdrSlotDesignations = map[int]XDRSlotDesignation{
	1:  {1, "noise", "闲置层"},
	2:  {2, "Pistol", "武器"},
	3:  {3, "Elites", "武器"},
	4:  {4, "Rifles", "武器"},
	5:  {5, "Shotgun", "武器"},
	6:  {6, "PumpShotgun", "武器"},
	7:  {7, "Grenade", "武器"},
	8:  {8, "FirstAidKit", "武器"},
	9:  {9, "SMG", "武器"},
	10: {10, "HuntingRifle", "武器"},
	11: {11, "SniperMilitary", "武器"},
	12: {12, "idle_FyringPan", "武器"},
	13: {13, "idle_Bat", "武器"},
	14: {14, "idle_Axe", "武器"},
	15: {15, "idle_guitar", "武器"},
	16: {16, "Chainsaw", "武器"},
	17: {17, "idle_standing", "站立/移动"},
	18: {18, "idle_crouching", "站立/移动"},
	19: {19, "idle_calm", "站立/移动"},
	20: {20, "idle_Injured", "站立/移动"},
	21: {21, "walk", "站立/移动"},
	22: {22, "CrouchWalk", "站立/移动"},
	23: {23, "Run", "站立/移动"},
	24: {24, "CalmWalk", "站立/移动"},
	25: {25, "CalmRun", "站立/移动"},
	26: {26, "LimpWalk", "受伤移动"},
	27: {27, "LimpRun", "受伤移动"},
	28: {28, "Shoot_GrenThrow", "战斗"},
	29: {29, "Melee_Sweep_Shove_Straight_stomp", "战斗"},
	30: {30, "Reload", "战斗"},
	31: {31, "Rescue", "救援/治疗"},
	32: {32, "Heal_self", "救援/治疗"},
	33: {33, "Heal_Other", "救援/治疗"},
	34: {34, "Jump_Fall_landing", "移动"},
	35: {35, "Ladder", "移动"},
	36: {36, "Incap", "倒地/受击"},
	37: {37, "Flinch", "倒地/受击"},
	38: {38, "Shoved", "倒地/受击"},
	39: {39, "gestures", "手势"},
	40: {40, "Pounced", "被控"},
	41: {41, "Smoked", "被控"},
	42: {42, "Crushed", "被控"},
	43: {43, "Ridden", "被控"},
	44: {44, "其它（44–47）", "其它"},
	45: {45, "其它（44–47）", "其它"},
	46: {46, "其它（44–47）", "其它"},
	47: {47, "其它（44–47）", "其它"},
	48: {48, "animfixes", "动画修复"},
}

// XDRSlotDesignationFor 返回官方建议用途；不在 1–48 内返回零值。
func XDRSlotDesignationFor(slot int) XDRSlotDesignation {
	if meta, ok := xdrSlotDesignations[slot]; ok {
		return meta
	}
	return XDRSlotDesignation{}
}

// XDRSlotRangeText 是给界面用的槽位范围说明（框架每个角色最多 48 个槽）。
const XDRSlotRangeText = "1–48"
