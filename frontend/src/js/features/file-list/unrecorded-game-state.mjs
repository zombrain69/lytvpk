import { t } from "../../core/i18n-runtime.js";

// 三个动作的文案走目录（mods.gameState.*；动作名复用 mods.batch.gameEnable|gameDisable）：
// 静态表只存 key，取值时经 t() 取当前语言（模块求值期不调 t()）。
const UNRECORDED_GAME_STATE_ACTIONS = Object.freeze([
  Object.freeze({
    id: "game-disabled",
    labelKey: "mods.batch.gameDisable",
    descriptionKey: "mods.gameState.descGameOff",
    disabled: false,
  }),
  Object.freeze({
    id: "game-enabled",
    labelKey: "mods.batch.gameEnable",
    descriptionKey: "mods.gameState.descGameOn",
    disabled: false,
  }),
  Object.freeze({
    id: "disabled",
    labelKey: "mods.gameState.labelDisabled",
    descriptionKey: "mods.gameState.descDisabled",
    disabled: false,
  }),
]);

export function getUnrecordedGameStateOptions(file = {}) {
  const location = String(file?.location || "").trim().toLowerCase();
  return UNRECORDED_GAME_STATE_ACTIONS.map((option) => {
    const { labelKey, descriptionKey, ...rest } = option;
    const resolved = { ...rest, label: t(labelKey), description: t(descriptionKey) };
    if (option.id !== "disabled" || location !== "workshop") {
      return resolved;
    }
    return {
      ...resolved,
      disabled: true,
      disabledReason: t("mods.gameState.workshopDisabled"),
    };
  });
}

export function getGameStateDisplayModel(file = {}) {
  if (Boolean(file?.gameStateKnown)) {
    return {
      mode: "toggle",
      state: file?.gameEnabled ? "enabled" : "disabled",
      options: [],
    };
  }

  return {
    mode: "unrecorded",
    state: "unknown",
    options: getUnrecordedGameStateOptions(file),
  };
}
