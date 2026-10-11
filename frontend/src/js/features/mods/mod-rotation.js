import { showError, showNotification } from "../../core/toast.js";
import { showLoadingScreen, showMainScreen, updateLoadingMessage } from "../state.js";
import { getConfig, saveConfig } from "../../core/config.js";
import { refreshFilesKeepFilter } from "../file-list/filters.js";
import { showConfirmModal } from "../modals/confirm.js";
import { GetModRotation, ManualRotateMods, SetModRotation } from "../../../../wailsjs/go/app/App";
import { escapeHtml } from "../../core/utils.js";
import { t } from "../../core/i18n-runtime.js";

const ROTATION_ICON_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="icon-svg">
  <path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"></path>
  <path d="M3 3v5h5"></path>
  <path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"></path>
  <path d="M16 21h5v-5"></path>
</svg>`;

export async function manualRotate(type) {
  try {
    document.getElementById("confirm-modal")?.classList.add("hidden");
    showLoadingScreen();

    let config = {
      enableCharacters: false,
      enableWeapons: false,
    };

    let typeName = "";
    if (type === "weapon") {
      config.enableWeapons = true;
      typeName = t("mods.rotation.type.weapon");
    } else if (type === "character") {
      config.enableCharacters = true;
      typeName = t("mods.rotation.type.character");
    } else {
      config.enableCharacters = true;
      config.enableWeapons = true;
      typeName = t("mods.rotation.type.all");
    }

    updateLoadingMessage(t("mods.rotation.loadingMessage", { type: typeName }));
    await ManualRotateMods(config);
    await refreshFilesKeepFilter();

    showMainScreen();
    showNotification(t("mods.rotation.completedMessage", { type: typeName }), "success");
  } catch (e) {
    showMainScreen();
    console.error("手动轮换失败:", e);
  }
}

export async function initModRotationState() {
  if (typeof GetModRotation !== "function") {
    console.warn("后端 GetModRotation 方法不可用");
    return;
  }

  const config = getConfig();
  let rotationConfig = config.modRotationConfig;

  if (!rotationConfig) {
    const enabled = config.modRotationEnabled || false;
    rotationConfig = {
      enableCharacters: enabled,
      enableWeapons: enabled,
    };
  }

  try {
    rotationConfig = await GetModRotation();
    config.modRotationConfig = rotationConfig;
    saveConfig(config);
    updateModRotationUI(rotationConfig);
  } catch (e) {
    console.error("初始化Mod轮换状态失败:", e);
  }
}

export function updateModRotationUI(config) {
  const btn = document.getElementById("mod-rotation-btn");
  if (!btn) return;

  const charEnabled = config.enableCharacters;
  const weaponEnabled = config.enableWeapons;
  const customCount = Array.isArray(config.tags) ? config.tags.length : 0;
  const anyEnabled = charEnabled || weaponEnabled || customCount > 0;

  if (anyEnabled) {
    btn.classList.add("btn-rotation-enabled");
    btn.classList.remove("btn-outline");

    let text = t("mods.rotation.btn.enabled");
    if (charEnabled && weaponEnabled) {
      text = t("mods.rotation.btn.enabled");
    } else if (charEnabled) {
      text = t("mods.rotation.btn.characterEnabled");
    } else if (weaponEnabled) {
      text = t("mods.rotation.btn.weaponEnabled");
    } else if (customCount > 0) {
      text = t("mods.rotation.btn.categoryEnabled", { count: customCount });
    }

    btn.innerHTML = `<span class="icon">${ROTATION_ICON_SVG}</span> ${text}`;
  } else {
    btn.classList.remove("btn-rotation-enabled");
    btn.classList.add("btn-outline");
    btn.innerHTML = `<span class="icon">${ROTATION_ICON_SVG}</span> ${t("mods.rotation.btn.disabled")}`;
  }
}

export async function toggleModRotation() {
  if (typeof SetModRotation !== "function") {
    showError(t("mods.rotation.error.backendUnavailable"));
    return;
  }

  const config = getConfig();
  let currentConfig = config.modRotationConfig;
  if (!currentConfig) {
    const enabled = config.modRotationEnabled || false;
    currentConfig = {
      enableCharacters: enabled,
      enableWeapons: enabled,
    };
  }

  const htmlContent = `
    <div class="rotation-settings">
      <p class="rotation-title">${t("mods.rotation.modal.prompt")}</p>
      <div class="rotation-options">
        <label class="rotation-option-item">
          <span class="option-label">${t("mods.rotation.modal.characters")}</span>
          <div class="rotation-switch">
            <input type="checkbox" id="rotation-char-check" ${currentConfig.enableCharacters ? "checked" : ""}>
            <span class="rotation-slider round"></span>
          </div>
        </label>
        <label class="rotation-option-item">
          <span class="option-label">${t("mods.rotation.modal.weapons")}</span>
          <div class="rotation-switch">
            <input type="checkbox" id="rotation-weapon-check" ${currentConfig.enableWeapons ? "checked" : ""}>
            <span class="rotation-slider round"></span>
          </div>
        </label>
        <label class="rotation-option-item" style="display:block">
          <span class="option-label">${t("mods.rotation.modal.byCategory")}</span>
          <input type="text" id="rotation-tags-input" class="form-input" style="margin-top:6px"
                 placeholder="${t("mods.rotation.modal.tagsPlaceholder")}"
                 value="${escapeHtml((currentConfig.tags || []).join(","))}">
          <div style="font-size:0.8em;color:var(--text-tertiary);margin-top:4px">
            ${t("mods.rotation.modal.tagsHint")}
          </div>
        </label>
        <div style="margin-top: 8px; border-top: 1px solid var(--border-light); padding-top: 12px; display: flex; gap: 10px;">
           <button onclick="window.manualRotate('character')" class="btn btn-outline btn-small" style="flex: 1; justify-content: center;">
             <span class="icon">
               <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="icon-svg"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path><circle cx="12" cy="7" r="4"></circle></svg>
             </span> ${t("mods.rotation.modal.rotateCharacters")}
           </button>
           <button onclick="window.manualRotate('weapon')" class="btn btn-outline btn-small" style="flex: 1; justify-content: center;">
             <span class="icon">
               <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="icon-svg"><circle cx="12" cy="12" r="10"></circle><line x1="22" y1="12" x2="18" y2="12"></line><line x1="6" y1="12" x2="2" y2="12"></line><line x1="12" y1="6" x2="12" y2="2"></line><line x1="12" y1="22" x2="12" y2="18"></line></svg>
             </span> ${t("mods.rotation.modal.rotateWeapons")}
           </button>
        </div>
        <div style="font-size: 0.85em; color: var(--text-tertiary); margin-top: 6px; text-align: center;">${t("mods.rotation.modal.manualHint")}</div>
      </div>
      <div class="rotation-desc-container">
        <p>${t("mods.rotation.modal.desc1")}</p>
        <p>${t("mods.rotation.modal.desc2")}</p>
        <p><strong>${t("mods.rotation.modal.desc3")}</strong></p>
        <p>${t("mods.rotation.modal.desc4")}</p>
      </div>
    </div>
  `;

  showConfirmModal(
    t("mods.rotation.modal.title"),
    htmlContent,
    async () => {
      const charCheck = document.getElementById("rotation-char-check");
      const weaponCheck = document.getElementById("rotation-weapon-check");
      const tagsInput = document.getElementById("rotation-tags-input");
      const tags = String(tagsInput?.value || "")
        .split(/[,，、]/)
        .map((tag) => tag.trim())
        .filter((tag, index, list) => tag && list.indexOf(tag) === index);

      const newConfig = {
        enableCharacters: charCheck ? charCheck.checked : false,
        enableWeapons: weaponCheck ? weaponCheck.checked : false,
        tags,
      };

      try {
        config.modRotationConfig = newConfig;
        config.modRotationEnabled =
          newConfig.enableCharacters || newConfig.enableWeapons || tags.length > 0;
        saveConfig(config);
        await SetModRotation(newConfig);
        updateModRotationUI(newConfig);

        if (newConfig.enableCharacters || newConfig.enableWeapons || tags.length > 0) {
          showNotification(t("mods.rotation.notification.updated"), "success");
        } else {
          showNotification(t("mods.rotation.notification.disabled"), "info");
        }
      } catch (e) {
        showError(t("mods.rotation.error.operationFailed", { error: e }));
      }
    },
    true
  );
}
