(function () {
  const statusEl = document.getElementById("status");
  const contentEl = document.getElementById("content");
  const viewTitle = document.getElementById("view-title");
  const breadcrumbsEl = document.getElementById("breadcrumbs");
  const navEl = document.getElementById("nav");
  const runLayoutHelpers = typeof RunLayout !== "undefined" ? RunLayout : {};
  const footerContentEl = document.getElementById("footer-content");
  const DEFAULT_FOOTER_TEXT = "Footer placeholder — add helpful links soon.";

  const params = new URLSearchParams(window.location.search);
  const batchParam = params.get("batch");
  const runParam = params.get("run");

  const LIST_LIMIT = 50;
  const EVENTS_LIMIT = 10000;
  const SCROLL_FOLLOW_THRESHOLD_PX = 24;

  const EVENT_META = {
    abort: { emoji: "⛔", label: "Session aborted" },
    "assistant.intent": {
      emoji: "🎯",
      label: "Assistant intent",
      render: renderAssistantIntent,
    },
    "assistant.message": {
      emoji: "🤖",
      label: "Assistant message",
      render: renderAssistantMessage,
    },
    "assistant.message_delta": { hidden: true },
    "assistant.reasoning": {
      emoji: "🧠",
      label: "Assistant reasoning",
      render: renderAssistantReasoning,
    },
    "assistant.reasoning_delta": { hidden: true },
    "assistant.turn_end": {
      emoji: "🔚",
      label: "Turn end",
      render: (evt) => renderTurnMarker(evt, "Turn end"),
    },
    "assistant.turn_start": {
      emoji: "🔜",
      label: "Turn start",
      render: (evt) => renderTurnMarker(evt, "Turn start"),
    },
    "assistant.usage": {
      emoji: "📊",
      label: "Usage",
      render: renderAssistantUsage,
    },
    "hook.end": { emoji: "🪝", label: "Hook end" },
    "hook.start": { emoji: "🪝", label: "Hook start" },
    "item.completed": {
      emoji: "✅",
      label: "Item completed",
      render: renderCodexItemCompleted,
      summary: (data, evt) => formatCodexItemSummary(evt),
    },
    "item.started": {
      emoji: "🟡",
      label: "Item started",
      render: renderCodexItemStarted,
      summary: (data, evt) => formatCodexItemSummary(evt),
    },
    "pending_messages.modified": {
      emoji: "⏳",
      label: "Pending messages updated",
      render: renderPendingMessage,
    },
    "session.compaction_complete": {
      emoji: "🧹",
      label: "Compaction complete",
    },
    "session.compaction_start": { emoji: "🧹", label: "Compaction start" },
    "session.error": {
      emoji: "⚠️",
      label: "Session error",
      summary: (data) => data?.message || data?.reason,
    },
    "session.handoff": { emoji: "🤝", label: "Session handoff" },
    "session.idle": { emoji: "💤", label: "Session idle", render: renderSessionIdle },
    "session.info": { emoji: "ℹ️", label: "Session info" },
    "session.model_change": {
      emoji: "🧠",
      label: "Model change",
      summary: (data) =>
        [data?.previousModel, data?.newModel].filter(Boolean).join(" → "),
    },
    "session.resume": { emoji: "▶️", label: "Session resume" },
    "session.shutdown": { emoji: "⏻", label: "Session shutdown" },
    "session.snapshot_rewind": { emoji: "⏪", label: "Snapshot rewind" },
    "session.start": { emoji: "🚀", label: "Session start" },
    "session.truncation": {
      emoji: "✂️",
      label: "Session truncation",
      summary: (data) => formatTruncationSummary(data),
    },
    "session.usage_info": {
      emoji: "📈",
      label: "Usage info",
      render: renderSessionUsageInfo,
    },
    "skill.invoked": {
      emoji: "📦",
      label: "Skill invoked",
      render: renderSkillInvoked,
      summary: (data) =>
        data?.name || (data?.content ? data.content.split("\n")[0] : ""),
    },
    "subagent.completed": { emoji: "✅", label: "Subagent completed" },
    "subagent.failed": {
      emoji: "❌",
      label: "Subagent failed",
      summary: (data) => data?.reason,
    },
    "subagent.selected": {
      emoji: "🧩",
      label: "Subagent selected",
      summary: (data) => data?.agentDisplayName || data?.agentName,
    },
    "subagent.started": {
      emoji: "▶️",
      label: "Subagent started",
      summary: (data) => data?.agentDisplayName || data?.agentName,
    },
    "thread.started": {
      emoji: "🧵",
      label: "Thread started",
      render: renderCodexThreadStarted,
    },
    "system.message": {
      emoji: "🛡️",
      label: "System message",
      render: renderSystemMessage,
    },
    "tool.execution_complete": {
      emoji: "🛠️",
      label: "Tool complete",
      render: renderToolExecutionComplete,
      summary: (data) => formatToolSummary(data),
    },
    "tool.execution_partial_result": {
      emoji: "🛠️",
      label: "Tool partial result",
      summary: (data) => data?.partialOutput || formatToolSummary(data),
    },
    "tool.execution_progress": {
      emoji: "🛠️",
      label: "Tool progress",
      summary: (data) => data?.progressMessage || formatToolSummary(data),
    },
    "tool.execution_start": {
      emoji: "🛠️",
      label: "Tool start",
      render: renderToolExecutionStart,
      summary: (data) => formatToolSummary(data),
    },
    "tool.user_requested": { emoji: "🙋", label: "Tool requested by user" },
    "turn.completed": {
      emoji: "🏁",
      label: "Turn completed",
      render: renderCodexTurnCompleted,
    },
    "turn.started": {
      emoji: "▶️",
      label: "Turn started",
      render: renderCodexTurnStarted,
    },
    "user.message": {
      emoji: "🧑",
      label: "User message",
      render: renderUserMessage,
    },
  };

  const DEFAULT_EVENT_META = {
    emoji: "❔",
    label: "Unknown event",
    render: renderUnknownEvent,
  };

  const ENVELOPE_TYPE_EVENT = "event";
  const ENVELOPE_TYPE_ERROR = "error";
  const ENVELOPE_TYPE_HEARTBEAT = "heartbeat";
  const LIVE_RETRY_BASE_DELAY_MS = 1000;
  const LIVE_RETRY_MAX_DELAY_MS = 15000;
  const LIVE_RETRY_MAX_ATTEMPTS = 5;
  const LIVE_RETRY_JITTER = 0.2;

  let liveStreamState = null;
  let liveUnloadCleanup = null;
  let liveReconnectController = null;
  let runLayoutCleanup = null;

  function setStatus(text, type = "info", options = {}) {
    statusEl.className = `status ${type}`;
    statusEl.textContent = text;
    const action = options.action;
    if (action && action.label) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "button-link";
      btn.textContent = action.label;
      if (typeof action.onClick === "function") {
        btn.addEventListener("click", action.onClick);
      }
      statusEl.appendChild(document.createTextNode(" "));
      statusEl.appendChild(btn);
    }
  }

  function clearContent() {
    contentEl.innerHTML = "";
  }

  function renderBreadcrumbs(items) {
    breadcrumbsEl.innerHTML = "";
    if (!items || !items.length) {
      return;
    }
    items.forEach((item, idx) => {
      if (idx > 0) {
        const sep = document.createElement("span");
        sep.textContent = "›";
        sep.setAttribute("aria-hidden", "true");
        breadcrumbsEl.appendChild(sep);
      }
      if (item.href) {
        const link = document.createElement("a");
        link.href = item.href;
        link.textContent = item.label;
        breadcrumbsEl.appendChild(link);
      } else {
        const span = document.createElement("span");
        span.textContent = item.label;
        breadcrumbsEl.appendChild(span);
      }
    });
  }

  function createLink(href, text) {
    const a = document.createElement("a");
    a.href = href;
    a.textContent = text;
    return a;
  }

  function createButtonLink(href, text) {
    const a = createLink(href, text);
    a.className = "button-link";
    return a;
  }

  function setFooterContent(text) {
    if (!footerContentEl) return;
    const nextText =
      text === null || text === undefined ? DEFAULT_FOOTER_TEXT : String(text);
    footerContentEl.textContent = nextText;
  }

  const NAV_ITEMS = [
    { label: "Batches", href: "#/", route: "batches" },
    { label: "Control Plane", href: "#/control-plane", route: "control-plane" },
    { label: "Repositories", href: "#/repositories", route: "repositories" },
    { label: "Agents", href: "#/agents", route: "agents" },
    { label: "Version", href: "#/version", route: "version" },
  ];
  const AGENT_RUNTIMES = ["codex", "copilot"];

  function renderNav(activeRoute) {
    if (!navEl) return;
    navEl.innerHTML = "";
    navEl.className = "nav-stack nav-list";
    NAV_ITEMS.forEach((item) => {
      const link = document.createElement("a");
      link.href = item.href;
      link.textContent = item.label;
      link.className = "button-link nav-link";
      if (item.route === activeRoute) {
        link.setAttribute("aria-current", "page");
      }
      navEl.appendChild(link);
    });
  }

  function formatDate(raw) {
    if (!raw) return "—";
    const date = new Date(raw);
    if (Number.isNaN(date.getTime())) return raw;
    return date.toLocaleString();
  }

  function emptyState(text) {
    const div = document.createElement("div");
    div.className = "empty";
    div.textContent = text;
    return div;
  }

  async function fetchJSON(url, options) {
    const res = await fetch(url, options);
    const contentType = res.headers.get("content-type") || "";
    let body;
    if (contentType.includes("application/json")) {
      try {
        body = await res.json();
      } catch (_) {
        body = null;
      }
    } else {
      body = await res.text();
    }
    if (!res.ok) {
      const msg =
        (body && body.error) ||
        (body && body.message) ||
        (typeof body === "string" ? body : "") ||
        `Request failed (${res.status})`;
      const err = new Error(msg);
      err.status = res.status;
      throw err;
    }
    return body;
  }

  const batchCache = new Map();

  async function fetchBatchDetail(batchId) {
    if (!batchId) {
      throw new Error("Batch ID is required");
    }
    if (batchCache.has(batchId)) {
      return batchCache.get(batchId);
    }
    const data = await fetchJSON(`/batches/${encodeURIComponent(batchId)}`);
    const batch = data?.batch || data;
    if (!batch || !batch.batch_id) {
      throw new Error("Batch not found");
    }
    batchCache.set(batchId, batch);
    return batch;
  }

  async function fetchVersionData() {
    const res = await fetch("/version");
    const contentType = (res.headers && res.headers.get && res.headers.get("content-type")) || "";
    const rawText = await res.text();
    let parsed = null;
    if (contentType.includes("application/json")) {
      try {
        parsed = JSON.parse(rawText);
      } catch (_) {
        parsed = null;
      }
    }
    if (!res.ok) {
      const message =
        (parsed && (parsed.error || parsed.message)) ||
        rawText ||
        `Request failed (${res.status})`;
      const err = new Error(message);
      err.status = res.status;
      throw err;
    }
    return parsed !== null ? parsed : rawText;
  }

  async function fetchAgentsList() {
    const data = await fetchJSON("/agent");
    if (Array.isArray(data)) return data;
    if (Array.isArray(data?.agents)) return data.agents;
    return [];
  }

  async function fetchRepositoriesList() {
    const data = await fetchJSON("/repository");
    if (Array.isArray(data)) return data;
    if (Array.isArray(data?.repositories)) return data.repositories;
    return [];
  }

  function buildAgentTable(agents, { onEdit, onDelete }) {
    const rows = (agents || []).map((agent) => {
      const status = agent?.status || "unknown";
      const isBusy = String(status).toLowerCase() === "busy";
      const actions = document.createElement("div");
      actions.className = "actions";

      const editBtn = document.createElement("button");
      editBtn.type = "button";
      editBtn.textContent = "Edit";
      editBtn.disabled = isBusy;
      if (isBusy) editBtn.title = "Agent is busy";
      if (typeof onEdit === "function" && !isBusy) {
        editBtn.addEventListener("click", () => onEdit(agent));
      }
      actions.appendChild(editBtn);

      const deleteBtn = document.createElement("button");
      deleteBtn.type = "button";
      deleteBtn.textContent = "Delete";
      deleteBtn.disabled = isBusy;
      if (isBusy) deleteBtn.title = "Agent is busy";
      if (typeof onDelete === "function" && !isBusy) {
        deleteBtn.addEventListener("click", () => onDelete(agent));
      }
      actions.appendChild(deleteBtn);

      return [
        agent?.agent_id || "—",
        agent?.name || "—",
        agent?.runtime || "—",
        status,
        formatDate(agent?.created_at),
        formatDate(agent?.updated_at),
        actions,
      ];
    });

    if (!rows.length) {
      const wrapper = document.createElement("div");
      wrapper.appendChild(emptyState("No agents found."));
      return wrapper;
    }

    return buildTable(
      ["Agent ID", "Name", "Runtime", "Status", "Created", "Updated", "Actions"],
      rows
    );
  }

  function setAgentFormState(form, disabled) {
    if (!form) return;
    const controls = form.querySelectorAll
      ? form.querySelectorAll("input, select, button")
      : form.children || [];
    if (controls.forEach) {
      controls.forEach((node) => {
        if (node) node.disabled = !!disabled;
      });
    } else {
      for (const node of controls) {
        if (node) node.disabled = !!disabled;
      }
    }
  }

  function setRepositoryFormState(form, disabled) {
    if (!form) return;
    const controls = form.querySelectorAll
      ? form.querySelectorAll("input, button")
      : form.children || [];
    if (controls.forEach) {
      controls.forEach((node) => {
        if (node) node.disabled = !!disabled;
      });
    } else {
      for (const node of controls) {
        if (node) node.disabled = !!disabled;
      }
    }
  }

  async function renderAgentsView() {
    viewTitle.textContent = "Agents";
    renderBreadcrumbs([{ label: "Agents" }]);
    setStatus("Loading agents…", "loading");
    clearContent();

    let agents = [];
    let editingId = "";

    const container = document.createElement("div");
    container.className = "agents-view";

    const createForm = document.createElement("form");
    createForm.className = "card";
    const createHeader = document.createElement("h3");
    createHeader.textContent = "Create agent";
    createForm.appendChild(createHeader);
    const createName = document.createElement("input");
    createName.type = "text";
    createName.placeholder = "Name";
    const createRuntime = document.createElement("select");
    AGENT_RUNTIMES.forEach((rt) => {
      const opt = document.createElement("option");
      opt.value = rt;
      opt.textContent = rt;
      createRuntime.appendChild(opt);
    });
    createForm.appendChild(createName);
    createForm.appendChild(createRuntime);
    const createSubmit = document.createElement("button");
    createSubmit.type = "submit";
    createSubmit.textContent = "Create";
    createForm.appendChild(createSubmit);

    const editForm = document.createElement("form");
    editForm.className = "card";
    const editHeader = document.createElement("h3");
    editHeader.textContent = "Edit agent";
    editForm.appendChild(editHeader);
    const editHint = document.createElement("div");
    editHint.className = "pill";
    editHint.textContent = "Select an agent to edit";
    editForm.appendChild(editHint);
    const editName = document.createElement("input");
    editName.type = "text";
    editName.placeholder = "Name";
    const editRuntime = document.createElement("select");
    AGENT_RUNTIMES.forEach((rt) => {
      const opt = document.createElement("option");
      opt.value = rt;
      opt.textContent = rt;
      editRuntime.appendChild(opt);
    });
    editForm.appendChild(editName);
    editForm.appendChild(editRuntime);
    const editSubmit = document.createElement("button");
    editSubmit.type = "submit";
    editSubmit.textContent = "Update";
    editSubmit.disabled = true;
    const editCancel = document.createElement("button");
    editCancel.type = "button";
    editCancel.textContent = "Clear";
    editCancel.disabled = true;
    editForm.appendChild(editSubmit);
    editForm.appendChild(editCancel);

    const tableContainer = document.createElement("div");
    tableContainer.className = "card";

    container.appendChild(createForm);
    container.appendChild(editForm);
    container.appendChild(tableContainer);
    contentEl.appendChild(container);

    function setEditing(agent) {
      editingId = agent?.agent_id || "";
      if (editingId) {
        editHint.textContent = `Editing ${agent.name || editingId}`;
        editName.value = agent.name || "";
        editRuntime.value = AGENT_RUNTIMES.includes(agent.runtime) ? agent.runtime : AGENT_RUNTIMES[0];
        editSubmit.disabled = false;
        editCancel.disabled = false;
      } else {
        editHint.textContent = "Select an agent to edit";
        editName.value = "";
        editRuntime.value = AGENT_RUNTIMES[0];
        editSubmit.disabled = true;
        editCancel.disabled = true;
      }
    }

    function renderTable(list) {
      tableContainer.innerHTML = "";
      const table = buildAgentTable(list, {
        onEdit: setEditing,
        onDelete: async (agent) => {
          try {
            setStatus(`Deleting ${agent.name || agent.agent_id}…`, "loading");
            await fetchJSON(`/agent/${encodeURIComponent(agent.agent_id)}`, {
              method: "DELETE",
            });
            if (agent.agent_id === editingId) {
              setEditing(null);
            }
            await loadAgents(false);
            setStatus("Agent deleted", "success");
          } catch (err) {
            setStatus(err.message || "Failed to delete agent", "error");
          }
        },
      });
      tableContainer.appendChild(table);
    }

    async function loadAgents(showStatus = true) {
      try {
        if (showStatus) {
          setStatus("Loading agents…", "loading");
        }
        agents = await fetchAgentsList();
        if (editingId && !agents.find((a) => a.agent_id === editingId)) {
          setEditing(null);
        }
        renderTable(agents);
        const count = agents.length;
        setStatus(`Loaded ${count} agent${count === 1 ? "" : "s"}`, "success");
      } catch (err) {
        tableContainer.innerHTML = "";
        tableContainer.appendChild(emptyState("Unable to load agents."));
        setStatus(err.message || "Failed to load agents", "error");
      }
    }

    createForm.addEventListener("submit", async (evt) => {
      evt.preventDefault();
      const name = (createName.value || "").trim();
      const runtime = (createRuntime.value || "").toLowerCase();
      if (!name) {
        setStatus("Name is required", "error");
        return;
      }
      if (!AGENT_RUNTIMES.includes(runtime)) {
        setStatus("Runtime must be codex or copilot", "error");
        return;
      }
      setAgentFormState(createForm, true);
      try {
        setStatus(`Creating ${name}…`, "loading");
        await fetchJSON("/agent", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, runtime }),
        });
        createName.value = "";
        createRuntime.value = AGENT_RUNTIMES[0];
        await loadAgents(false);
        setStatus("Agent created", "success");
      } catch (err) {
        setStatus(err.message || "Failed to create agent", "error");
      } finally {
        setAgentFormState(createForm, false);
      }
    });

    editCancel.addEventListener("click", () => setEditing(null));

    editForm.addEventListener("submit", async (evt) => {
      evt.preventDefault();
      if (!editingId) {
        setStatus("Select an agent to edit", "error");
        return;
      }
      const name = (editName.value || "").trim();
      const runtime = (editRuntime.value || "").toLowerCase();
      if (!name) {
        setStatus("Name is required", "error");
        return;
      }
      if (!AGENT_RUNTIMES.includes(runtime)) {
        setStatus("Runtime must be codex or copilot", "error");
        return;
      }
      setAgentFormState(editForm, true);
      try {
        setStatus(`Updating ${name}…`, "loading");
        await fetchJSON(`/agent/${encodeURIComponent(editingId)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, runtime }),
        });
        await loadAgents(false);
        setStatus("Agent updated", "success");
      } catch (err) {
        setStatus(err.message || "Failed to update agent", "error");
      } finally {
        setAgentFormState(editForm, false);
      }
    });

    await loadAgents(false);
  }

  async function renderRepositoriesView() {
    viewTitle.textContent = "Repositories";
    renderBreadcrumbs([{ label: "Repositories" }]);
    setStatus("Loading repositories…", "loading");
    clearContent();

    let repositories = [];
    let editingId = "";

    const container = document.createElement("div");
    container.className = "repositories-view";

    const createForm = document.createElement("form");
    createForm.className = "card";
    const createHeader = document.createElement("h3");
    createHeader.textContent = "Create repository";
    createForm.appendChild(createHeader);
    const createName = document.createElement("input");
    createName.type = "text";
    createName.placeholder = "Name";
    const createPath = document.createElement("input");
    createPath.type = "text";
    createPath.placeholder = "/absolute/path/to/repo";
    const createHint = document.createElement("div");
    createHint.className = "pill";
    createHint.textContent = "Path must be absolute (starts with /)";
    const createSubmit = document.createElement("button");
    createSubmit.type = "submit";
    createSubmit.textContent = "Create";
    createForm.appendChild(createName);
    createForm.appendChild(createPath);
    createForm.appendChild(createHint);
    createForm.appendChild(createSubmit);

    const editForm = document.createElement("form");
    editForm.className = "card";
    const editHeader = document.createElement("h3");
    editHeader.textContent = "Edit repository";
    editForm.appendChild(editHeader);
    const editHint = document.createElement("div");
    editHint.className = "pill";
    editHint.textContent = "Select a repository to edit";
    editForm.appendChild(editHint);
    const editName = document.createElement("input");
    editName.type = "text";
    editName.placeholder = "Name";
    const editPath = document.createElement("input");
    editPath.type = "text";
    editPath.placeholder = "/absolute/path/to/repo";
    const editPathHint = document.createElement("div");
    editPathHint.className = "pill";
    editPathHint.textContent = "Path must be absolute (starts with /)";
    const editSubmit = document.createElement("button");
    editSubmit.type = "submit";
    editSubmit.textContent = "Update";
    editSubmit.disabled = true;
    const editCancel = document.createElement("button");
    editCancel.type = "button";
    editCancel.textContent = "Clear";
    editCancel.disabled = true;
    editForm.appendChild(editName);
    editForm.appendChild(editPath);
    editForm.appendChild(editPathHint);
    editForm.appendChild(editSubmit);
    editForm.appendChild(editCancel);

    const tableContainer = document.createElement("div");
    tableContainer.className = "card";

    container.appendChild(createForm);
    container.appendChild(editForm);
    container.appendChild(tableContainer);
    contentEl.appendChild(container);

    function setEditing(repo) {
      editingId = repo?.repository_id || "";
      if (editingId) {
        editHint.textContent = `Editing ${repo.name || editingId}`;
        editName.value = repo.name || "";
        editPath.value = repo.path || "";
        editSubmit.disabled = false;
        editCancel.disabled = false;
      } else {
        editHint.textContent = "Select a repository to edit";
        editName.value = "";
        editPath.value = "";
        editSubmit.disabled = true;
        editCancel.disabled = true;
      }
    }

    function renderTable(list) {
      tableContainer.innerHTML = "";
      const rows = (list || []).map((repo) => {
        const actions = document.createElement("div");
        actions.className = "actions";

        const editBtn = document.createElement("button");
        editBtn.type = "button";
        editBtn.textContent = "Edit";
        editBtn.addEventListener("click", () => setEditing(repo));
        actions.appendChild(editBtn);

        const deleteBtn = document.createElement("button");
        deleteBtn.type = "button";
        deleteBtn.textContent = "Delete";
        deleteBtn.addEventListener("click", async () => {
          try {
            setStatus(`Deleting ${repo.name || repo.repository_id}…`, "loading");
            await fetchJSON(`/repository/${encodeURIComponent(repo.repository_id)}`, {
              method: "DELETE",
            });
            if (repo.repository_id === editingId) {
              setEditing(null);
            }
            await loadRepositories(false);
            setStatus("Repository deleted", "success");
          } catch (err) {
            setStatus(err.message || "Failed to delete repository", "error");
          }
        });
        actions.appendChild(deleteBtn);

        return [
          repo?.repository_id || "—",
          repo?.name || "—",
          repo?.path || "—",
          formatDate(repo?.created_at),
          formatDate(repo?.updated_at),
          actions,
        ];
      });

      if (!rows.length) {
        tableContainer.appendChild(emptyState("No repositories found."));
        return;
      }

      tableContainer.appendChild(
        buildTable(
          ["Repository ID", "Name", "Path", "Created", "Updated", "Actions"],
          rows
        )
      );
    }

    async function loadRepositories(showStatus = true) {
      try {
        if (showStatus) {
          setStatus("Loading repositories…", "loading");
        }
        repositories = await fetchRepositoriesList();
        if (editingId && !repositories.find((r) => r.repository_id === editingId)) {
          setEditing(null);
        }
        renderTable(repositories);
        const count = repositories.length;
        setStatus(`Loaded ${count} repositor${count === 1 ? "y" : "ies"}`, "success");
      } catch (err) {
        tableContainer.innerHTML = "";
        tableContainer.appendChild(emptyState("Unable to load repositories."));
        setStatus(err.message || "Failed to load repositories", "error");
      }
    }

    function validateRepositoryInput(name, path) {
      if (!name) {
        setStatus("Name is required", "error");
        return false;
      }
      if (!path || !path.startsWith("/")) {
        setStatus("Path must be absolute (starts with /)", "error");
        return false;
      }
      return true;
    }

    createForm.addEventListener("submit", async (evt) => {
      evt.preventDefault();
      const name = (createName.value || "").trim();
      const path = (createPath.value || "").trim();
      if (!validateRepositoryInput(name, path)) return;
      setRepositoryFormState(createForm, true);
      try {
        setStatus(`Creating ${name}…`, "loading");
        await fetchJSON("/repository", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, path }),
        });
        createName.value = "";
        createPath.value = "";
        await loadRepositories(false);
        setStatus("Repository created", "success");
      } catch (err) {
        setStatus(err.message || "Failed to create repository", "error");
      } finally {
        setRepositoryFormState(createForm, false);
      }
    });

    editCancel.addEventListener("click", () => setEditing(null));

    editForm.addEventListener("submit", async (evt) => {
      evt.preventDefault();
      if (!editingId) {
        setStatus("Select a repository to edit", "error");
        return;
      }
      const name = (editName.value || "").trim();
      const path = (editPath.value || "").trim();
      if (!validateRepositoryInput(name, path)) return;
      setRepositoryFormState(editForm, true);
      try {
        setStatus(`Updating ${name}…`, "loading");
        await fetchJSON(`/repository/${encodeURIComponent(editingId)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, path }),
        });
        await loadRepositories(false);
        setStatus("Repository updated", "success");
      } catch (err) {
        setStatus(err.message || "Failed to update repository", "error");
      } finally {
        setRepositoryFormState(editForm, false);
      }
    });

    await loadRepositories(false);
  }

  async function fetchRepositoryBatches(repoId) {
    if (!repoId) return [];
    const data = await fetchJSON(
      `/repository/${encodeURIComponent(repoId)}/batches?limit=${LIST_LIMIT}`
    );
    if (Array.isArray(data?.batches)) return data.batches;
    return [];
  }

  async function fetchBatchItems(batch) {
    if (Array.isArray(batch?.items)) return batch.items;
    try {
      const detail = await fetchBatchDetail(batch?.batch_id);
      if (Array.isArray(detail?.items)) return detail.items;
    } catch (err) {
      console.warn("Failed to fetch batch detail", err);
    }
    return [];
  }

  function buildTasksContent(items) {
    const container = document.createElement("div");
    container.className = "tasks-section";

    const countPill = document.createElement("div");
    countPill.className = "pill";
    countPill.textContent = `${items.length} task${items.length === 1 ? "" : "s"}`;
    container.appendChild(countPill);

    if (!items.length) {
      container.appendChild(emptyState("No tasks found for this batch."));
      return container;
    }

    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "details-toggle";
    toggle.textContent = "Show tasks";
    toggle.setAttribute("aria-expanded", "false");

    const scroll = document.createElement("div");
    scroll.className = "tasks-scroll";
    scroll.style.maxHeight = "240px";
    scroll.style.overflowY = "auto";
    scroll.hidden = true;

    const rows = items.map((item, idx) => [
      idx + 1,
      item?.input || "—",
      item?.status || "unknown",
      Number.isFinite(item?.attempts) ? item.attempts : "—",
    ]);
    scroll.appendChild(
      buildTable(["#", "Task", "Status", "Attempts"], rows)
    );

    toggle.addEventListener("click", () => {
      const willShow = scroll.hidden;
      scroll.hidden = !willShow;
      toggle.textContent = `${willShow ? "Hide" : "Show"} tasks`;
      toggle.setAttribute("aria-expanded", String(willShow));
    });

    container.appendChild(toggle);
    container.appendChild(scroll);
    return container;
  }

  function buildRestartAction(repoId, batch, reloadRepoBatches) {
    const actions = document.createElement("div");
    actions.className = "actions";
    const status = String(batch?.status || "").toLowerCase();
    if (status !== "failed") {
      const pill = document.createElement("div");
      pill.className = "pill";
      pill.textContent = "Restart available on failed batches only";
      actions.appendChild(pill);
      return actions;
    }

    const restartBtn = document.createElement("button");
    restartBtn.type = "button";
    restartBtn.textContent = "Restart";
    restartBtn.addEventListener("click", async () => {
      if (restartBtn.disabled) return;
      restartBtn.disabled = true;
      try {
        setStatus(`Restarting ${batch.batch_id}…`, "loading");
        await fetchJSON(
          `/repository/${encodeURIComponent(repoId)}/batch/${encodeURIComponent(
            batch.batch_id
          )}/restart`,
          { method: "PUT" }
        );
        setStatus(`Restarted batch ${batch.batch_id}`, "success");
        await reloadRepoBatches();
      } catch (err) {
        setStatus(err.message || `Failed to restart ${batch.batch_id}`, "error");
      } finally {
        restartBtn.disabled = false;
      }
    });
    actions.appendChild(restartBtn);
    return actions;
  }

  async function renderControlPlaneView() {
    viewTitle.textContent = "Control Plane";
    renderBreadcrumbs([{ label: "Control Plane" }]);
    setStatus("Loading control plane…", "loading");
    clearContent();

    let repositories = [];
    try {
      repositories = await fetchRepositoriesList();
    } catch (err) {
      contentEl.appendChild(emptyState("Unable to load repositories."));
      setStatus(err.message || "Failed to load control plane", "error");
      return;
    }

    if (!repositories.length) {
      contentEl.appendChild(emptyState("No repositories found."));
      setStatus("No repositories available", "warning");
      return;
    }

    const container = document.createElement("div");
    container.className = "control-plane";
    contentEl.appendChild(container);

    for (const repo of repositories) {
      const card = document.createElement("div");
      card.className = "card";
      const header = document.createElement("div");
      header.className = "view-header";
      const title = document.createElement("h3");
      title.textContent = repo?.name || repo?.repository_id || "Repository";
      const repoIdEl = document.createElement("div");
      repoIdEl.className = "pill";
      repoIdEl.textContent = repo?.repository_id || "—";
      header.appendChild(title);
      header.appendChild(repoIdEl);
      card.appendChild(header);
      const body = document.createElement("div");
      card.appendChild(body);
      container.appendChild(card);

      const reloadBatches = async () => {
        body.innerHTML = "";
        let batches = [];
        try {
          batches = await fetchRepositoryBatches(repo.repository_id);
        } catch (err) {
          body.appendChild(emptyState("Unable to load batches for repository."));
          setStatus(err.message || "Failed to load repository batches", "error");
          return;
        }

        if (!batches.length) {
          body.appendChild(emptyState("No batches for this repository."));
          return;
        }

        const rows = batches.map((batch) => {
          const tasksCell = document.createElement("div");
          tasksCell.textContent = "Loading tasks…";
          fetchBatchItems(batch)
            .then((items) => {
              tasksCell.innerHTML = "";
              tasksCell.appendChild(buildTasksContent(items));
            })
            .catch(() => {
              tasksCell.innerHTML = "";
              tasksCell.appendChild(emptyState("Unable to load tasks for batch."));
            });

          return [
            createLink(
              `/app?batch=${encodeURIComponent(batch.batch_id)}`,
              batch.batch_id || "—"
            ),
            tasksCell,
            batch?.status || "unknown",
            buildRestartAction(repo.repository_id, batch, reloadBatches),
          ];
        });

        body.appendChild(
          buildTable(["Batch ID", "Tasks", "Status", "Actions"], rows)
        );
      };

      await reloadBatches();
    }

    setStatus(`Loaded ${repositories.length} repositories`, "success");
  }

  function buildTable(headers, rows) {
    const table = document.createElement("table");
    const thead = document.createElement("thead");
    const tr = document.createElement("tr");
    headers.forEach((h) => {
      const th = document.createElement("th");
      th.textContent = h;
      tr.appendChild(th);
    });
    thead.appendChild(tr);
    table.appendChild(thead);

    const tbody = document.createElement("tbody");
    rows.forEach((row) => {
      const trRow = document.createElement("tr");
      row.forEach((cell) => {
        const td = document.createElement("td");
        if (cell instanceof Node) {
          td.appendChild(cell);
        } else {
          td.textContent = cell;
        }
        trRow.appendChild(td);
      });
      tbody.appendChild(trRow);
    });
    table.appendChild(tbody);
    return table;
  }

  function renderVersionBody(data) {
    const container = document.createElement("div");
    container.className = "meta-grid";

    if (data && typeof data === "object") {
      const entries = Object.entries(data);
      if (!entries.length) {
        const empty = document.createElement("div");
        empty.textContent = "No version information returned.";
        container.appendChild(empty);
        return container;
      }
      entries.forEach(([key, value]) => {
        const row = document.createElement("div");
        row.className = "version-row";
        const label = document.createElement("div");
        label.className = "pill";
        label.textContent = key;
        const text = document.createElement("div");
        text.textContent =
          value === null || value === undefined
            ? "—"
            : typeof value === "object"
              ? JSON.stringify(value, null, 2)
              : String(value);
        row.appendChild(label);
        row.appendChild(text);
        container.appendChild(row);
      });
      return container;
    }

    const text = document.createElement("div");
    text.textContent =
      data === null || data === undefined || data === ""
        ? "No version information returned."
        : String(data);
    container.appendChild(text);
    return container;
  }

  async function renderVersionView() {
    resetLiveStream("switching view");
    viewTitle.textContent = "Version";
    renderBreadcrumbs([{ label: "Version" }]);
    clearContent();
    setStatus("Loading version…", "loading");
    try {
      const data = await fetchVersionData();
      const card = document.createElement("div");
      card.className = "card";

      const title = document.createElement("div");
      title.className = "view-header";
      const heading = document.createElement("h3");
      heading.textContent = "Version detail";
      title.appendChild(heading);
      card.appendChild(title);

      card.appendChild(renderVersionBody(data));
      contentEl.appendChild(card);
      setStatus("Version loaded", "success");
    } catch (err) {
      contentEl.appendChild(emptyState("Unable to load version info."));
      setStatus(err?.message || "Failed to load version", "error");
    }
  }

  function getEventType(evt) {
    return (evt && (evt.event?.type || evt.type)) || "unknown";
  }

  function getEventMeta(type) {
    const meta = EVENT_META[type];
    return meta ? { ...DEFAULT_EVENT_META, ...meta } : { ...DEFAULT_EVENT_META };
  }

  function shouldRenderEvent(evt) {
    const meta = getEventMeta(getEventType(evt));
    return !meta.hidden;
  }

  function filterRenderableEvents(events) {
    return (events || []).filter(shouldRenderEvent);
  }

  const mergeHelpers = window.LiveEventsMerge || {};
  const reconnectHelpers = window.LiveReconnect || {};

  const getIngestedAt =
    mergeHelpers.getIngestedAt ||
    function getIngestedAtFallback(evt) {
      return evt?.ingested_at || evt?.ingestedAt || "";
    };

  const eventKeyFromEvent =
    mergeHelpers.eventKeyFromEvent ||
    function eventKeyFromEventFallback(evt) {
      return [
        evt?.session_id || evt?.sessionId || "",
        evt?.run_id || evt?.runId || "",
        evt?.batch_id || evt?.batchId || "",
        getIngestedAt(evt) || "",
      ].join("|");
    };

  const deriveLatestIngested =
    mergeHelpers.deriveLatestIngested ||
    function deriveLatestIngestedFallback(events) {
      let latest = "";
      for (const evt of events || []) {
        const ts = getIngestedAt(evt);
        if (!ts) continue;
        const tsDate = new Date(ts);
        const latestDate = latest ? new Date(latest) : null;
        if (!latest || (latestDate && tsDate > latestDate)) {
          latest = ts;
        }
      }
      return latest;
    };

  function getEventData(evt) {
    return (evt && (evt.event?.data || evt.data)) || {};
  }

  function getRawEvent(evt) {
    return (evt && (evt.event || evt)) || {};
  }

  function formatUsageSummary(data) {
    if (!data) return "";
    const parts = [];
    if (Number.isFinite(data.inputTokens)) parts.push(`in ${data.inputTokens}`);
    if (Number.isFinite(data.outputTokens)) parts.push(`out ${data.outputTokens}`);
    if (Number.isFinite(data.cacheReadTokens))
      parts.push(`cache ${data.cacheReadTokens}`);
    if (Number.isFinite(data.cacheWriteTokens))
      parts.push(`cache write ${data.cacheWriteTokens}`);
    if (Number.isFinite(data.cost)) parts.push(`$${data.cost.toFixed(4)}`);
    return parts.join(" · ");
  }

  function formatUsageInfoSummary(data) {
    if (!data) return "";
    const parts = [];
    if (Number.isFinite(data.currentTokens)) parts.push(`current ${data.currentTokens}`);
    if (Number.isFinite(data.tokenLimit)) parts.push(`limit ${data.tokenLimit}`);
    if (Number.isFinite(data.messagesLength))
      parts.push(`messages ${data.messagesLength}`);
    return parts.join(" · ");
  }

  function formatTruncationSummary(data) {
    if (!data) return "";
    const parts = [];
    if (Number.isFinite(data.eventsRemoved)) parts.push(`events removed ${data.eventsRemoved}`);
    if (Number.isFinite(data.tokensRemoved)) parts.push(`tokens removed ${data.tokensRemoved}`);
    if (data.upToEventId) parts.push(`up to ${data.upToEventId}`);
    return parts.join(" · ");
  }

  function formatToolSummary(data) {
    if (!data) return "";
    const name = data.toolName || data.mcpToolName || data.mcpServerName || data.name;
    const callId = data.toolCallId;
    const parts = [];
    if (name) parts.push(name);
    if (callId) parts.push(`(${callId})`);
    if (data.result?.content) parts.push(data.result.content);
    if (typeof data.partialOutput === "string") parts.push(data.partialOutput);
    if (typeof data.progressMessage === "string") parts.push(data.progressMessage);
    if (data.arguments && typeof data.arguments === "string") {
      parts.push(data.arguments.slice(0, 80));
    }
    return parts.join(" ").trim();
  }

  function buildEventContent(evt, meta) {
    if (typeof meta.render === "function") {
      return meta.render(evt);
    }

    const summary = formatEventSummary(evt, meta);
    if (summary) {
      const summaryEl = document.createElement("div");
      summaryEl.className = "event-summary";
      summaryEl.textContent = summary;
      return summaryEl;
    }
    return null;
  }

  function resolveMessageText(data) {
    const raw = data?.content ?? data?.transformedContent;
    if (Array.isArray(raw)) {
      return raw
        .map((part) => {
          if (typeof part === "string") return part;
          if (part && typeof part.text === "string") return part.text;
          return "";
        })
        .join("");
    }
    return raw || "";
  }

  function createChatBubble(text, variant, fallbackText = "(no content)") {
    const bubble = document.createElement("div");
    bubble.className = ["chat-bubble", variant].filter(Boolean).join(" ");
    bubble.textContent = text?.trim() || fallbackText;
    return bubble;
  }

  function createMetricChip(label, value) {
    if (value === null || value === undefined || value === "") return null;
    const chip = document.createElement("span");
    chip.className = "pill";
    chip.textContent = `${label}: ${value}`;
    return chip;
  }

  function renderMetricsRow(chips, fallbackText) {
    const row = document.createElement("div");
    row.className = "event-content event-meta";
    const validChips = chips.filter(Boolean);
    if (!validChips.length) {
      row.textContent = fallbackText;
      row.classList.add("event-summary");
      return row;
    }
    validChips.forEach((chip) => row.appendChild(chip));
    return row;
  }

  function formatNumber(value) {
    return Number.isFinite(value) ? value.toLocaleString() : null;
  }

  function formatCurrency(value) {
    return Number.isFinite(value) ? `$${value.toFixed(4)}` : null;
  }

  function formatDurationMs(value) {
    if (!Number.isFinite(value)) return null;
    if (value >= 1000) return `${(value / 1000).toFixed(2)}s`;
    return `${value}ms`;
  }

  function renderUnknownEvent(evt) {
    const type = getEventType(evt);
    const data = getEventData(evt);
    const keys =
      data && typeof data === "object" ? Object.keys(data).filter(Boolean) : [];
    const info = [];
    if (type && type !== "unknown") info.push(`Type: ${type}`);
    if (keys.length) info.push(`Fields: ${keys.slice(0, 5).join(", ")}`);
    const summary = info.join(" • ") || "Unrecognized event";
    return createChatBubble(summary, "chat-system", "Unknown event");
  }

  function renderUserMessage(evt) {
    const data = getEventData(evt);
    return createChatBubble(resolveMessageText(data), "chat-user");
  }

  function renderSystemMessage(evt) {
    const data = getEventData(evt);
    return createChatBubble(
      resolveMessageText(data),
      "chat-system",
      "(system message not provided)"
    );
  }

  function renderPendingMessage() {
    return createChatBubble(
      "",
      "chat-pending",
      "Pending messages updated"
    );
  }

  function renderAssistantIntent(evt) {
    const data = getEventData(evt);
    return createChatBubble(data?.intent, "chat-intent", "(intent not provided)");
  }

  function renderAssistantReasoning(evt) {
    const data = getEventData(evt);
    const text = data?.content || data?.reasoningText;
    return createChatBubble(text, "chat-assistant", "(reasoning not provided)");
  }

  function renderAssistantMessage(evt) {
    const data = getEventData(evt);
    const text = resolveMessageText(data);
    const fallback =
      data?.encryptedContent
        ? "Encrypted message (content not available)"
        : "(no content)";
    return createChatBubble(text, "chat-assistant", fallback);
  }

  function formatToolName(data) {
    return (
      data?.toolName ||
      data?.mcpToolName ||
      data?.mcpServerName ||
      data?.name ||
      ""
    );
  }

  function truncateText(text, limit = 200) {
    if (text === null || text === undefined) return "";
    const str = String(text);
    return str.length > limit ? `${str.slice(0, limit - 1)}…` : str;
  }

  function parseFrontMatter(text) {
    if (typeof text !== "string") return { meta: {}, body: "" };
    const source = text.startsWith("\ufeff") ? text.slice(1) : text;
    if (!source.startsWith("---\n")) return { meta: {}, body: source };
    const end = source.indexOf("\n---", 4);
    if (end === -1) return { meta: {}, body: source };
    const meta = {};
    const metaBlock = source.slice(4, end);
    metaBlock.split("\n").forEach((line) => {
      const idx = line.indexOf(":");
      if (idx === -1) return;
      const key = line.slice(0, idx).trim();
      const value = line.slice(idx + 1).trim();
      if (key) meta[key] = value;
    });
    const body = source.slice(end + 4);
    return { meta, body };
  }

  function extractFirstNonEmptyLine(text, skipValue) {
    if (!text) return "";
    const lines = text.split(/\r?\n/);
    for (const line of lines) {
      const value = line.trim();
      if (!value) continue;
      if (skipValue && value === skipValue.trim()) {
        skipValue = null;
        continue;
      }
      return value;
    }
    return "";
  }

  function stringifyCompact(value, limit = 200) {
    if (value === null || value === undefined) return "";
    if (typeof value === "string") return truncateText(value, limit);
    try {
      return truncateText(JSON.stringify(value), limit);
    } catch (_) {
      return "";
    }
  }

  function summarizeToolArguments(args, limit = 200) {
    if (args === null || args === undefined) return "(no arguments)";
    const summary = stringifyCompact(args, limit);
    return summary || "(no arguments)";
  }

  function summarizeToolResult(data, limit = 200) {
    if (!data) return "(no result)";
    if (data.error) {
      if (typeof data.error === "string") return `Error: ${truncateText(data.error, limit)}`;
      if (typeof data.error.message === "string")
        return `Error: ${truncateText(data.error.message, limit)}`;
    }
    if (data.result?.content) return truncateText(data.result.content, limit);
    if (data.result?.detailedContent) return truncateText(data.result.detailedContent, limit);
    if (data.output) return stringifyCompact(data.output, limit);
    if (data.partialOutput) return stringifyCompact(data.partialOutput, limit);
    if (data.progressMessage) return truncateText(data.progressMessage, limit);
    return "(no result)";
  }

  function renderToolExecutionStart(evt) {
    const data = getEventData(evt);
    const name = formatToolName(data) || "Tool execution";
    const lines = [
      [name, data?.toolCallId ? `Call ${data.toolCallId}` : ""].filter(Boolean).join(" • "),
      `Args: ${summarizeToolArguments(data?.arguments, 180)}`,
    ].filter(Boolean);
    return createChatBubble(lines.join("\n"), "chat-system", "Tool call started");
  }

  function renderToolExecutionComplete(evt) {
    const data = getEventData(evt);
    const name = formatToolName(data) || "Tool execution";
    const status = data?.error ? "Failed" : "Complete";
    const lines = [
      [name, status, data?.toolCallId ? `Call ${data.toolCallId}` : ""]
        .filter(Boolean)
        .join(" • "),
      summarizeToolResult(data, 200),
    ].filter(Boolean);
    return createChatBubble(lines.join("\n"), "chat-system", "Tool call complete");
  }

  function renderSkillInvoked(evt) {
    const data = getEventData(evt);
    const rawContent = typeof data?.content === "string" ? data.content : "";
    const { meta, body } = parseFrontMatter(rawContent);
    const name = data?.name || meta.name || extractFirstNonEmptyLine(body);
    const description =
      data?.description ||
      meta.description ||
      extractFirstNonEmptyLine(body, name);
    const lines = [];
    if (name) lines.push(truncateText(name, 120));
    if (description) lines.push(truncateText(description, 200));
    if (!lines.length && rawContent) {
      lines.push(truncateText(extractFirstNonEmptyLine(rawContent), 200));
    }
    return createChatBubble(lines.join("\n"), "chat-system", "Skill invoked");
  }

  function renderTurnMarker(evt, label) {
    const data = getEventData(evt);
    const turnId = data?.turnId;
    const parts = [label];
    if (turnId !== undefined && turnId !== null && String(turnId).trim() !== "") {
      parts.push(`Turn ${turnId}`);
    }
    return createChatBubble(parts.join(" • "), "chat-system", label);
  }

  function renderAssistantUsage(evt) {
    const data = getEventData(evt);
    const chips = [
      createMetricChip("Input", formatNumber(data?.inputTokens)),
      createMetricChip("Output", formatNumber(data?.outputTokens)),
      createMetricChip("Cache read", formatNumber(data?.cacheReadTokens)),
      createMetricChip("Cache write", formatNumber(data?.cacheWriteTokens)),
      createMetricChip("Cost", formatCurrency(data?.cost)),
      createMetricChip("Duration", formatDurationMs(data?.duration)),
    ];
    return renderMetricsRow(chips, "No usage metrics provided");
  }

  function renderSessionUsageInfo(evt) {
    const data = getEventData(evt);
    const chips = [
      createMetricChip("Current", formatNumber(data?.currentTokens)),
      createMetricChip("Limit", formatNumber(data?.tokenLimit)),
      createMetricChip("Messages", formatNumber(data?.messagesLength)),
    ];
    return renderMetricsRow(chips, "No usage info provided");
  }

  function renderSessionIdle() {
    const summary = document.createElement("div");
    summary.className = "event-summary";
    summary.textContent = "Idle heartbeat";
    return summary;
  }

  function formatCodexItemSummary(evt) {
    const raw = getRawEvent(evt);
    const item = raw?.item || {};
    const itemType = item?.type || "item";
    switch (itemType) {
      case "agent_message":
      case "reasoning": {
        const text = truncateText(item?.text || "", 160);
        return text ? `${itemType} • ${text}` : itemType;
      }
      case "command_execution": {
        const command = truncateText(item?.command || "", 120);
        const status = item?.status || "";
        const exitCode =
          item?.exit_code === null || item?.exit_code === undefined
            ? ""
            : `exit ${item.exit_code}`;
        return [itemType, status, exitCode, command].filter(Boolean).join(" • ");
      }
      case "file_change": {
        const changes = Array.isArray(item?.changes) ? item.changes : [];
        if (!changes.length) return itemType;
        const firstPath = changes[0]?.path ? truncateText(changes[0].path, 80) : "";
        return [itemType, `${changes.length} change(s)`, firstPath].filter(Boolean).join(" • ");
      }
      default:
        return itemType;
    }
  }

  function renderCodexThreadStarted(evt) {
    const raw = getRawEvent(evt);
    const threadId = raw?.thread_id || "(no thread id)";
    return createChatBubble(`Thread ${threadId}`, "chat-system", "Thread started");
  }

  function renderCodexTurnStarted() {
    return createChatBubble("Turn started", "chat-system", "Turn started");
  }

  function renderCodexTurnCompleted(evt) {
    const raw = getRawEvent(evt);
    const usage = raw?.usage || {};
    const chips = [
      createMetricChip("Input", formatNumber(usage?.input_tokens)),
      createMetricChip("Output", formatNumber(usage?.output_tokens)),
      createMetricChip("Cached input", formatNumber(usage?.cached_input_tokens)),
    ];
    return renderMetricsRow(chips, "Turn completed");
  }

  function renderCodexItemStarted(evt) {
    const raw = getRawEvent(evt);
    const item = raw?.item || {};
    if (item?.type === "command_execution") {
      const command = truncateText(item?.command || "", 220);
      const text = [item.type, item?.status || "in_progress", command]
        .filter(Boolean)
        .join("\n");
      return createChatBubble(text, "chat-system", "Command started");
    }
    return createChatBubble(formatCodexItemSummary(evt), "chat-system", "Item started");
  }

  function renderCodexItemCompleted(evt) {
    const raw = getRawEvent(evt);
    const item = raw?.item || {};

    if (item?.type === "agent_message") {
      return createChatBubble(item?.text || "", "chat-assistant", "(no content)");
    }
    if (item?.type === "reasoning") {
      return createChatBubble(item?.text || "", "chat-assistant", "(reasoning not provided)");
    }
    if (item?.type === "command_execution") {
      const command = truncateText(item?.command || "", 220);
      const exitCode =
        item?.exit_code === null || item?.exit_code === undefined
          ? ""
          : `exit ${item.exit_code}`;
      const text = [
        item.type,
        item?.status || "completed",
        exitCode,
        command,
      ]
        .filter(Boolean)
        .join("\n");
      return createChatBubble(text, "chat-system", "Command completed");
    }
    if (item?.type === "file_change") {
      const changes = Array.isArray(item?.changes) ? item.changes : [];
      const lines = [`File changes: ${changes.length}`];
      changes.slice(0, 5).forEach((change) => {
        lines.push(
          [change?.kind || "change", truncateText(change?.path || "", 140)]
            .filter(Boolean)
            .join(" • ")
        );
      });
      return createChatBubble(lines.join("\n"), "chat-system", "File changes");
    }

    return createChatBubble(formatCodexItemSummary(evt), "chat-system", "Item completed");
  }

  function formatEventSummary(evt, meta) {
    const data = evt?.event?.data || {};
    if (typeof meta.summary === "function") {
      const summary = meta.summary(data, evt);
      if (summary) return summary;
    }
    return "";
  }

  function createDetailsSection(contentNode) {
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "details-toggle";
    toggle.textContent = "Show details";
    toggle.setAttribute("aria-expanded", "false");

    const details = document.createElement("div");
    details.className = "event-details";
    details.hidden = true;
    details.appendChild(contentNode);

    toggle.addEventListener("click", () => {
      const willShow = details.hidden;
      details.hidden = !willShow;
      toggle.textContent = willShow ? "Hide details" : "Show details";
      toggle.setAttribute("aria-expanded", String(willShow));
    });

    return { toggle, details };
  }

  async function renderBatches() {
    viewTitle.textContent = "Batches";
    renderBreadcrumbs([]);
    setStatus("Loading batches…", "loading");
    clearContent();

    try {
      const data = await fetchJSON(`/batches?limit=${LIST_LIMIT}`);
      const batches = data.batches || [];
      if (!batches.length) {
        contentEl.appendChild(emptyState("No batches yet."));
        setStatus("No batches found", "warning");
        return;
      }

      const rows = batches.map((b) => [
        createLink(`/app?batch=${encodeURIComponent(b.batch_id)}`, b.batch_id),
        b.session_name || "—",
        b.status || "unknown",
        formatDate(b.created_at),
      ]);
      contentEl.appendChild(
        buildTable(["Batch", "Session", "Status", "Created"], rows)
      );
      setStatus(`Showing ${data.count ?? batches.length} batches`, "success");
    } catch (err) {
      clearContent();
      contentEl.appendChild(emptyState("Unable to load batches."));
      setStatus(err.message || "Failed to load batches", "error");
    }
  }

  function buildRunsTable(batchId, runs) {
    const rows = (runs || []).map((run) => [
      createLink(
        `/app?batch=${encodeURIComponent(batchId)}&run=${encodeURIComponent(
          run.run_id
        )}`,
        run.run_id
      ),
      run.task_ref || "—",
      run.status || "unknown",
      Number.isFinite(Number(run.total_events))
        ? Number(run.total_events)
        : run.total_events ?? "—",
      formatDate(run.started_at),
      formatDate(run.finished_at),
    ]);

    return buildTable(
      ["Run", "Task Ref", "Status", "Total Events", "Started", "Finished"],
      rows
    );
  }

  async function renderRuns(batchId) {
    setStatus("Loading runs…", "loading");
    clearContent();

    let batchDetail;
    try {
      batchDetail = await fetchBatchDetail(batchId);
    } catch (err) {
      contentEl.appendChild(emptyState("Unable to load batch details."));
      setStatus(err.message || "Failed to load runs", "error");
      return;
    }

    const repoId = batchDetail?.repository_id;
    if (!repoId) {
      contentEl.appendChild(emptyState("Batch is missing repository context."));
      setStatus("Unable to load runs (no repository id)", "error");
      return;
    }

    const batchLabel = batchDetail.session_name || batchDetail.batch_id || batchId;
    viewTitle.textContent = `Runs for ${batchLabel}`;
    renderBreadcrumbs([
      { label: "Batches", href: "/app" },
      { label: batchLabel },
    ]);
    clearContent();

    const actions = document.createElement("div");
    actions.className = "actions";
    actions.appendChild(createButtonLink("/app", "Back to batches"));
    contentEl.appendChild(actions);

    try {
      const data = await fetchJSON(
        `/repository/${encodeURIComponent(
          repoId
        )}/batch/${encodeURIComponent(batchId)}/runs?limit=${LIST_LIMIT}`
      );
      const runs = data.runs || [];
      if (!runs.length) {
        contentEl.appendChild(emptyState("No runs for this batch."));
        setStatus("No runs found", "warning");
        return;
      }

      contentEl.appendChild(buildRunsTable(batchId, runs));
      setStatus(`Showing ${data.count ?? runs.length} runs`, "success");
    } catch (err) {
      contentEl.appendChild(emptyState("Unable to load runs."));
      setStatus(err.message || "Failed to load runs", "error");
    }
  }

  function renderRunMeta(run) {
    const grid = document.createElement("div");
    grid.className = "meta-grid";

    const fields = [
      ["Run ID", run.run_id],
      ["Batch ID", run.batch_id],
      ["Task Ref", run.task_ref || "—"],
      ["Epic Ref", run.parent_ref || "—"],
      ["Status", run.status || "unknown"],
      ["Session ID", run.session_id || "—"],
      ["Started", formatDate(run.started_at)],
      ["Finished", formatDate(run.finished_at)],
    ];

    fields.forEach(([label, value]) => {
      const div = document.createElement("div");
      const strong = document.createElement("strong");
      strong.textContent = `${label}: `;
      div.appendChild(strong);
      div.appendChild(document.createTextNode(value ?? "—"));
      grid.appendChild(div);
    });

    return grid;
  }

  function computeRunHeaderOffset() {
    if (typeof document === "undefined") return 0;
    const pageHeader = document.querySelector("body > header");
    const statusBar = document.getElementById("status");
    const headerRect = pageHeader?.getBoundingClientRect();
    const statusRect = statusBar?.getBoundingClientRect();
    const offsets = [];
    if (headerRect) offsets.push(headerRect.height);
    if (statusRect) offsets.push(statusRect.height);
    const total = offsets.reduce((sum, h) => sum + h, 0);
    return Math.max(0, total);
  }

  function isNearBottom(element, thresholdPx = SCROLL_FOLLOW_THRESHOLD_PX) {
    if (!element) return true;
    const threshold = Number.isFinite(thresholdPx) ? thresholdPx : SCROLL_FOLLOW_THRESHOLD_PX;
    const scrollTop = Number(element.scrollTop) || 0;
    const clientHeight = Number(element.clientHeight) || 0;
    const scrollHeight = Number(element.scrollHeight) || 0;
    const distance = scrollHeight - (scrollTop + clientHeight);
    return distance <= threshold;
  }

  function createScrollFollower(element, options = {}) {
    const thresholdPx = Number.isFinite(options.thresholdPx)
      ? options.thresholdPx
      : SCROLL_FOLLOW_THRESHOLD_PX;
    const onModeChange = typeof options.onModeChange === "function" ? options.onModeChange : () => {};

    let mode = "follow";

    function setMode(next) {
      if (!next || next === mode) return;
      mode = next;
      onModeChange(mode);
    }

    function computeMode() {
      return isNearBottom(element, thresholdPx) ? "follow" : "paused";
    }

    function handleScroll() {
      setMode(computeMode());
    }

    function handleContentMutated() {
      if (!element) return;
      if (mode === "follow") {
        element.scrollTop = element.scrollHeight;
      }
    }

    function cleanup() {
      if (element?.removeEventListener) {
        element.removeEventListener("scroll", handleScroll);
      }
    }

    if (element?.addEventListener) {
      element.addEventListener("scroll", handleScroll);
    }
    mode = computeMode();
    onModeChange(mode);

    return {
      handleContentMutated,
      getMode: () => mode,
      isAtBottom: () => isNearBottom(element, thresholdPx),
      cleanup,
    };
  }

  function updateLiveFollowIndicator(indicatorEl, mode) {
    if (!indicatorEl) return;
    const normalized = mode === "paused" ? "paused" : "live";
    if (indicatorEl.classList?.remove) {
      indicatorEl.classList.remove("live", "paused");
      indicatorEl.classList.add(normalized);
    } else {
      indicatorEl.className = `pill live-status ${normalized}`;
    }
    indicatorEl.textContent = normalized === "paused" ? "Paused" : "Live";
    indicatorEl.setAttribute("data-follow-mode", normalized);
    indicatorEl.title = normalized === "paused" ? "Scroll to bottom to resume live updates" : "Following new events";
  }

  function applyRunLayoutSizing(container) {
    if (!container || !container.style) return;
    const offset = computeRunHeaderOffset();
    if (typeof container.style.setProperty === "function") {
      container.style.setProperty("--run-header-offset", `${offset}px`);
    } else {
      container.style["--run-header-offset"] = `${offset}px`;
    }
    const scroll = container.querySelector(".events-scroll");
    if (!scroll || !scroll.style) return;
    const viewportHeight = window.innerHeight || 0;
    const minHeight = Math.max(240, Math.round(viewportHeight * 0.4));
    scroll.style.minHeight = `${minHeight}px`;
    scroll.style.overflowY = "auto";
  }

  function attachRunLayout(container) {
    applyRunLayoutSizing(container);
    const handler = () => applyRunLayoutSizing(container);
    window.addEventListener("resize", handler);
    return () => window.removeEventListener("resize", handler);
  }

  function renderEventsList(events) {
    const wrapper = document.createElement("div");
    wrapper.className = "events";

    const visibleEvents = filterRenderableEvents(events || []);

    if (!visibleEvents.length) {
      wrapper.appendChild(emptyState("No events recorded for this run."));
      return wrapper;
    }

    visibleEvents.forEach((evt) => {
      const type = getEventType(evt);
      const meta = getEventMeta(type);

      const item = document.createElement("div");
      item.className = "event";

      const emoji = document.createElement("div");
      emoji.className = "event-emoji";
      emoji.textContent = meta.emoji || DEFAULT_EVENT_META.emoji;
      item.appendChild(emoji);

      const body = document.createElement("div");
      body.className = "event-body";

      const header = document.createElement("div");
      header.className = "event-header";

      const title = document.createElement("div");
      title.className = "event-title";
      title.textContent = meta.label || DEFAULT_EVENT_META.label;
      header.appendChild(title);

      const metaRow = document.createElement("div");
      metaRow.className = "event-meta";

      const ts = document.createElement("span");
      ts.className = "event-ts";
      ts.textContent = formatDate(evt.ingested_at);
      metaRow.appendChild(ts);

      const typeLabel = document.createElement("span");
      typeLabel.className = "event-type";
      typeLabel.textContent = type;
      metaRow.appendChild(typeLabel);

      header.appendChild(metaRow);
      body.appendChild(header);

      const contentNode = buildEventContent(evt, meta);
      if (contentNode) {
        contentNode.classList.add("event-content");
        body.appendChild(contentNode);
      }

      const pre = document.createElement("pre");
      pre.textContent = JSON.stringify(evt.event ?? evt, null, 2);

      const { toggle, details } = createDetailsSection(pre);

      body.appendChild(toggle);
      body.appendChild(details);

      item.appendChild(body);
      wrapper.appendChild(item);
    });

    return wrapper;
  }

  function createRunLayout() {
    const layout = document.createElement("div");
    layout.className = "run-layout";

    const header = document.createElement("div");
    header.className = "run-header card";

    const eventsSection = document.createElement("div");
    eventsSection.className = "run-events card";

    const eventsScroll = document.createElement("div");
    eventsScroll.className = "events-scroll";
    eventsSection.appendChild(eventsScroll);

    layout.appendChild(header);
    layout.appendChild(eventsSection);

    return { layout, header, eventsScroll, eventsSection };
  }

  function formatEventsInfoText(visibleCount, totalCount, streamingHiddenCount, truncatedCount) {
    const baseLabel = `${visibleCount} of ${totalCount} events shown`;
    const hiddenMessages = [];
    if (streamingHiddenCount > 0) {
      hiddenMessages.push(`${streamingHiddenCount} streaming deltas hidden`);
    }
    if (truncatedCount > 0) {
      hiddenMessages.push(`${truncatedCount} truncated by server`);
    }
    if (hiddenMessages.length > 0) {
      return `${baseLabel} (${hiddenMessages.join("; ")})`;
    }
    return `${visibleCount} event${visibleCount === 1 ? "" : "s"} loaded`;
  }

  function updateEventsUI(state) {
    if (!state || !state.infoEl || !state.eventsContainer) return;

    const visibleEvents = filterRenderableEvents(state.events);
    const totalCount = Math.max(
      Number.isFinite(Number(state.totalCount)) ? Number(state.totalCount) : 0,
      state.events.length
    );
    const streamingHiddenCount = Math.max(0, state.events.length - visibleEvents.length);
    const truncatedCount = Math.max(0, totalCount - state.events.length);

    state.infoEl.textContent = formatEventsInfoText(
      visibleEvents.length,
      totalCount,
      streamingHiddenCount,
      truncatedCount
    );

    const nextContainer = renderEventsList(visibleEvents);
    state.eventsContainer.replaceWith(nextContainer);
    state.eventsContainer = nextContainer;
    state.totalCount = totalCount;
    if (state.scrollFollower?.handleContentMutated) {
      state.scrollFollower.handleContentMutated();
    }
  }

  function resetLiveStream(reason) {
    if (liveStreamState?.socket) {
      try {
        liveStreamState.socket.close(1000, reason || "closing");
      } catch (_) {
        // noop
      }
    }
    if (liveReconnectController) {
      liveReconnectController.reset();
      liveReconnectController = null;
    }
    if (liveUnloadCleanup) {
      liveUnloadCleanup();
      liveUnloadCleanup = null;
    }
    if (liveStreamState?.scrollFollower?.cleanup) {
      liveStreamState.scrollFollower.cleanup();
    }
    if (runLayoutCleanup) {
      runLayoutCleanup();
      runLayoutCleanup = null;
    }
    liveStreamState = null;
  }

  function ensureUnloadCleanup() {
    if (liveUnloadCleanup) return;
    const handler = () => resetLiveStream("page unload");
    window.addEventListener("beforeunload", handler);
    liveUnloadCleanup = () => window.removeEventListener("beforeunload", handler);
  }

  function buildLiveURL(runId, cursor) {
    const proto = window.location.protocol === "https:" ? "wss" : "ws";
    const search = cursor ? `?last_ingested=${encodeURIComponent(cursor)}` : "";
    return `${proto}://${window.location.host}/runs/${encodeURIComponent(runId)}/events/live${search}`;
  }

  function ensureReconnectController(runId) {
    if (liveReconnectController) return liveReconnectController;
    if (!reconnectHelpers.createReconnectController) return null;
    liveReconnectController = reconnectHelpers.createReconnectController({
      maxAttempts: LIVE_RETRY_MAX_ATTEMPTS,
      baseDelayMs: LIVE_RETRY_BASE_DELAY_MS,
      maxDelayMs: LIVE_RETRY_MAX_DELAY_MS,
      jitterFactor: LIVE_RETRY_JITTER,
      setStatus,
      onRetry: () => {
        if (!liveStreamState || liveStreamState.runId !== runId) return;
        connectLiveStream(runId, liveStreamState.batchId, true);
      },
      onExhausted: (reason) => {
        setStatus(
          `Live updates unavailable${reason ? ` (${reason})` : ""}. Please refresh to continue.`,
          "warning",
          {
            action: {
              label: "Refresh",
              onClick: () => window.location.reload(),
            },
          }
        );
      },
    });
    return liveReconnectController;
  }

  function scheduleLiveReconnect(reason) {
    if (!liveStreamState) return;
    const reconnect = ensureReconnectController(liveStreamState.runId);
    if (!reconnect) return;
    if (reconnect.hasTimer()) {
      return { scheduled: true, attempts: reconnect.getAttempts(), delay: 0 };
    }
    return reconnect.scheduleReconnect(reason);
  }

  function handleLiveEnvelope(envelope) {
    if (!liveStreamState || !envelope) return;

    if (mergeHelpers.mergeLiveEnvelope) {
      const merged = mergeHelpers.mergeLiveEnvelope(liveStreamState, envelope);
      const { changed, cursorChanged, ...rest } = merged;
      liveStreamState = { ...liveStreamState, ...rest };
      if (changed) {
        updateEventsUI(liveStreamState);
      } else if (cursorChanged) {
        liveStreamState.cursor = merged.cursor;
      }
    } else {
      if (envelope.type === ENVELOPE_TYPE_EVENT && envelope.event) {
        const evt = envelope.event;
        const key = eventKeyFromEvent(evt);
        if (liveStreamState.seenKeys.has(key)) {
          return;
        }
        liveStreamState.events.push(evt);
        liveStreamState.seenKeys.add(key);
        if (envelope.cursor) {
          liveStreamState.cursor = envelope.cursor;
        } else if (getIngestedAt(evt)) {
          liveStreamState.cursor = getIngestedAt(evt);
        }
        liveStreamState.totalCount = Math.max(
          (liveStreamState.totalCount ?? liveStreamState.events.length) + 1,
          liveStreamState.events.length
        );
        updateEventsUI(liveStreamState);
      } else if (envelope.type === ENVELOPE_TYPE_HEARTBEAT) {
        if (envelope.cursor) {
          liveStreamState.cursor = envelope.cursor;
        }
      }
    }

    if (envelope.type === ENVELOPE_TYPE_ERROR) {
      setStatus(`Live stream error: ${envelope.error || "unknown error"}`, "warning");
      console.error("[live events] error", {
        runId: liveStreamState.runId,
        batchId: liveStreamState.batchId,
        error: envelope.error,
      });
    }
  }

  function connectLiveStream(runId, batchId, isRetry = false) {
    if (!runId || !liveStreamState) return;

    const url = buildLiveURL(runId, liveStreamState.cursor);
    if (liveStreamState.socket) {
      try {
        liveStreamState.socket.close(1000, "restarting");
      } catch (_) {
        // noop
      }
    }

    console.info("[live events] connecting", {
      runId,
      batchId,
      cursor: liveStreamState.cursor || "(none)",
      url,
    });

    const socket = new WebSocket(url);
    liveStreamState.socket = socket;
    ensureUnloadCleanup();
    const reconnect = ensureReconnectController(runId);

    socket.addEventListener("open", () => {
      console.info("[live events] connected", { runId, batchId });
      if (reconnect) reconnect.handleConnected();
    });

    socket.addEventListener("message", (event) => {
      try {
        const payload = typeof event.data === "string" ? JSON.parse(event.data) : null;
        if (!payload) return;
        handleLiveEnvelope(payload);
      } catch (err) {
        console.error("[live events] failed to parse message", err);
      }
    });

    socket.addEventListener("error", (err) => {
      setStatus("Live stream error (see console)", "warning");
      console.error("[live events] socket error", err);
      scheduleLiveReconnect("socket error");
    });

    socket.addEventListener("close", (evt) => {
      if (liveStreamState && liveStreamState.socket === socket) {
        liveStreamState.socket = null;
      }
      console.info("[live events] closed", {
        runId,
        batchId,
        code: evt.code,
        reason: evt.reason,
      });
      scheduleLiveReconnect(evt.reason || `code ${evt.code}`);
    });
  }

  async function renderRunView(runId, batchIdFromQuery) {
    resetLiveStream("switching view");
    viewTitle.textContent = `Run ${runId}`;
    setStatus("Loading run…", "loading");
    clearContent();

    let runData;
    try {
      runData = await fetchJSON(`/runs/${encodeURIComponent(runId)}`);
    } catch (err) {
      contentEl.appendChild(emptyState("Run not found."));
      setStatus(err.message || "Failed to load run", "error");
      return;
    }

    const run = runData.run || {};
    const batchId = run.batch_id || batchIdFromQuery || "";
    let batchLabel = batchId ? `Batch ${batchId}` : "";
    try {
      const batchDetail = batchId ? await fetchBatchDetail(batchId) : null;
      if (batchDetail) {
        const name = batchDetail.session_name;
        batchLabel = name ? `${name} (${batchDetail.batch_id})` : `Batch ${batchDetail.batch_id}`;
      }
    } catch (_) {
      // non-fatal; fallback to raw batch id
    }
    renderBreadcrumbs([
      { label: "Batches", href: "/app" },
      batchId
        ? { label: batchLabel, href: `/app?batch=${encodeURIComponent(batchId)}` }
        : null,
      { label: `Run ${runId}` },
    ].filter(Boolean));

    const actions = document.createElement("div");
    actions.className = "actions";
    actions.appendChild(createButtonLink("/app", "Back to batches"));
    if (batchId) {
      actions.appendChild(
        createButtonLink(
          `/app?batch=${encodeURIComponent(batchId)}`,
          "Back to runs"
        )
      );
    }

    const layout = createRunLayout();
    layout.header.appendChild(actions);
    layout.header.appendChild(renderRunMeta(run));
    const liveStatus = document.createElement("div");
    liveStatus.className = "pill live-status";
    layout.header.appendChild(liveStatus);

    let eventsData;
    try {
      eventsData = await fetchJSON(
        `/runs/${encodeURIComponent(runId)}/events?limit=${EVENTS_LIMIT}`
      );
    } catch (err) {
      contentEl.appendChild(emptyState("Unable to load events."));
      setStatus(err.message || "Failed to load events", "error");
      return;
    }

    const info = document.createElement("div");
    info.className = "pill";
    const rawEvents = eventsData.events || [];
    const visibleEvents = filterRenderableEvents(rawEvents);
    const totalCountRaw = eventsData.count ?? rawEvents.length;
    const totalCount = Number.isFinite(Number(totalCountRaw))
      ? Number(totalCountRaw)
      : rawEvents.length;
    const streamingHiddenCount = Math.max(0, rawEvents.length - visibleEvents.length);
    const truncatedCount = Math.max(0, totalCount - rawEvents.length);
    info.textContent = formatEventsInfoText(
      visibleEvents.length,
      totalCount,
      streamingHiddenCount,
      truncatedCount
    );
    layout.header.appendChild(info);

    const eventsContainer = renderEventsList(visibleEvents);
    layout.eventsScroll.appendChild(eventsContainer);
    contentEl.appendChild(layout.layout);

    const scrollFollower = createScrollFollower(layout.eventsScroll, {
      thresholdPx: SCROLL_FOLLOW_THRESHOLD_PX,
      onModeChange: (mode) => updateLiveFollowIndicator(liveStatus, mode),
    });

    liveStreamState = {
      runId,
      batchId,
      events: rawEvents.slice(),
      totalCount,
      infoEl: info,
      eventsContainer,
      cursor: deriveLatestIngested(rawEvents),
      seenKeys: new Set(rawEvents.map(eventKeyFromEvent)),
      socket: null,
      retryCount: 0,
      scrollFollower,
      liveStatusEl: liveStatus,
    };

    runLayoutCleanup = attachRunLayout(layout.layout);

    ensureReconnectController(runId);
    console.info("[live events] prepared initial state", {
      runId,
      batchId,
      cursor: liveStreamState.cursor || "(none)",
      eventCount: liveStreamState.events.length,
    });

    if (eventsData.events_truncated) {
      const limitUsedRaw =
        eventsData.event_limit_used ?? eventsData.count ?? EVENTS_LIMIT;
      const limitUsed = Number.isFinite(Number(limitUsedRaw))
        ? Number(limitUsedRaw)
        : limitUsedRaw;
      const note = document.createElement("div");
      note.className = "status warning";
      note.textContent = `Only showing first ${limitUsed} events (server truncated).`;
      layout.eventsSection.insertBefore(note, layout.eventsScroll);
    }

    updateEventsUI(liveStreamState);
    connectLiveStream(runId, batchId);
    setStatus("Run loaded", "success");
  }

  const layoutApi = {
    createRunLayout,
    applyRunLayoutSizing,
    computeRunHeaderOffset,
    attachRunLayout,
    isNearBottom,
    createScrollFollower,
  };

  if (typeof globalThis !== "undefined") {
    globalThis.RunLayout = Object.assign(globalThis.RunLayout || {}, layoutApi);
  }
  if (typeof module !== "undefined" && module.exports) {
    module.exports.RunLayout = layoutApi;
    module.exports.RunList = { buildRunsTable, renderRuns };
    module.exports.EventRender = {
      getEventType,
      getEventMeta,
      renderEventsList,
      formatCodexItemSummary,
    };
    module.exports.VersionView = {
      renderVersionView,
      fetchVersionData,
      getRouteFromHash,
      renderNav,
      routeApp,
    };
    module.exports.AgentsView = {
      renderAgentsView,
      fetchAgentsList,
      getRouteFromHash,
      renderNav,
      routeApp,
    };
    module.exports.RepositoriesView = {
      renderRepositoriesView,
      fetchRepositoriesList,
      getRouteFromHash,
      renderNav,
      routeApp,
    };
    module.exports.ControlPlaneView = {
      renderControlPlaneView,
      fetchRepositoryBatches,
      getRouteFromHash,
      renderNav,
      routeApp,
    };
    module.exports.Footer = {
      setFooterContent,
      DEFAULT_FOOTER_TEXT,
    };
  }

  function getRouteFromHash(rawHash) {
    const hash = rawHash === undefined ? window.location.hash || "" : rawHash || "";
    const normalized = hash.replace(/^#/, "").replace(/^\/+/, "").trim();
    if (!normalized) return "";
    const [path] = normalized.split(/[?#]/);
    if (path === "control-plane") return "control-plane";
    if (path === "repositories") return "repositories";
    if (path === "version") return "version";
    if (path === "agents") return "agents";
    return "";
  }

  function routeApp() {
    const route = getRouteFromHash();
    if (runParam) {
      renderNav("batches");
      return renderRunView(runParam, batchParam);
    }
    if (batchParam) {
      renderNav("batches");
      return renderRuns(batchParam);
    }
    if (route === "repositories") {
      renderNav("repositories");
      return renderRepositoriesView();
    }
    if (route === "control-plane") {
      renderNav("control-plane");
      return renderControlPlaneView();
    }
    if (route === "agents") {
      renderNav("agents");
      return renderAgentsView();
    }
    if (route === "version") {
      renderNav("version");
      return renderVersionView();
    }
    renderNav("batches");
    return renderBatches();
  }

  function handleHashChange() {
    if (runParam || batchParam) return;
    routeApp();
  }

  window.addEventListener("hashchange", handleHashChange);
  document.addEventListener("DOMContentLoaded", routeApp);
  setFooterContent(DEFAULT_FOOTER_TEXT);
})();
