const assert = require("node:assert");
const test = require("node:test");

class FakeElement {
  constructor(tagName) {
    this.tagName = (tagName || "div").toUpperCase();
    this.children = [];
    this.parentNode = null;
    this.className = "";
    this.textContent = "";
    this.href = "";
    this.attributes = new Map();
    this._listeners = {};
  }

  appendChild(child) {
    if (!child) return child;
    child.parentNode = this;
    this.children.push(child);
    return child;
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value));
  }

  getAttribute(name) {
    return this.attributes.get(name);
  }

  addEventListener(type, fn) {
    if (!this._listeners[type]) this._listeners[type] = [];
    this._listeners[type].push(fn);
  }

  querySelectorAll(selector) {
    const selectors = selector.split(",").map((s) => s.trim().toUpperCase());
    const results = [];
    const visit = (node) => {
      if (selectors.includes(node.tagName)) {
        results.push(node);
      }
      node.children.forEach(visit);
    };
    this.children.forEach(visit);
    return results;
  }
}

class FakeDocument {
  constructor() {
    this._byId = new Map();
    this._listeners = {};
    this.body = new FakeElement("body");
  }

  createElement(tag) {
    return new FakeElement(tag);
  }

  getElementById(id) {
    return this._byId.get(id) || null;
  }

  addEventListener(type, fn) {
    if (!this._listeners[type]) this._listeners[type] = [];
    this._listeners[type].push(fn);
  }

  dispatch(type, evt) {
    (this._listeners[type] || []).forEach((fn) => fn(evt));
  }
}

function setupDom({ search, hash, pathname } = {}) {
  const listeners = {};
  global.Node = FakeElement;
  global.document = new FakeDocument();

  const ids = [
    "status",
    "content",
    "view-title",
    "breadcrumbs",
    "nav-toggle",
    "nav-dropdown",
    "nav",
    "footer-content",
  ];
  ids.forEach((id) => {
    const tag = id === "nav" ? "nav" : "div";
    const el = new FakeElement(tag);
    el.id = id;
    document._byId.set(id, el);
    document.body.appendChild(el);
  });
  document.getElementById("nav-dropdown").appendChild(document.getElementById("nav"));

  const defaultHref = `http://localhost${pathname || "/"}${search || ""}${hash || ""}`;
  global.window = {
    __NAV_SKIP_ROUTE__: true,
    location: {
      hash: hash || "",
      search: search || "",
      pathname: pathname || "/",
      origin: "http://localhost",
      protocol: "http:",
      host: "localhost",
      href: defaultHref,
    },
    history: {
      replaceState: (_state, _title, url) => {
        const parsed = new URL(url, "http://localhost");
        window.location.href = parsed.href;
        window.location.hash = parsed.hash;
        window.location.search = parsed.search;
        window.location.pathname = parsed.pathname;
      },
    },
    addEventListener: (type, fn) => {
      listeners[type] = listeners[type] || [];
      listeners[type].push(fn);
    },
    removeEventListener: (type, fn) => {
      listeners[type] = (listeners[type] || []).filter((cb) => cb !== fn);
    },
    dispatch: (type, evt) => {
      (listeners[type] || []).forEach((fn) => fn(evt));
    },
    dispatchEvent: (evt) => {
      if (!evt || !evt.type) return;
      (listeners[evt.type] || []).forEach((fn) => fn(evt));
    },
  };

  return { listeners };
}

function loadModule() {
  delete require.cache[require.resolve("./app.js")];
  return require("./app.js");
}

test("nav links clear batch/run query params and update hash routing", () => {
  setupDom({
    search: "?batch=batch-1&run=run-9",
    hash: "#/runs",
    pathname: "/app",
  });

  const { NavRouting, VersionView } = loadModule();
  VersionView.renderNav("batches");

  const nav = document.getElementById("nav");
  const links = nav.children;
  assert.ok(links.length >= 3, "nav links rendered");

  Array.from(links).forEach((link) => {
    assert.ok(!link.href.includes("?batch"), "href strips batch query");
    assert.ok(!link.href.includes("?run"), "href strips run query");
  });

  let hashChanges = 0;
  window.addEventListener("hashchange", () => {
    hashChanges += 1;
  });

  const controlLink =
    Array.from(links).find((link) => (link.textContent || "").includes("Control")) || links[1];
  const clickHandlers = controlLink._listeners?.click || [];
  assert.ok(clickHandlers.length > 0, "nav link has click handler");
  clickHandlers.forEach((fn) => fn({ preventDefault() {} }));

  assert.strictEqual(window.location.search, "", "search cleared after nav");
  assert.ok(window.location.hash.includes("control-plane"), "hash set to target route");
  assert.ok(hashChanges > 0, "hashchange dispatched");

  const params = NavRouting.getQueryParams();
  assert.strictEqual(params.batch, null);
  assert.strictEqual(params.run, null);
});
