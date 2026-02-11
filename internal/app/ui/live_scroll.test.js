const assert = require("node:assert");
const test = require("node:test");

global.document =
  global.document ||
  ({
    getElementById() {
      return null;
    },
    addEventListener() {},
    querySelector() {
      return null;
    },
  });
global.window =
  global.window ||
  ({
    location: { search: "" },
    addEventListener() {},
    removeEventListener() {},
  });

delete require.cache[require.resolve("./app.js")];
const { createScrollFollower, isNearBottom } = require("./app.js").RunLayout || global.RunLayout;

function makeScrollElement({ clientHeight = 200, scrollHeight = 600, scrollTop = 0 } = {}) {
  const listeners = {};
  const el = {
    clientHeight,
    scrollHeight,
    scrollTop,
    addEventListener(event, fn) {
      listeners[event] = listeners[event] || [];
      listeners[event].push(fn);
    },
    removeEventListener(event, fn) {
      if (!listeners[event]) return;
      listeners[event] = listeners[event].filter((f) => f !== fn);
    },
    emit(event) {
      (listeners[event] || []).forEach((fn) => fn());
    },
  };
  return el;
}

test("isNearBottom respects threshold and auto-follow scrolls to bottom", () => {
  const el = makeScrollElement({ clientHeight: 200, scrollHeight: 600, scrollTop: 390 });
  assert.equal(isNearBottom(el, 20), true);
  el.scrollTop = 330;
  assert.equal(isNearBottom(el, 20), false);
  el.scrollTop = 400;

  const modes = [];
  const follower = createScrollFollower(el, {
    thresholdPx: 30,
    onModeChange: (mode) => modes.push(mode),
  });

  assert.equal(follower.getMode(), "follow");
  el.scrollHeight = 800;
  el.scrollTop = 500;
  follower.handleContentMutated();
  assert.equal(el.scrollTop, el.scrollHeight);
  follower.cleanup();
});

test("scroll follower pauses on manual scroll and resumes at bottom", () => {
  const el = makeScrollElement({ clientHeight: 200, scrollHeight: 600, scrollTop: 420 });
  const follower = createScrollFollower(el, { thresholdPx: 10 });

  el.scrollTop = 100;
  el.emit("scroll");
  assert.equal(follower.getMode(), "paused");

  const before = el.scrollTop;
  el.scrollHeight = 900;
  follower.handleContentMutated();
  assert.equal(el.scrollTop, before);

  el.scrollTop = el.scrollHeight - el.clientHeight;
  el.emit("scroll");
  assert.equal(follower.getMode(), "follow");

  el.scrollHeight = 950;
  follower.handleContentMutated();
  assert.equal(el.scrollTop, el.scrollHeight);
  follower.cleanup();
});
