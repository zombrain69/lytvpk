import { GetVPKPreviewImage } from "../../../../wailsjs/go/app/App";

const MAX_PREVIEW_ENTRIES = 96;
const MAX_PREVIEW_BYTES = 32 * 1024 * 1024;
const MAX_CONCURRENT_PREVIEW_LOADS = 2;
const MAX_CARD_PREVIEW_ENTRIES = 192;
const MAX_CARD_PREVIEW_BYTES = 16 * 1024 * 1024;
const CARD_PREVIEW_MAX_WIDTH = 640;
const CARD_PREVIEW_MAX_HEIGHT = 360;
const CARD_PREVIEW_CONVERT_BYTES = 768 * 1024;

// Map insertion order is the LRU order: the newest entry is always last.
const previewCache = new Map();
const previewRequests = new Map();
const previewQueue = [];
const cardPreviewCache = new Map();
const cardPreviewRequests = new Map();
let previewCacheBytes = 0;
let activePreviewLoads = 0;
let cardPreviewCacheBytes = 0;

function getPreviewKey(file) {
	const revision = String(file?.previewRevision || "").trim();
	if (revision) return revision;

	const path = String(file?.path || "");
	const name = String(file?.name || "");
	const size = String(file?.size || "");
	const modified = String(file?.lastModified || "");
	return `${name}\u0000${size}\u0000${modified}\u0000${path}`;
}

function estimatePreviewBytes(data) {
  // Preview data is an ASCII Base64 string. JavaScript engines typically store
  // it as UTF-16, so use a conservative two-byte estimate for cache budgeting.
  return String(data || "").length * 2;
}

function touchEntry(cache, key, entry) {
  cache.delete(key);
  cache.set(key, entry);
}

function putPreview(key, data) {
  const previous = previewCache.get(key);
  if (previous) previewCacheBytes -= previous.bytes;

  const entry = { data, bytes: estimatePreviewBytes(data) };
  previewCache.set(key, entry);
  previewCacheBytes += entry.bytes;

  while (
    previewCache.size > MAX_PREVIEW_ENTRIES ||
    previewCacheBytes > MAX_PREVIEW_BYTES
  ) {
    const oldest = previewCache.entries().next().value;
    if (!oldest) break;
    const [oldestKey, oldestEntry] = oldest;
    previewCache.delete(oldestKey);
    previewCacheBytes -= oldestEntry.bytes;
  }
}

// undefined means "not requested yet"; an empty string is a remembered
// no-preview result and prevents repeated archive reads for the same VPK.
export function getCachedVPKPreview(file) {
  const key = getPreviewKey(file);
  const entry = previewCache.get(key);
  if (!entry) return undefined;
  touchEntry(previewCache, key, entry);
  return entry.data;
}

// Card previews are deliberately separate from the full preview cache. Detail
// dialogs still receive the original animated/high-resolution image, while the
// grid keeps a small static thumbnail that avoids dozens of simultaneous GIF
// animation loops and oversized texture uploads.
export function getCachedVPKCardPreview(file) {
  const key = getPreviewKey(file);
  const entry = cardPreviewCache.get(key);
  if (!entry) return undefined;
  touchEntry(cardPreviewCache, key, entry);
  return entry.data;
}

function putCardPreview(key, data) {
  const previous = cardPreviewCache.get(key);
  if (previous) cardPreviewCacheBytes -= previous.bytes;
  const entry = { data, bytes: estimatePreviewBytes(data) };
  cardPreviewCache.set(key, entry);
  cardPreviewCacheBytes += entry.bytes;
  while (
    cardPreviewCache.size > MAX_CARD_PREVIEW_ENTRIES ||
    cardPreviewCacheBytes > MAX_CARD_PREVIEW_BYTES
  ) {
    const oldest = cardPreviewCache.entries().next().value;
    if (!oldest) break;
    const [oldestKey, oldestEntry] = oldest;
    cardPreviewCache.delete(oldestKey);
    cardPreviewCacheBytes -= oldestEntry.bytes;
  }
}

export async function loadVPKPreview(file) {
  return loadVPKPreviewWithOptions(file);
}

export async function loadVPKCardPreview(file) {
  const key = getPreviewKey(file);
  const cached = getCachedVPKCardPreview(file);
  if (cached !== undefined) return cached;

  let request = cardPreviewRequests.get(key);
  if (!request) {
    request = loadVPKPreview(file)
      .then((data) => createCardPreviewData(data))
      .then((data) => {
        putCardPreview(key, data);
        return data;
      })
      .finally(() => cardPreviewRequests.delete(key));
    cardPreviewRequests.set(key, request);
  }
  return request;
}

function createCardPreviewData(data) {
  const source = String(data || "");
  if (!source || typeof document === "undefined") return Promise.resolve(source);

  const mime = source.match(/^data:([^;,]+)/i)?.[1]?.toLowerCase() || "";
  const isAnimatedCandidate = mime === "image/gif";
  const shouldDownsample = isAnimatedCandidate || source.length >= CARD_PREVIEW_CONVERT_BYTES;
  if (!shouldDownsample) return Promise.resolve(source);

  return new Promise((resolve) => {
    const image = new Image();
    image.decoding = "async";
    image.onload = () => {
      const width = image.naturalWidth || image.width;
      const height = image.naturalHeight || image.height;
      if (!width || !height) {
        resolve(source);
        return;
      }
      const scale = Math.min(1, CARD_PREVIEW_MAX_WIDTH / width, CARD_PREVIEW_MAX_HEIGHT / height);
      if (!isAnimatedCandidate && scale >= 1) {
        resolve(source);
        return;
      }

      const canvas = document.createElement("canvas");
      canvas.width = Math.max(1, Math.round(width * scale));
      canvas.height = Math.max(1, Math.round(height * scale));
      const context = canvas.getContext("2d", { alpha: true });
      if (!context) {
        resolve(source);
        return;
      }
      context.drawImage(image, 0, 0, canvas.width, canvas.height);
      // WebP preserves transparency and is supported by the bundled Chromium;
      // browsers that reject it return a PNG data URL, which remains valid.
      const thumbnail = canvas.toDataURL("image/webp", 0.82);
      resolve(thumbnail && thumbnail.length < source.length ? thumbnail : source);
    };
    image.onerror = () => resolve(source);
    image.src = source;
  });
}

// priority=true 用于详情页等用户主动请求，避免它排在列表卡片的预览请求之后。
// 相同 VPK 的并发请求仍共享同一个 Promise，不会重复读取归档。
export async function loadVPKPreviewWithOptions(file, { priority = false } = {}) {
  const key = getPreviewKey(file);
  const cached = getCachedVPKPreview(file);
  if (cached !== undefined) return cached;

  let request = previewRequests.get(key);
  if (!request) {
    request = new Promise((resolve, reject) => {
      const task = { file, key, resolve, reject };
      if (priority) previewQueue.unshift(task);
      else previewQueue.push(task);
      runNextPreviewLoad();
    });
    previewRequests.set(key, request);
  } else if (priority) {
    // 卡片请求可能已经排队；详情是用户主动操作，应把同一任务提升到队首。
    const queuedIndex = previewQueue.findIndex((task) => task.key === key);
    if (queuedIndex > 0) {
      const [queuedTask] = previewQueue.splice(queuedIndex, 1);
      previewQueue.unshift(queuedTask);
      runNextPreviewLoad();
    }
  }
  return request;
}

function runNextPreviewLoad() {
  while (
    activePreviewLoads < MAX_CONCURRENT_PREVIEW_LOADS &&
    previewQueue.length > 0
  ) {
    const next = previewQueue.shift();
    activePreviewLoads += 1;
    GetVPKPreviewImage(next.file.path)
      .then((data) => String(data || ""))
      .then((data) => {
        putPreview(next.key, data);
        next.resolve(data);
      })
      .catch(next.reject)
      .finally(() => {
        previewRequests.delete(next.key);
        activePreviewLoads -= 1;
        runNextPreviewLoad();
      });
  }
}
