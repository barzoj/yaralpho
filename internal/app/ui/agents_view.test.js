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

  replaceWith(node) {
    this.replacedWith = node;
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
  let text = node.textContent || "";
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

test("agents route renders list and disables busy actions", async () => {
  setupDom({ hash: "#/agents" });
  const agents = [
    {
      agent_id: "a1",
      name: "idle-agent",
      runtime: "codex",
      status: "idle",
      created_at: "2026-02-15T00:00:00Z",
      updated_at: "2026-02-15T00:00:00Z",
    },
    {
      agent_id: "a2",
      name: "busy-agent",
      runtime: "copilot",
      status: "busy",
      created_at: "2026-02-15T01:00:00Z",
      updated_at: "2026-02-15T01:00:00Z",
    },
  ];
  global.fetch = (url, options = {}) => {
    assert.strictEqual(url, "/agent");
    assert.strictEqual(options.method, undefined);
    return mockJsonResponse(agents);
  };

  const { AgentsView } = loadModule();
  await AgentsView.routeApp();

  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /Loaded 2/);
  const table = findFirstTag(document.getElementById("content"), "TABLE");
  assert.ok(table, "table rendered");
  const tbody = table.children.find((el) => el.tagName === "TBODY");
  const rows = tbody?.children || [];
  assert.strictEqual(rows.length, 2);
  const busyActionsDiv = rows[1].children[6].children[0];
  const busyButtons = busyActionsDiv.children;
  assert.ok(busyButtons[0].disabled);
  assert.ok(busyButtons[1].disabled);
});

test("create agent posts and refetches list", async () => {
  setupDom({ hash: "#/agents" });
  let agents = [];
  const calls = [];
  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    calls.push({ url, method, body: options.body });
    if (url === "/agent" && method === "GET") {
      return mockJsonResponse(agents);
    }
    if (url === "/agent" && method === "POST") {
      const body = JSON.parse(options.body);
      const now = "2026-02-15T02:00:00Z";
      agents = [
        {
          agent_id: "a-new",
          name: body.name,
          runtime: body.runtime,
          status: "idle",
          created_at: now,
          updated_at: now,
        },
      ];
      return mockJsonResponse(agents[0], 201);
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { AgentsView } = loadModule();
  await AgentsView.routeApp();

  const container = document.getElementById("content").children[0];
  const createForm = container.children[0];
  const nameInput = createForm.children[1];
  const runtimeSelect = createForm.children[2];
  nameInput.value = "worker-1";
  runtimeSelect.value = "copilot";
  await createForm.trigger("submit");

  const postCalls = calls.filter((c) => c.method === "POST");
  assert.strictEqual(postCalls.length, 1);
  assert.deepStrictEqual(JSON.parse(postCalls[0].body), { name: "worker-1", runtime: "copilot" });
  const text = collectText(document.getElementById("content"));
  assert.match(text, /worker-1/);
  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /Agent created/);
});

test("edit and delete update list with busy guard intact", async () => {
  setupDom({ hash: "#/agents" });
  let agents = [
    {
      agent_id: "a1",
      name: "first",
      runtime: "codex",
      status: "idle",
      created_at: "2026-02-15T03:00:00Z",
      updated_at: "2026-02-15T03:00:00Z",
    },
  ];
  const calls = [];
  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    calls.push({ url, method, body: options.body });
    if (url === "/agent" && method === "GET") {
      return mockJsonResponse(agents);
    }
    if (url.startsWith("/agent/") && method === "PUT") {
      const body = JSON.parse(options.body);
      agents = [
        {
          ...agents[0],
          name: body.name,
          runtime: body.runtime,
          updated_at: "2026-02-15T04:00:00Z",
        },
      ];
      return mockJsonResponse(agents[0]);
    }
    if (url.startsWith("/agent/") && method === "DELETE") {
      agents = [];
      return mockJsonResponse({}, 204);
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { AgentsView } = loadModule();
  await AgentsView.routeApp();

  const container = document.getElementById("content").children[0];
  const editForm = container.children[1];
  let table = findFirstTag(container, "TABLE");
  let tbody = table.children.find((el) => el.tagName === "TBODY");
  let actionsDiv = tbody.children[0].children[6].children[0];
  await actionsDiv.children[0].trigger("click"); // select for edit

  const editName = editForm.children[2];
  const editRuntime = editForm.children[3];
  editName.value = "updated";
  editRuntime.value = "copilot";
  await editForm.trigger("submit");

  const putCalls = calls.filter((c) => c.method === "PUT");
  assert.strictEqual(putCalls.length, 1);
  const textAfterEdit = collectText(document.getElementById("content"));
  assert.match(textAfterEdit, /updated/);

  table = findFirstTag(container, "TABLE");
  tbody = table.children.find((el) => el.tagName === "TBODY");
  actionsDiv = tbody.children[0].children[6].children[0];
  await actionsDiv.children[1].trigger("click"); // delete
  const deleteCalls = calls.filter((c) => c.method === "DELETE");
  assert.strictEqual(deleteCalls.length, 1);
  const textAfterDelete = collectText(document.getElementById("content"));
  assert.match(textAfterDelete, /No agents found/i);
});
