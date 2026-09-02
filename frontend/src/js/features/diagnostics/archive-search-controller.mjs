export function createImeAwareSearchController(render, options = {}) {
  const schedule = options.schedule || ((callback, delay) => window.setTimeout(callback, delay));
  const clear = options.clear || ((timerId) => window.clearTimeout(timerId));
  const delay = Number.isFinite(options.delay) ? options.delay : 120;

  let composing = false;
  let timerId = null;

  const cancel = () => {
    if (timerId !== null) {
      clear(timerId);
      timerId = null;
    }
  };

  const scheduleRender = () => {
    if (composing) return;
    cancel();
    timerId = schedule(() => {
      timerId = null;
      if (!composing) render();
    }, delay);
  };

  return {
    input: scheduleRender,
    compositionStart() {
      composing = true;
      cancel();
    },
    compositionEnd() {
      composing = false;
      scheduleRender();
    },
    cancel,
    get isComposing() {
      return composing;
    },
  };
}
