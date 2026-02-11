const assert = require("node:assert");
const test = require("node:test");

const { computeBackoffDelay, createReconnectController } = require("./live_reconnect");

test("computeBackoffDelay grows exponentially with jitter and caps at max", () => {
  const delays = [];
  for (let attempt = 1; attempt <= 5; attempt += 1) {
    delays.push(
      computeBackoffDelay(attempt, {
        baseDelayMs: 100,
        maxDelayMs: 500,
        jitterFactor: 0,
        rand: () => 0,
      })
    );
  }

  assert.deepEqual(delays, [100, 200, 400, 500, 500]);
});

test("reconnect controller retries with status updates and stops after max attempts", async () => {
  const statuses = [];
  const scheduled = [];
  let retryCalls = 0;

  const controller = createReconnectController({
    maxAttempts: 2,
    baseDelayMs: 100,
    maxDelayMs: 200,
    jitterFactor: 0,
    rand: () => 0,
    setStatus: (msg) => statuses.push(msg),
    schedule: (fn, delay) => {
      scheduled.push(delay);
      return setTimeout(fn, 0);
    },
    onRetry: () => {
      retryCalls += 1;
      statuses.push("retrying-now");
    },
    onExhausted: (reason) => statuses.push(`exhausted:${reason || "none"}`),
  });

  controller.scheduleReconnect("first-drop");
  await new Promise((resolve) => setTimeout(resolve, 5));
  controller.scheduleReconnect("second-drop");
  await new Promise((resolve) => setTimeout(resolve, 5));
  // this one should exhaust and not schedule (timer already consumed twice)
  const result = controller.scheduleReconnect("still-down");

  await new Promise((resolve) => setTimeout(resolve, 10));

  assert.equal(result.scheduled, false);
  assert.deepEqual(scheduled, [100, 200]);
  assert.ok(retryCalls >= 2);
  assert.ok(statuses.some((msg) => msg.includes("retrying-now")));
  assert.ok(statuses.some((msg) => msg.startsWith("exhausted:")));
});
