import assert from "node:assert/strict";
import test from "node:test";

import { createImeAwareSearchController } from "./archive-search-controller.mjs";

function createFakeClock() {
  let nextId = 0;
  const pending = new Map();
  return {
    schedule(callback) {
      const id = ++nextId;
      pending.set(id, callback);
      return id;
    },
    clear(id) {
      pending.delete(id);
    },
    flush() {
      const callbacks = [...pending.values()];
      pending.clear();
      callbacks.forEach((callback) => callback());
    },
    size() {
      return pending.size;
    },
  };
}

test("IME composition never renders the archive manager before compositionend", () => {
  const clock = createFakeClock();
  let renders = 0;
  const controller = createImeAwareSearchController(() => {
    renders += 1;
  }, {
    schedule: clock.schedule,
    clear: clock.clear,
  });

  controller.compositionStart();
  controller.input();
  clock.flush();

  assert.equal(renders, 0);
  assert.equal(clock.size(), 0);

  controller.compositionEnd();
  assert.equal(clock.size(), 1);
  clock.flush();
  assert.equal(renders, 1);
});

test("starting IME composition cancels a pending Latin-input render", () => {
  const clock = createFakeClock();
  let renders = 0;
  const controller = createImeAwareSearchController(() => {
    renders += 1;
  }, {
    schedule: clock.schedule,
    clear: clock.clear,
  });

  controller.input();
  assert.equal(clock.size(), 1);
  controller.compositionStart();
  clock.flush();

  assert.equal(renders, 0);
  assert.equal(clock.size(), 0);
});
