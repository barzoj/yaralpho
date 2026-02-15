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
    // no-op for tests
  }
}

function setupDom({ hash = "", search = "", pathname = "/app" } = {}) {
  const listeners = {};
  global.Node = FakeElement;
  global.document = new FakeDocument();
  ["status", "content", "view-title", "breadcrumbs", "nav", "nav-toggle", "nav-dropdown"].forEach((id) => {
    document._byId.set(id, new FakeElement("div"));
  });

  const locationObj = {
    hash,
    search,
    pathname,
    protocol: "http:",
    host: "localhost",
    origin: "http://localhost",
    href: `http://localhost${pathname}${search}${hash}`,
  };

  global.Event = class {
    constructor(type) {
      this.type = type;
    }
  };

  global.window = {
    location: locationObj,
    listeners,
    history: {
      pushState: (_, __, url) => {
        const parsed = new URL(url, "http://localhost");
        locationObj.hash = parsed.hash || "";
        locationObj.search = parsed.search || "";
        locationObj.pathname = parsed.pathname || "";
        locationObj.href = parsed.href;
        const handlerList = listeners.popstate || [];
        handlerList.forEach((fn) => fn({ state: null }));
      },
    },
    addEventListener: (event, handler) => {
      if (!listeners[event]) listeners[event] = [];
      listeners[event].push(handler);
    },
    removeEventListener: (event, handler) => {
      if (!listeners[event]) return;
      listeners[event] = listeners[event].filter((fn) => fn !== handler);
    },
    dispatchEvent: (evt) => {
      const handlerList = listeners[evt.type] || [];
      handlerList.forEach((fn) => fn(evt));
    },
  };
}

function loadModule() {
  delete require.cache[require.resolve("./app.js")];
  return require("./app.js").NavRouting;
}

function setFetchStub(respond) {
  global.fetch = (url, options = {}) => respond(url, options);
}

function textContent(node) {
  if (!node) return "";
  let text = typeof node.textContent === "string" ? node.textContent : "";
  for (const child of node.children || []) {
    text += ` ${textContent(child)}`;
  }
  return text.trim();
}

test("getRouteFromHash normalizes routes", () => {
  setupDom({ hash: "" });
  const { getRouteFromHash } = loadModule();
  assert.strictEqual(getRouteFromHash(""), "");
  assert.strictEqual(getRouteFromHash("#/agents"), "agents");
  assert.strictEqual(getRouteFromHash("#/agents?foo=bar"), "agents");
  assert.strictEqual(getRouteFromHash("#/repositories"), "repositories");
  assert.strictEqual(getRouteFromHash("#/control-plane"), "control-plane");
});

test("default route renders control plane when no hash/query", async () => {
  setupDom({ hash: "", search: "" });
  setFetchStub((url, options = {}) => {
    if (url === "/repository" && (!options.method || options.method === "GET")) {
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => "application/json" },
        json: () => Promise.resolve([]),
        text: () => Promise.resolve("[]"),
      });
    }
    throw new Error(`Unexpected fetch ${url}`);
  });
  const { routeApp } = loadModule();

  await routeApp();

  assert.strictEqual(document.getElementById("view-title").textContent, "Control Plane");
  const navText = textContent(document.getElementById("nav"));
  assert.match(navText, /Control Plane/);
});

test("hash routes dispatch correct view", async () => {
  setupDom({ hash: "#/agents", search: "" });
  setFetchStub((url, options = {}) => {
    if (url === "/agent" && (!options.method || options.method === "GET")) {
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => "application/json" },
        json: () => Promise.resolve([]),
        text: () => Promise.resolve("[]"),
      });
    }
    throw new Error(`Unexpected fetch ${url}`);
  });
  const { routeApp } = loadModule();

  await routeApp();

  assert.strictEqual(document.getElementById("view-title").textContent, "Agents");
  const navText = textContent(document.getElementById("nav"));
  assert.match(navText, /Agents/);
});

test("batch query param takes precedence over hash route", async () => {
  setupDom({ hash: "#/agents", search: "?batch=b1" });
  setFetchStub((url, options = {}) => {
    const method = (options && options.method) || "GET";
    if (url === "/batches/b1" && method === "GET") {
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => "application/json" },
        json: () => Promise.resolve({ batch: { batch_id: "b1", repository_id: "repo1", session_name: "Batch One" } }),
        text: () => Promise.resolve("{}"),
      });
    }
    if (url.startsWith("/repository/repo1/batch/b1/runs") && method === "GET") {
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => "application/json" },
        json: () => Promise.resolve({ runs: [], count: 0 }),
        text: () => Promise.resolve("{}"),
      });
    }
    throw new Error(`Unexpected fetch ${url}`);
  });
  const { routeApp } = loadModule();

  await routeApp();

  assert.match(document.getElementById("view-title").textContent, /Runs for/);
});

test("navigateToRoute clears query params and updates hash", async () => {
  setupDom({ hash: "#/agents", search: "?batch=b1", pathname: "/app" });
  window.__NAV_SKIP_ROUTE__ = true;
  const { navigateToRoute, buildNavHref } = loadModule();

  navigateToRoute("repositories");

  assert.strictEqual(window.location.search, "");
  assert.strictEqual(window.location.hash, "#/repositories");
  assert.strictEqual(buildNavHref("version"), "http://localhost/app#/version");
});
