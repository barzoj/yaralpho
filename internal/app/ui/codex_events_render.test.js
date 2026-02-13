const assert = require("node:assert");
const test = require("node:test");

class FakeClassList {
  constructor(el) {
    this.el = el;
    this.values = new Set();
  }
  add(...names) {
    names.forEach((n) => this.values.add(String(n)));
    this.el.className = Array.from(this.values).join(" ");
  }
  remove(...names) {
    names.forEach((n) => this.values.delete(String(n)));
    this.el.className = Array.from(this.values).join(" ");
  }
}

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.textContent = "";
    this.href = "";
    this.className = "";
    this.classList = new FakeClassList(this);
    this.style = {};
    this.hidden = false;
  }

  appendChild(child) {
    if (child) this.children.push(child);
    return child;
  }

  setAttribute() {}
  addEventListener() {}
  querySelector() {
    return null;
  }
}

class FakeDocument {
  constructor() {
    this._byId = new Map();
  }

  createElement(tag) {
    return new FakeElement(tag);
  }

  createTextNode(text) {
    const el = new FakeElement("#text");
    el.textContent = String(text);
    return el;
  }

  getElementById(id) {
    if (!this._byId.has(id)) {
      this._byId.set(id, new FakeElement("div"));
    }
    return this._byId.get(id);
  }

  addEventListener() {}
}

function flattenText(node) {
  const out = [];
  if (!node) return out;
  if (typeof node.textContent === "string" && node.textContent) {
    out.push(node.textContent);
  }
  for (const child of node.children || []) {
    out.push(...flattenText(child));
  }
  return out;
}

test("renders codex event types with dedicated labels and content", () => {
  global.Node = FakeElement;
  global.document = new FakeDocument();
  ["status", "content", "view-title", "breadcrumbs"].forEach((id) => {
    global.document._byId.set(id, new FakeElement("div"));
  });
  global.window = {
    location: { search: "" },
    addEventListener() {},
    removeEventListener() {},
  };

  delete require.cache[require.resolve("./app.js")];
  const { EventRender } = require("./app.js");
  const { renderEventsList, getEventMeta } = EventRender;

  assert.equal(getEventMeta("thread.started").label, "Thread started");
  assert.equal(getEventMeta("item.completed").label, "Item completed");

  const events = [
    {
      ingested_at: "2026-02-13T00:00:00Z",
      event: { type: "thread.started", thread_id: "thr-1" },
    },
    {
      ingested_at: "2026-02-13T00:00:01Z",
      event: { type: "turn.started" },
    },
    {
      ingested_at: "2026-02-13T00:00:02Z",
      event: {
        type: "item.started",
        item: {
          type: "command_execution",
          command: "go test ./...",
          status: "in_progress",
        },
      },
    },
    {
      ingested_at: "2026-02-13T00:00:03Z",
      event: {
        type: "item.completed",
        item: {
          type: "agent_message",
          text: "Implemented fix",
          status: "completed",
        },
      },
    },
    {
      ingested_at: "2026-02-13T00:00:04Z",
      event: {
        type: "turn.completed",
        usage: {
          input_tokens: 10,
          output_tokens: 5,
          cached_input_tokens: 2,
        },
      },
    },
  ];

  const list = renderEventsList(events);
  const text = flattenText(list).join("\n");

  assert.match(text, /Thread started/);
  assert.match(text, /Item started/);
  assert.match(text, /Item completed/);
  assert.match(text, /Turn completed/);
  assert.match(text, /Implemented fix/);
  assert.match(text, /go test \.\/\.\.\./);
});
