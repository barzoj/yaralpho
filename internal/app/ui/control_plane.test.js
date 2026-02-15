const assert = require("node:assert");
const test = require("node:test");

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.textContent = "";
    this.href = "";
    this.className = "";
    this.style = {};
    this.attributes = new Map();
    this._innerHTML = "";
    this.value = "";
    this.disabled = false;
    this.listeners = {};
  }

  set innerHTML(value) {
    this._innerHTML = value;
    this.children = [];
  }

  get innerHTML() {
    return this._innerHTML;
  }

  appendChild(child) {
    if (child) {
      this.children.push(child);
    }
    return child;
  }

  replaceChildren(...nodes) {
    this.children = [];
    nodes.forEach((n) => this.appendChild(n));
  }

  setAttribute(name, value) {
    this.attributes.set(name, value);
  }

  getAttribute(name) {
    return this.attributes.get(name);
  }

  addEventListener(event, handler) {
    if (!this.listeners[event]) this.listeners[event] = [];
    this.listeners[event].push(handler);
  }

  async trigger(event) {
    const handlers = this.listeners[event] || [];
    for (const handler of handlers) {
      await handler({
        preventDefault() {},
        stopPropagation() {},
      });
    }
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
    // no-op for fake DOM
  }
}

function setupDom({ hash = "#/control-plane" } = {}) {
  global.Node = FakeElement;
  global.document = new FakeDocument();
  ["status", "content", "view-title", "breadcrumbs", "nav"].forEach((id) => {
    document._byId.set(id, new FakeElement("div"));
  });
  global.window = {
    location: { hash, search: "", protocol: "http:", host: "localhost" },
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() {},
    history: { pushState() {} },
  };
}

function loadModule() {
  delete require.cache[require.resolve("./app.js")];
  return require("./app.js");
}

function mockJsonResponse(body, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (name) => (name && name.toLowerCase() === "content-type" ? "application/json" : "") },
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(typeof body === "string" ? body : JSON.stringify(body)),
  });
}

function collectText(node) {
  if (!node) return "";
  let text =
    typeof node.textContent === "string"
      ? node.textContent
      : String(node.textContent ?? "");
  for (const child of node.children || []) {
    text += ` ${collectText(child)}`;
  }
  return text.trim();
}

function findFirstTag(node, tagName) {
  if (!node) return null;
  if (node.tagName === tagName.toUpperCase()) return node;
  for (const child of node.children || []) {
    const found = findFirstTag(child, tagName);
    if (found) return found;
  }
  return null;
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

test("control plane renders repositories, batches, and guards restart", async () => {
  setupDom({ hash: "#/control-plane" });
  const repos = [{ repository_id: "repo1", name: "Repo One" }];
  const batches = [
    { batch_id: "b-failed", repository_id: "repo1", status: "failed" },
    { batch_id: "b-done", repository_id: "repo1", status: "completed" },
  ];
  const batchDetails = {
    "b-failed": {
      batch_id: "b-failed",
      repository_id: "repo1",
      status: "failed",
      items: [
        { input: "t1", status: "failed", attempts: 1 },
        { input: "t2", status: "pending", attempts: 0 },
      ],
    },
    "b-done": {
      batch_id: "b-done",
      repository_id: "repo1",
      status: "completed",
      items: [{ input: "done", status: "completed", attempts: 1 }],
    },
  };

  global.fetch = (url, options = {}) => {
    const method = options.method || "GET";
    if (url === "/repository" && method === "GET") return mockJsonResponse(repos);
    if (url === "/repository/repo1/batches?limit=50" && method === "GET") {
      return mockJsonResponse({ batches, count: batches.length });
    }
    if (url === "/batches/b-failed" && method === "GET") return mockJsonResponse(batchDetails["b-failed"]);
    if (url === "/batches/b-done" && method === "GET") return mockJsonResponse(batchDetails["b-done"]);
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { ControlPlaneView } = loadModule();
  await ControlPlaneView.routeApp();
  await flush();
  await flush();

  const nav = document.getElementById("nav");
  const navActive = nav.children.find((child) => child.getAttribute && child.getAttribute("aria-current") === "page");
  assert.ok(navActive, "Control Plane link should be active");

  const table = findFirstTag(document.getElementById("content"), "TABLE");
  assert.ok(table, "control plane table rendered");
  const tbody = table.children.find((el) => el.tagName === "TBODY");
  assert.strictEqual(tbody.children.length, 2);

  const [failedRow, doneRow] = tbody.children;
  assert.match(collectText(failedRow.children[0]), /b-failed/);
  assert.match(collectText(doneRow.children[0]), /b-done/);

  const failedActions = failedRow.children[3];
  const restartBtn = findFirstTag(failedActions, "BUTTON");
  assert.ok(restartBtn, "failed batch has restart button");

  const doneActions = doneRow.children[3];
  const pill = findFirstTag(doneActions, "DIV");
  assert.ok(pill, "non-failed batch shows guard pill");
  assert.match(collectText(doneActions).toLowerCase(), /failed batches only/);
  assert.ok(!doneActions.children.some((c) => c.textContent === "Restart"), "no restart button for completed batch");

  const tasksCell = failedRow.children[1];
  const toggle = findFirstTag(tasksCell, "BUTTON");
  assert.ok(toggle, "tasks toggle exists");
  await toggle.trigger("click");
  const innerTable = findFirstTag(tasksCell, "TABLE");
  assert.ok(innerTable, "tasks table rendered");
  const tasksBody = innerTable.children.find((el) => el.tagName === "TBODY");
  assert.strictEqual(tasksBody.children.length, 2, "renders all tasks");
  const statuses = collectText(tasksBody);
  assert.match(statuses, /failed/i);
  assert.match(statuses, /pending/i);
});

test("restart refreshes batch status and clears task cache", async () => {
  setupDom({ hash: "#/control-plane" });
  const repos = [{ repository_id: "repo1", name: "Repo One" }];
  let batches = [{ batch_id: "b1", repository_id: "repo1", status: "failed" }];
  let batchDetail = {
    batch_id: "b1",
    repository_id: "repo1",
    status: "failed",
    items: [{ input: "old", status: "failed", attempts: 1 }],
  };
  let lastBatchesResponse = [];
  const calls = [];

  global.fetch = (url, options = {}) => {
    const method = options.method || "GET";
    calls.push({ url, method });
    if (url === "/repository" && method === "GET") return mockJsonResponse(repos);
    if (url === "/repository/repo1/batches?limit=50" && method === "GET") {
      lastBatchesResponse = batches.map((b) => ({ ...b }));
      return mockJsonResponse({ batches, count: batches.length });
    }
    if (url === "/batches/b1" && method === "GET") return mockJsonResponse(batchDetail);
    if (url === "/repository/repo1/batch/b1/restart" && method === "PUT") {
      batches = [{ batch_id: "b1", repository_id: "repo1", status: "pending" }];
      batchDetail = {
        batch_id: "b1",
        repository_id: "repo1",
        status: "pending",
        items: [{ input: "old", status: "queued", attempts: 2 }],
      };
      return mockJsonResponse({ batch_id: "b1", status: "pending", repository_id: "repo1" });
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { ControlPlaneView } = loadModule();
  await ControlPlaneView.routeApp();
  await flush();
  await flush();

  const table = findFirstTag(document.getElementById("content"), "TABLE");
  const tbody = table.children.find((el) => el.tagName === "TBODY");
  const row = tbody.children[0];
  const actions = row.children[3];
  const restartBtn = findFirstTag(actions, "BUTTON");
  assert.ok(restartBtn, "restart button present");

  const tasksCell = row.children[1];
  const toggle = findFirstTag(tasksCell, "BUTTON");
  await toggle.trigger("click");
  await flush();
  let innerTable = findFirstTag(tasksCell, "TABLE");
  let tasksBody = innerTable.children.find((el) => el.tagName === "TBODY");
  assert.match(collectText(tasksBody), /failed/i, "initial task status shown");

  await restartBtn.trigger("click");
  await flush();
  await flush();
  assert.strictEqual(batches[0].status, "pending", "mock batches updated");
  assert.strictEqual(
    lastBatchesResponse[0]?.status,
    "pending",
    "latest batches response reflects restart"
  );

  const updatedTable = findFirstTag(document.getElementById("content"), "TABLE");
  const updatedBody = updatedTable.children.find((el) => el.tagName === "TBODY");
  const updatedRow = updatedBody.children[0];
  const batchGets = calls.filter(
    (c) => c.url === "/repository/repo1/batches?limit=50" && c.method === "GET"
  );
  assert.ok(batchGets.length >= 2, "batches refetched after restart");
  const restartIndex = calls.findIndex(
    (c) => c.method === "PUT" && c.url === "/repository/repo1/batch/b1/restart"
  );
  const lastBatchGetIndex = calls.findLastIndex
    ? calls.findLastIndex(
        (c) => c.method === "GET" && c.url === "/repository/repo1/batches?limit=50"
      )
    : (() => {
        let idx = -1;
        calls.forEach((c, i) => {
          if (c.method === "GET" && c.url === "/repository/repo1/batches?limit=50") idx = i;
        });
        return idx;
      })();
  assert.ok(
    lastBatchGetIndex > restartIndex,
    "batches refetch happens after restart call"
  );
  const statusCellText = collectText(updatedRow.children[2]);
  assert.match(statusCellText.toLowerCase(), /pending/, "status updated after restart");
  const updatedTasksCell = updatedRow.children[1];
  const updatedToggle = findFirstTag(updatedTasksCell, "BUTTON");
  await updatedToggle.trigger("click");
  await flush();
  await flush();
  await flush();
  const detailGets = calls.filter(
    (c) => c.url === "/batches/b1" && c.method === "GET"
  );
  assert.ok(detailGets.length >= 2, "batch detail refetched after restart");
  innerTable = findFirstTag(updatedTasksCell, "TABLE");
  tasksBody = innerTable.children.find((el) => el.tagName === "TBODY");
  const statuses = collectText(tasksBody);
  assert.match(statuses, /queued/i, "tasks refreshed after restart");
  assert.ok(!/failed/i.test(statuses), "stale task status cleared");

  const restartCalls = calls.filter((c) => c.method === "PUT");
  assert.strictEqual(restartCalls.length, 1, "restart invoked once");
});
