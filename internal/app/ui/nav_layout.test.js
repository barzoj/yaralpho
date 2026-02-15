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
  }

  appendChild(child) {
    if (!child) return child;
    child.parentNode = this;
    this.children.push(child);
    return child;
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
    this.body = new FakeElement("body");
  }

  createElement(tag) {
    return new FakeElement(tag);
  }

  getElementById(id) {
    return this._byId.get(id) || null;
  }

  addEventListener() {
    // no-op for tests
  }
}

function setupDom() {
  global.Node = FakeElement;
  global.document = new FakeDocument();
  const ids = ["status", "content", "view-title", "breadcrumbs", "nav", "footer-content"];
  ids.forEach((id) => {
    const el = new FakeElement(id === "nav" ? "nav" : "div");
    document._byId.set(id, el);
    document.body.appendChild(el);
  });
  global.window = {
    location: { hash: "", protocol: "http:", host: "localhost" },
    addEventListener() {},
    removeEventListener() {},
  };
}

function loadModule() {
  delete require.cache[require.resolve("./app.js")];
  return require("./app.js");
}

test("renderNav stacks links vertically and preserves status layout", () => {
  setupDom();
  const { VersionView } = loadModule();

  VersionView.renderNav("control-plane");

  const navEl = document.getElementById("nav");
  const statusEl = document.getElementById("status");
  assert.ok(navEl.className.includes("nav-stack"), "nav-stack class applied");
  assert.ok(navEl.className.includes("nav-list"), "nav-list class applied");
  assert.ok(navEl.children.length >= 4, "nav contains links");

  navEl.children.forEach((link) => {
    assert.ok(link.className.includes("nav-link"), "nav-link class applied");
    assert.ok(link.className.includes("button-link"), "button-link retained");
    assert.strictEqual(link.parentNode, navEl);
  });

  const active = navEl.children.find((link) => link.getAttribute("aria-current") === "page");
  assert.ok(active, "active link present");
  assert.strictEqual(active.textContent, "Control Plane");

  assert.strictEqual(statusEl.parentNode, document.body, "status remains in main layout");
});
