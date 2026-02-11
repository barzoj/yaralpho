const assert = require("node:assert");
const test = require("node:test");

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.textContent = "";
    this.href = "";
    this.className = "";
  }

  appendChild(child) {
    if (child) {
      this.children.push(child);
    }
    return child;
  }
}

class FakeDocument {
  constructor() {
    this._byId = new Map();
  }

  createElement(tag) {
    return new FakeElement(tag);
  }

  getElementById(id) {
    if (!this._byId.has(id)) {
      this._byId.set(id, new FakeElement("div"));
    }
    return this._byId.get(id);
  }

  addEventListener() {
    // no-op for tests
  }
}

test("buildRunsTable renders task ref and total events columns", () => {
  global.Node = FakeElement;
  global.document = new FakeDocument();

  // ensure required elements exist for module init
  ["status", "content", "view-title", "breadcrumbs"].forEach((id) => {
    global.document._byId.set(id, new FakeElement("div"));
  });

  global.window = {
    location: { search: "" },
    addEventListener() {},
    removeEventListener() {},
  };

  delete require.cache[require.resolve("./app.js")];
  const { buildRunsTable } = require("./app.js").RunList;

  const table = buildRunsTable("batch-1", [
    {
      run_id: "run-1",
      task_ref: "task-123",
      status: "succeeded",
      total_events: 4,
      started_at: "2026-02-11T10:00:00Z",
      finished_at: "2026-02-11T10:05:00Z",
    },
  ]);

  const [thead, tbody] = table.children;
  assert.ok(thead, "table has header");
  assert.ok(tbody, "table has body");

  const headerCells = (thead.children[0]?.children || []).map((cell) => cell.textContent);
  assert.deepStrictEqual(headerCells, ["Run", "Task Ref", "Status", "Total Events", "Started", "Finished"]);

  const firstRowCells = (tbody.children[0]?.children || []);
  assert.strictEqual(firstRowCells[0]?.children[0]?.textContent, "run-1");
  assert.strictEqual(firstRowCells[1]?.textContent, "task-123");
  assert.strictEqual(firstRowCells[2]?.textContent, "succeeded");
  assert.strictEqual(firstRowCells[3]?.textContent, 4);
  assert.ok(firstRowCells[4]?.textContent, "started_at rendered");
  assert.ok(firstRowCells[5]?.textContent, "finished_at rendered");
});
