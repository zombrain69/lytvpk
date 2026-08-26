import { GetVPKPreviewImage } from "../../../../wailsjs/go/app/App";

const MAX_PREVIEW_ENTRIES = 96;
const MAX_PREVIEW_BYTES = 32 * 1024 * 1024;
const MAX_CONCURRENT_PREVIEW_LOADS = 2;

// Map insertion order is the LRU order: the newest entry is always last.
const previewCache = new Map();
const previewRequests = new Map();
const previewQueue = [];
let previewCacheBytes = 0;
let activePreviewLoads = 0;

function getPreviewKey(file) {
  const path = String(file?.path || "");
  const modified = String(file?.lastModified || "");
  return `${path}\u0000${modified}`;
}

function estimatePreviewBytes(data) {
  // Preview data is an ASCII Base64 string. JavaScript engines typically store
  // it as UTF-16, so use a conservative two-byte estimate for cache budgeting.
  return String(data || "").length * 2;
}

function touchEntry(key, entry) {
  previewCache.delete(key);
  previewCache.set(key, entry);
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
  touchEntry(key, entry);
  return entry.data;
}

export async function loadVPKPreview(file) {
  const key = getPreviewKey(file);
  const cached = getCachedVPKPreview(file);
  if (cached !== undefined) return cached;

  let request = previewRequests.get(key);
  if (!request) {
    request = new Promise((resolve, reject) => {
      previewQueue.push({ file, key, resolve, reject });
      runNextPreviewLoad();
    });
    previewRequests.set(key, request);
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
