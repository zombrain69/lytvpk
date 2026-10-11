import { buildVTFBytes } from "./vtf-core.js";

self.addEventListener("message", (event) => {
  try {
    const bytes = buildVTFBytes(event.data);
    self.postMessage({ ok: true, bytes }, [bytes.buffer]);
  } catch (error) {
    self.postMessage({
      ok: false,
      error: error?.message || String(error || ""),
      code: typeof error?.code === "string" ? error.code : "",
    });
  }
});
