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

test("repositories route renders list and shows path hint", async () => {
  setupDom({ hash: "#/repositories" });
  const repos = [
    {
      repository_id: "r1",
      name: "first",
      path: "/path/one",
      created_at: "2026-02-15T00:00:00Z",
      updated_at: "2026-02-15T00:00:00Z",
    },
    {
      repository_id: "r2",
      name: "second",
      path: "/path/two",
      created_at: "2026-02-15T01:00:00Z",
      updated_at: "2026-02-15T01:00:00Z",
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

  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /Loaded 2/);
  const table = findFirstTag(document.getElementById("content"), "TABLE");
  assert.ok(table, "table rendered");
  const tbody = table.children.find((el) => el.tagName === "TBODY");
  assert.strictEqual(tbody?.children.length, 2);
  const containerText = collectText(document.getElementById("content"));
  assert.match(containerText, /absolute/);
});

test("create repository posts and refreshes list", async () => {
  setupDom({ hash: "#/repositories" });
  let repos = [];
  const calls = [];
  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    calls.push({ url, method, body: options.body });
    if (url === "/repository" && method === "GET") {
      return mockJsonResponse(repos);
    }
    if (url === "/repository" && method === "POST") {
      const body = JSON.parse(options.body);
      const now = "2026-02-15T02:00:00Z";
      repos = [
        {
          repository_id: "r-new",
          name: body.name,
          path: body.path,
          created_at: now,
          updated_at: now,
        },
      ];
      return mockJsonResponse(repos[0], 201);
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { RepositoriesView } = loadModule();
  await RepositoriesView.routeApp();

  const container = document.getElementById("content").children[0];
  const createForm = container.children[0];
  const nameInput = createForm.children[1];
  const pathInput = createForm.children[2];
  nameInput.value = "repo-one";
  pathInput.value = "/abs/repo";
  await createForm.trigger("submit");

  const postCalls = calls.filter((c) => c.method === "POST");
  assert.strictEqual(postCalls.length, 1);
  assert.deepStrictEqual(JSON.parse(postCalls[0].body), { name: "repo-one", path: "/abs/repo" });
  const text = collectText(document.getElementById("content"));
  assert.match(text, /repo-one/);
  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /Repository created/);
});

test("edit and delete handle updates and active batch conflict", async () => {
  setupDom({ hash: "#/repositories" });
  let repos = [
    {
      repository_id: "r1",
      name: "first",
      path: "/path/one",
      created_at: "2026-02-15T03:00:00Z",
      updated_at: "2026-02-15T03:00:00Z",
    },
  ];
  const calls = [];
  let deleteAttempts = 0;
  global.fetch = (url, options = {}) => {
    const method = (options && options.method) || "GET";
    calls.push({ url, method, body: options.body });
    if (url === "/repository" && method === "GET") {
      return mockJsonResponse(repos);
    }
    if (url.startsWith("/repository/") && method === "PUT") {
      const body = JSON.parse(options.body);
      repos = [
        {
          ...repos[0],
          name: body.name,
          path: body.path,
          updated_at: "2026-02-15T04:00:00Z",
        },
      ];
      return mockJsonResponse(repos[0]);
    }
    if (url.startsWith("/repository/") && method === "DELETE") {
      deleteAttempts += 1;
      if (deleteAttempts === 1) {
        return mockJsonResponse({ error: "repository has active batches" }, 409);
      }
      repos = [];
      return mockJsonResponse({}, 204);
    }
    throw new Error(`Unexpected fetch ${method} ${url}`);
  };

  const { RepositoriesView } = loadModule();
  await RepositoriesView.routeApp();

  const container = document.getElementById("content").children[0];
  const editForm = container.children[1];
  let table = findFirstTag(container, "TABLE");
  let tbody = table.children.find((el) => el.tagName === "TBODY");
  let actionsDiv = tbody.children[0].children[5].children[0];
  await actionsDiv.children[0].trigger("click"); // select for edit

  const editName = editForm.children[2];
  const editPath = editForm.children[3];
  editName.value = "updated";
  editPath.value = "/path/updated";
  await editForm.trigger("submit");

  const putCalls = calls.filter((c) => c.method === "PUT");
  assert.strictEqual(putCalls.length, 1);
  const textAfterEdit = collectText(document.getElementById("content"));
  assert.match(textAfterEdit, /updated/);

  table = findFirstTag(container, "TABLE");
  tbody = table.children.find((el) => el.tagName === "TBODY");
  actionsDiv = tbody.children[0].children[5].children[0];
  await actionsDiv.children[1].trigger("click"); // delete attempt with conflict
  const statusAfterConflict = collectText(document.getElementById("status"));
  assert.match(statusAfterConflict.toLowerCase(), /active|conflict/);
  await actionsDiv.children[1].trigger("click"); // delete success
  const deleteCalls = calls.filter((c) => c.method === "DELETE");
  assert.strictEqual(deleteCalls.length, 2);
  const textAfterDelete = collectText(document.getElementById("content"));
  assert.match(textAfterDelete, /No repositories found/i);
});

test("repositories view uses mobile-friendly layout helpers", async () => {
  setupDom({ hash: "#/repositories" });
  const repos = [
    {
      repository_id: "r1",
      name: "one",
      path: "/path/one",
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
  const tableWrapper = container.children[2];

  assert.match(createForm.className, /stack-sm/);
  assert.match(editForm.className, /stack-sm/);
  assert.strictEqual(createForm.getAttribute("aria-label"), "Create repository");
  assert.strictEqual(editForm.getAttribute("aria-label"), "Edit repository");
  assert.match(tableWrapper.className, /scroll-card/);
  assert.strictEqual(tableWrapper.getAttribute("aria-label"), "Repositories table");

  const table = findFirstTag(tableWrapper, "TABLE");
  assert.ok(table, "table rendered");
  assert.match(table.className || "", /sticky-header/);
});
