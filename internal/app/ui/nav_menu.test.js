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

  contains(node) {
    let current = node;
    while (current) {
      if (current === this) return true;
      current = current.parentNode || null;
    }
    return false;
  }

  addEventListener(type, fn) {
    if (!this._listeners[type]) this._listeners[type] = [];
    this._listeners[type].push(fn);
  }

  focus() {
    if (global.document) {
      global.document.activeElement = this;
    }
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
    this.activeElement = null;
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

  removeEventListener(type, fn) {
    if (!this._listeners[type]) return;
    this._listeners[type] = this._listeners[type].filter((cb) => cb !== fn);
  }

  dispatch(type, evt) {
    (this._listeners[type] || []).forEach((fn) => fn(evt));
  }
}

function setupDom(width = 500) {
  global.Node = FakeElement;
  global.document = new FakeDocument();
  const ids = ["status", "content", "view-title", "breadcrumbs", "nav-toggle", "nav-dropdown", "nav", "footer-content"];
  ids.forEach((id) => {
    const tag = id === "nav" ? "nav" : "div";
    const el = new FakeElement(tag);
    el.id = id;
    document._byId.set(id, el);
    document.body.appendChild(el);
  });
  document.getElementById("nav-dropdown").appendChild(document.getElementById("nav"));
  const listeners = {};
  global.window = {
    innerWidth: width,
    location: { hash: "", protocol: "http:", host: "localhost" },
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
  };
}

function loadModule() {
  delete require.cache[require.resolve("./app.js")];
  return require("./app.js");
}

test("nav menu toggles aria and visibility on mobile", () => {
  setupDom(500);
  const { VersionView, NavMenu } = loadModule();

  VersionView.renderNav("batches");
  NavMenu.syncNavMenuForViewport();

  const dropdown = document.getElementById("nav-dropdown");
  const toggle = document.getElementById("nav-toggle");

  assert.strictEqual(dropdown.getAttribute("data-open"), "false");
  assert.strictEqual(dropdown.getAttribute("aria-hidden"), "true");
  assert.strictEqual(toggle.getAttribute("aria-expanded"), "false");

  NavMenu.openNavMenu();

  assert.strictEqual(dropdown.getAttribute("data-open"), "true");
  assert.strictEqual(dropdown.getAttribute("aria-hidden"), "false");
  assert.strictEqual(toggle.getAttribute("aria-expanded"), "true");
  assert.ok(NavMenu.isNavMenuOpen());

  NavMenu.closeNavMenu();

  assert.strictEqual(dropdown.getAttribute("data-open"), "false");
  assert.strictEqual(dropdown.getAttribute("aria-hidden"), "true");
  assert.strictEqual(toggle.getAttribute("aria-expanded"), "false");
  assert.ok(!NavMenu.isNavMenuOpen());
});

test("nav menu closes on outside click, escape, and traps focus", () => {
  setupDom(500);
  const { VersionView, NavMenu } = loadModule();

  VersionView.renderNav("batches");
  NavMenu.openNavMenu();

  const navLinks = NavMenu.getNavLinks();
  assert.ok(navLinks.length >= 2, "nav links available");

  // Outside click closes
  document.dispatch("click", { target: new FakeElement("div") });
  assert.ok(!NavMenu.isNavMenuOpen(), "closed after outside click");

  // Re-open and close via Escape
  NavMenu.openNavMenu();
  document.dispatch("keydown", { key: "Escape", preventDefault() {} });
  assert.ok(!NavMenu.isNavMenuOpen(), "closed after Escape");

  // Focus trap cycles tab order
  NavMenu.openNavMenu();
  const first = navLinks[0];
  const last = navLinks[navLinks.length - 1];
  last.focus();
  document.dispatch("keydown", {
    key: "Tab",
    preventDefault() {},
    shiftKey: false,
  });
  assert.strictEqual(document.activeElement, first, "tab wraps to first link");

  first.focus();
  document.dispatch("keydown", {
    key: "Tab",
    preventDefault() {},
    shiftKey: true,
  });
  assert.strictEqual(document.activeElement, last, "shift+tab wraps to last link");
});

test("nav menu resets on desktop width", () => {
  setupDom(1024);
  const { VersionView, NavMenu } = loadModule();

  VersionView.renderNav("batches");
  NavMenu.openNavMenu();
  NavMenu.syncNavMenuForViewport();

  const dropdown = document.getElementById("nav-dropdown");
  assert.strictEqual(dropdown.getAttribute("data-open"), "true");
  assert.strictEqual(dropdown.getAttribute("aria-hidden"), "false");
  assert.ok(!NavMenu.isNavMenuOpen(), "state not considered open on desktop");
});
