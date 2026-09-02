const UNRECORDED_GAME_STATE_ACTIONS = Object.freeze([
  Object.freeze({
    id: "game-disabled",
    label: "游戏内关闭",
    description: "写入 addonlist.txt 为 0，文件仍保留在 addons 中。",
    disabled: false,
  }),
  Object.freeze({
    id: "game-enabled",
    label: "游戏内启用",
    description: "写入 addonlist.txt 为 1，文件仍保留在 addons 中。",
    disabled: false,
  }),
  Object.freeze({
    id: "disabled",
    label: "禁用",
    description: "移动到 disabled 目录，并从 addonlist.txt 中移除。",
    disabled: false,
  }),
]);

export function getUnrecordedGameStateOptions(file = {}) {
  const location = String(file?.location || "").trim().toLowerCase();
  return UNRECORDED_GAME_STATE_ACTIONS.map((option) => {
    if (option.id !== "disabled" || location !== "workshop") {
      return { ...option };
    }
    return {
      ...option,
      disabled: true,
      disabledReason: "workshop 文件不能直接禁用，请先复制到 addons。",
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
