const assert = require("node:assert");
const test = require("node:test");

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.style = {
      setProperty: (key, value) => {
        this.style[key] = value;
      },
    };
    this.className = "";
    this.textContent = "";
    this.parentNode = null;
    this.attributes = {};
    this._height = 0;
  }

  appendChild(child) {
    if (!child) return child;
    child.parentNode = this;
    this.children.push(child);
    return child;
  }

  insertBefore(child, before) {
    const idx = this.children.indexOf(before);
    if (idx === -1) {
      return this.appendChild(child);
    }
    child.parentNode = this;
    this.children.splice(idx, 0, child);
    return child;
  }

  replaceWith(next) {
    if (!this.parentNode) return;
    const idx = this.parentNode.children.indexOf(this);
    if (idx !== -1) {
      this.parentNode.children.splice(idx, 1, next);
      next.parentNode = this.parentNode;
    }
  }

  setAttribute(key, value) {
    this.attributes[key] = value;
  }

  getBoundingClientRect() {
    return { height: this._height };
  }

  querySelector(selector) {
    if (selector.startsWith(".")) {
      const cls = selector.slice(1);
      if (this.className.split(" ").includes(cls)) return this;
    }
    for (const child of this.children) {
      const found = child.querySelector(selector);
      if (found) return found;
    }
    return null;
  }
}

class FakeDocument {
  constructor() {
    this.root = new FakeElement("html");
    this.body = new FakeElement("body");
    this.root.appendChild(this.body);
    this._byId = new Map();
  }

  createElement(tag) {
    return new FakeElement(tag);
  }

  getElementById(id) {
    return this._byId.get(id) || null;
  }

  registerElement(id, el) {
    this._byId.set(id, el);
  }

  querySelector(selector) {
    if (selector === "body > header") {
      return this.body.children.find((child) => child.tagName === "HEADER") || null;
    }
    return this.body.querySelector(selector);
  }

  addEventListener() {
    // no-op for tests
  }
}

test("run layout keeps header separate from events scroll", () => {
  const doc = new FakeDocument();
  const pageHeader = new FakeElement("header");
  pageHeader._height = 40;
  doc.body.appendChild(pageHeader);
  doc.registerElement("status", Object.assign(new FakeElement("div"), { _height: 30 }));

  global.document = doc;
  global.window = {
    innerHeight: 800,
    addEventListener() {},
    removeEventListener() {},
    location: { search: "" },
  };

  delete require.cache[require.resolve("./app.js")];
  const { createRunLayout, applyRunLayoutSizing } = require("./app.js").RunLayout || global.RunLayout;

  const layout = createRunLayout();
  const actions = new FakeElement("div");
  const info = new FakeElement("div");
  info.className = "pill";
  const eventsList = new FakeElement("div");
  eventsList.className = "events";

  layout.header.appendChild(actions);
  layout.header.appendChild(info);
  layout.eventsScroll.appendChild(eventsList);
  doc.body.appendChild(layout.layout);

  applyRunLayoutSizing(layout.layout);

  assert.equal(layout.header.parentNode, layout.layout);
  assert.equal(layout.eventsScroll.parentNode, layout.eventsSection);
  assert.equal(layout.eventsSection.parentNode, layout.layout);
  assert.ok(layout.eventsScroll.children.includes(eventsList));
  assert.notEqual(layout.header, layout.eventsScroll);
});

test("scroll container receives sizing and overflow", () => {
  const doc = new FakeDocument();
  const pageHeader = new FakeElement("header");
  pageHeader._height = 24;
  const statusBar = Object.assign(new FakeElement("div"), { _height: 26 });
  statusBar.id = "status";
  doc.body.appendChild(pageHeader);
  doc.registerElement("status", statusBar);
  doc.body.appendChild(statusBar);

  global.document = doc;
  global.window = {
    innerHeight: 600,
    addEventListener() {},
    removeEventListener() {},
    location: { search: "" },
  };

  delete require.cache[require.resolve("./app.js")];
  const { createRunLayout, applyRunLayoutSizing } = require("./app.js").RunLayout || global.RunLayout;

  const layout = createRunLayout();
  doc.body.appendChild(layout.layout);

  applyRunLayoutSizing(layout.layout);

  assert.equal(layout.layout.style["--run-header-offset"], "50px");
  const minHeight = parseInt(layout.eventsScroll.style.minHeight, 10);
  assert.ok(minHeight >= 240, "minHeight set");
  assert.equal(layout.eventsScroll.style.overflowY, "auto");
});
