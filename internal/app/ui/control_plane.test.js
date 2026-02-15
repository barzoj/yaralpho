const assert = require("node:assert");
const test = require("node:test");

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.textContent = "";
    this.href = "";
    this.className = "";
    this.attributes = new Map();
    this.innerHTML = "";
    this.value = "";
    this.disabled = false;
    this.listeners = {};
    this.style = {};
  }

  appendChild(child) {
    if (child) {
      this.children.push(child);
      if (!this.value && this.tagName === "SELECT" && child.value) {
        this.value = child.value;
      }
    }
    return child;
  }

  replaceChildren(...nodes) {
    this.children = [];
    nodes.forEach((n) => this.appendChild(n));
  }

  replaceWith(node) {
    this.replacedWith = node;
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

  removeEventListener(event, handler) {
    if (!this.listeners[event]) return;
    this.listeners[event] = this.listeners[event].filter((fn) => fn !== handler);
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
    // no-op
  }
}

function setupDom({ search = "", hash = "" } = {}) {
  global.Node = FakeElement;
  global.document = new FakeDocument();
  ["status", "content", "view-title", "breadcrumbs", "nav"].forEach((id) => {
    document._byId.set(id, new FakeElement("div"));
  });
  global.window = {
    location: { search, hash, protocol: "http:", host: "localhost" },
    addEventListener() {},
    removeEventListener() {},
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
  let text = typeof node.textContent === "string" ? node.textContent : "";
  for (const child of node.children || []) {
    text += ` ${collectText(child)}`;
  }
  return text.trim();
}

function findAllTags(node, tagName, acc = []) {
  if (!node) return acc;
  if (node.tagName === tagName.toUpperCase()) acc.push(node);
  for (const child of node.children || []) {
    findAllTags(child, tagName, acc);
  }
  return acc;
}

function findFirst(node, predicate) {
  if (!node) return null;
  if (predicate(node)) return node;
  for (const child of node.children || []) {
    const found = findFirst(child, predicate);
    if (found) return found;
  }
  return null;
}

test("control plane renders repo batches and task list with scroll", async () => {
  setupDom({ hash: "#/control-plane" });
  const repos = [{ repository_id: "r1", name: "Repo One" }];
  const items = Array.from({ length: 120 }).map((_, idx) => ({
    input: `task-${idx + 1}`,
    status: idx % 2 === 0 ? "pending" : "failed",
    attempts: idx % 3,
  }));
  const batches = [
    {
      batch_id: "b1",
      repository_id: "r1",
      status: "failed",
      items,
    },
  ];

  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    if (url === "/repository" && method === "GET") {
      return mockJsonResponse(repos);
    }
    if (url.startsWith("/repository/r1/batches") && method === "GET") {
      return mockJsonResponse({ batches, count: batches.length });
    }
    if (url.startsWith("/batches/") && method === "GET") {
      return mockJsonResponse(batches[0]);
    }
    if (url.includes("/restart") && method === "PUT") {
      return mockJsonResponse({ batch_id: "b1", status: "pending", repository_id: "r1" });
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { ControlPlaneView } = loadModule();
  await ControlPlaneView.routeApp();

  const navText = collectText(document.getElementById("nav"));
  assert.match(navText, /Control Plane/);
  const contentText = collectText(document.getElementById("content"));
  assert.match(contentText, /Repo One/);
  assert.match(contentText, /b1/);

  const links = findAllTags(document.getElementById("content"), "A");
  assert.ok(
    links.some((l) => (l.href || "").includes("batch=b1")),
    "batch link points to runs view"
  );

  const scroll = findFirst(document.getElementById("content"), (node) => node.style?.overflowY === "auto");
  assert.ok(scroll, "tasks rendered in scrollable container");

  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /Loaded 1 repositories/);
});

test("restart is only enabled for failed batches and posts restart", async () => {
  setupDom({ hash: "#/control-plane" });
  const repos = [{ repository_id: "repo-1", name: "Repo One" }];
  const batches = [
    {
      batch_id: "b-failed",
      repository_id: "repo-1",
      status: "failed",
      items: [{ input: "t1", status: "failed", attempts: 1 }],
    },
    {
      batch_id: "b-pending",
      repository_id: "repo-1",
      status: "pending",
      items: [{ input: "t2", status: "pending", attempts: 0 }],
    },
  ];
  let restartCalls = 0;
  let afterRestart = false;

  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    if (url === "/repository" && method === "GET") {
      return mockJsonResponse(repos);
    }
    if (url.startsWith("/repository/repo-1/batches") && method === "GET") {
      const updated = afterRestart
        ? [{ ...batches[0], status: "pending" }, batches[1]]
        : batches;
      return mockJsonResponse({ batches: updated, count: updated.length });
    }
    if (url.startsWith("/repository/repo-1/batch/b-failed/restart") && method === "PUT") {
      restartCalls += 1;
      afterRestart = true;
      return mockJsonResponse({
        batch_id: "b-failed",
        status: "pending",
        repository_id: "repo-1",
      });
    }
    if (url.startsWith("/batches/b-failed") && method === "GET") {
      return mockJsonResponse(batches[0]);
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { ControlPlaneView } = loadModule();
  await ControlPlaneView.routeApp();

  const buttons = findAllTags(document.getElementById("content"), "BUTTON");
  const restartButtons = buttons.filter((btn) => btn.textContent === "Restart");
  assert.strictEqual(restartButtons.length, 1, "only failed batch exposes restart");

  await restartButtons[0].trigger("click");
  assert.strictEqual(restartCalls, 1, "restart endpoint called once");

  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /Restarted batch b-failed/);
});
