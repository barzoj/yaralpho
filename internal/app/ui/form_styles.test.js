const assert = require("node:assert");
const fs = require("node:fs");
const test = require("node:test");

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.textContent = "";
    this.href = "";
    this.className = "";
    this.attributes = new Map();
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

  querySelectorAll() {
    return [];
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
  ["status", "content", "view-title", "breadcrumbs", "nav", "nav-toggle", "nav-dropdown"].forEach((id) => {
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

function findFirstTag(node, tagName) {
  if (!node) return null;
  if (node.tagName === tagName.toUpperCase()) return node;
  for (const child of node.children || []) {
    const found = findFirstTag(child, tagName);
    if (found) return found;
  }
  return null;
}

test("agents forms apply shared classes and action buttons", async () => {
  setupDom({ hash: "#/agents" });
  const agents = [
    {
      agent_id: "a1",
      name: "worker",
      runtime: "codex",
      status: "idle",
      created_at: "2026-02-15T00:00:00Z",
      updated_at: "2026-02-15T00:00:00Z",
    },
  ];
  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    if (url === "/agent" && method === "GET") {
      return mockJsonResponse(agents);
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { AgentsView } = loadModule();
  await AgentsView.routeApp();

  const container = document.getElementById("content").children[0];
  const createForm = container.children[0];
  const editForm = container.children[1];
  assert.ok(createForm.className.includes("form-grid"));
  assert.ok(editForm.className.includes("form-grid"));
  assert.ok(createForm.children[1].className.includes("input-control")); // name
  assert.ok(createForm.children[2].className.includes("select-control")); // runtime
  assert.ok(createForm.children[3].className.includes("button-control"));
  assert.ok(editForm.children[2].className.includes("input-control")); // name
  assert.ok(editForm.children[3].className.includes("select-control")); // runtime
  assert.ok(editForm.children[4].className.includes("button-primary"));
  assert.ok(editForm.children[5].className.includes("button-secondary"));

  const table = findFirstTag(document.getElementById("content"), "TABLE");
  const tbody = table.children.find((el) => el.tagName === "TBODY");
  const actionCell = tbody.children[0].children[6];
  const buttons = actionCell.children[0].children;
  assert.ok(buttons[0].className.includes("button-control"));
  assert.ok(buttons[1].className.includes("button-control"));
});

test("repositories forms apply shared classes and action buttons", async () => {
  setupDom({ hash: "#/repositories" });
  const repos = [
    {
      repository_id: "r1",
      name: "repo",
      path: "/repo/path",
      created_at: "2026-02-15T00:00:00Z",
      updated_at: "2026-02-15T00:00:00Z",
    },
  ];
  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    if (url === "/repository" && method === "GET") {
      return mockJsonResponse(repos);
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { RepositoriesView } = loadModule();
  await RepositoriesView.routeApp();

  const container = document.getElementById("content").children[0];
  const createForm = container.children[0];
  const editForm = container.children[1];
  assert.ok(createForm.className.includes("form-grid"));
  assert.ok(editForm.className.includes("form-grid"));
  assert.ok(createForm.children[1].className.includes("input-control")); // name
  assert.ok(createForm.children[2].className.includes("input-control")); // path
  assert.ok(createForm.children[4].className.includes("button-primary"));
  assert.ok(editForm.children[2].className.includes("input-control")); // name
  assert.ok(editForm.children[3].className.includes("input-control")); // path
  assert.ok(editForm.children[5].className.includes("button-primary"));
  assert.ok(editForm.children[6].className.includes("button-secondary"));

  const table = findFirstTag(document.getElementById("content"), "TABLE");
  const tbody = table.children.find((el) => el.tagName === "TBODY");
  const actionCell = tbody.children[0].children[5];
  const buttons = actionCell.children[0].children;
  assert.ok(buttons[0].className.includes("button-control"));
  assert.ok(buttons[1].className.includes("button-control"));
});

test("form style tokens defined in index.html", () => {
  const html = fs.readFileSync(__dirname + "/index.html", "utf8");
  assert.match(html, /\.input-control,[\s\S]*\.select-control/);
  assert.match(html, /padding:\s*10px 12px/);
  assert.match(html, /border-radius:\s*10px/);
  assert.match(html, /outline:\s*2px solid var\(--accent\)/);
  assert.match(html, /\.button-primary/);
});
