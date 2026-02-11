const assert = require("node:assert");
const test = require("node:test");

const {
  mergeLiveEnvelope,
  eventKeyFromEvent,
  getIngestedAt,
  deriveLatestIngested,
} = require("./live_merge");

function baseState() {
  return {
    events: [],
    seenKeys: new Set(),
    cursor: "",
    totalCount: 0,
  };
}

const makeEvent = (ingested_at, overrides = {}) => ({
  batch_id: "b1",
  run_id: "r1",
  session_id: "s1",
  ingested_at,
  event: { type: "t" },
  ...overrides,
});

test("mergeLiveEnvelope appends in order and updates cursor", () => {
  const state = baseState();
  const first = mergeLiveEnvelope(state, {
    type: "event",
    cursor: "2024-01-01T00:00:02Z",
    event: makeEvent("2024-01-01T00:00:02Z"),
  });

  assert.equal(first.events.length, 1);
  assert.equal(first.cursor, "2024-01-01T00:00:02Z");
  assert.equal(first.totalCount, 1);
  assert.ok(first.changed);

  const second = mergeLiveEnvelope(first, {
    type: "event",
    event: makeEvent("2024-01-01T00:00:01Z"),
  });

  assert.equal(second.events.length, 2);
  assert.equal(getIngestedAt(second.events[0]), "2024-01-01T00:00:01Z");
  assert.equal(getIngestedAt(second.events[1]), "2024-01-01T00:00:02Z");
  assert.equal(second.totalCount, 2);
  assert.equal(second.cursor, "2024-01-01T00:00:02Z");
});

test("mergeLiveEnvelope dedupes by composite key", () => {
  const evt = makeEvent("2024-01-01T00:00:05Z");
  const state = {
    events: [evt],
    seenKeys: new Set([eventKeyFromEvent(evt)]),
    cursor: evt.ingested_at,
    totalCount: 1,
  };

  const next = mergeLiveEnvelope(state, { type: "event", event: { ...evt } });
  assert.equal(next.events.length, 1);
  assert.equal(next.totalCount, 1);
  assert.ok(!next.changed);
});

test("mergeLiveEnvelope handles out-of-order arrival", () => {
  const state = baseState();
  const c = mergeLiveEnvelope(state, {
    type: "event",
    event: makeEvent("2024-01-01T00:00:03Z"),
  });
  const d = mergeLiveEnvelope(c, {
    type: "event",
    event: makeEvent("2024-01-01T00:00:01Z"),
  });
  const e = mergeLiveEnvelope(d, {
    type: "event",
    event: makeEvent("2024-01-01T00:00:02Z"),
  });

  assert.deepEqual(
    e.events.map((evt) => getIngestedAt(evt)),
    [
      "2024-01-01T00:00:01Z",
      "2024-01-01T00:00:02Z",
      "2024-01-01T00:00:03Z",
    ]
  );
  assert.equal(e.totalCount, 3);
  assert.equal(e.cursor, "2024-01-01T00:00:03Z");
});

test("heartbeat only updates cursor", () => {
  const state = baseState();
  const next = mergeLiveEnvelope(state, { type: "heartbeat", cursor: "c1" });
  assert.equal(next.cursor, "c1");
  assert.ok(!next.changed);
});

test("deriveLatestIngested ignores missing timestamps", () => {
  const events = [
    makeEvent("", { ingestedAt: "" }),
    makeEvent("2024-01-02T00:00:00Z"),
    makeEvent("2024-01-01T00:00:00Z"),
  ];
  assert.equal(deriveLatestIngested(events), "2024-01-02T00:00:00Z");
});
