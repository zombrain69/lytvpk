function countLines(value) {
  return Math.max(1, String(value || "").split(/\r\n|\r|\n/).length);
}

// Connects a plain textarea to a non-interactive gutter. Keeping wrapping off
// ensures one gutter entry always maps to one physical line in the cfg file.
export function attachLineNumberGutter(editor, gutter) {
  if (!editor || !gutter) return { refresh() {}, syncScroll() {} };

  const syncScroll = () => {
    gutter.scrollTop = editor.scrollTop;
  };
  const refresh = () => {
    const lineNumbers = Array.from({ length: countLines(editor.value) }, (_, index) => String(index + 1));
    gutter.textContent = lineNumbers.join("\n");
    syncScroll();
  };

  editor.addEventListener("input", refresh);
  editor.addEventListener("scroll", syncScroll);
  refresh();

  return { refresh, syncScroll };
}
