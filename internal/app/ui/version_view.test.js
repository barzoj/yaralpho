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
  }

  appendChild(child) {
    if (child) {
      this.children.push(child);
    }
    return child;
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

function setupDom({ search = "", hash = "" } = {}) {
  global.Node = FakeElement;
  global.document = new FakeDocument();
  ["status", "content", "view-title", "breadcrumbs", "nav"].forEach((id) => {
    global.document._byId.set(id, new FakeElement("div"));
  });
  global.window = {
    location: { search, hash, protocol: "http:", host: "localhost" },
    addEventListener() {},
    removeEventListener() {},
  };
}

function loadModule() {
  delete require.cache[require.resolve("./app.js")];
  return require("./app.js").VersionView;
}

function createFetchResponse({ ok, status = 200, body = "", contentType = "text/plain" }) {
  return Promise.resolve({
    ok,
    status,
    headers: { get: (name) => (name && name.toLowerCase() === "content-type" ? contentType : null) },
    text: () => Promise.resolve(body),
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

test("renderVersionView renders JSON data", async () => {
  setupDom();
  const jsonBody = JSON.stringify({ version: "1.2.3", commit: "abc123" });
  global.fetch = () => createFetchResponse({ ok: true, status: 200, body: jsonBody, contentType: "application/json" });
  const { renderVersionView } = loadModule();

  await renderVersionView();

  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /loaded/i);
  const contentText = collectText(document.getElementById("content"));
  assert.match(contentText, /1\.2\.3/);
  assert.match(contentText, /version/i);
});

test("renderVersionView handles non-JSON response", async () => {
  setupDom();
  global.fetch = () => createFetchResponse({ ok: true, status: 200, body: "v1.0.0-plain", contentType: "text/plain" });
  const { renderVersionView } = loadModule();

  await renderVersionView();

  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /loaded/i);
  const contentText = collectText(document.getElementById("content"));
  assert.match(contentText, /v1\.0\.0-plain/);
});

test("renderVersionView surfaces errors", async () => {
  setupDom();
  global.fetch = () => createFetchResponse({ ok: false, status: 500, body: "boom", contentType: "text/plain" });
  const { renderVersionView } = loadModule();

  await renderVersionView();

  const statusText = collectText(document.getElementById("status"));
  assert.match(statusText, /(boom|failed)/i);
  const contentText = collectText(document.getElementById("content"));
  assert.match(contentText, /Unable to load version info/i);
});

test("routeApp dispatches version route on hash", async () => {
  setupDom({ hash: "#/version" });
  global.fetch = () => createFetchResponse({ ok: true, status: 200, body: "v2.0.0" });
  const { routeApp } = loadModule();

  await routeApp();

  assert.strictEqual(document.getElementById("view-title").textContent, "Version");
  const navText = collectText(document.getElementById("nav"));
  assert.match(navText, /Version/);
});
