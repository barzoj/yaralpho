(function () {
  const statusEl = document.getElementById("status");
  const contentEl = document.getElementById("content");
  const viewTitle = document.getElementById("view-title");
  const breadcrumbsEl = document.getElementById("breadcrumbs");

  const params = new URLSearchParams(window.location.search);
  const batchParam = params.get("batch");
  const runParam = params.get("run");

  const LIST_LIMIT = 50;
  const EVENTS_LIMIT = 10000;

  const EVENT_META = {
    abort: { emoji: "⛔", label: "Session aborted" },
    "assistant.intent": {
      emoji: "🎯",
      label: "Assistant intent",
      summary: (data) => data?.intent,
    },
    "assistant.message": {
      emoji: "🤖",
      label: "Assistant message",
      summary: (data) =>
        data?.content || (data?.encryptedContent ? "Encrypted message" : ""),
    },
    "assistant.message_delta": { hidden: true },
    "assistant.reasoning": {
      emoji: "🧠",
      label: "Assistant reasoning",
      summary: (data) => data?.content || data?.reasoningText,
    },
    "assistant.reasoning_delta": { hidden: true },
    "assistant.turn_end": {
      emoji: "🔚",
      label: "Turn end",
      summary: (data) => (data?.turnId ? `Turn ${data.turnId}` : ""),
    },
    "assistant.turn_start": {
      emoji: "🔜",
      label: "Turn start",
      summary: (data) => (data?.turnId ? `Turn ${data.turnId}` : ""),
    },
    "assistant.usage": {
      emoji: "📊",
      label: "Usage",
      summary: (data) => formatUsageSummary(data),
    },
    "hook.end": { emoji: "🪝", label: "Hook end" },
    "hook.start": { emoji: "🪝", label: "Hook start" },
    "pending_messages.modified": {
      emoji: "⏳",
      label: "Pending messages updated",
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
    "session.idle": { emoji: "💤", label: "Session idle" },
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
      summary: (data) => formatUsageInfoSummary(data),
    },
    "skill.invoked": {
      emoji: "📦",
      label: "Skill invoked",
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
    "system.message": {
      emoji: "🛡️",
      label: "System message",
      summary: (data) => data?.content || data?.transformedContent,
    },
    "tool.execution_complete": {
      emoji: "🛠️",
      label: "Tool complete",
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
      summary: (data) => formatToolSummary(data),
    },
    "tool.user_requested": { emoji: "🙋", label: "Tool requested by user" },
    "user.message": {
      emoji: "🧑",
      label: "User message",
      summary: (data) => data?.content || data?.transformedContent,
    },
  };

  const DEFAULT_EVENT_META = { emoji: "❔", label: "Unknown event" };

  function setStatus(text, type = "info") {
    statusEl.textContent = text;
    statusEl.className = `status ${type}`;
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

  async function fetchJSON(url) {
    const res = await fetch(url);
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

  function getEventType(evt) {
    return (evt && (evt.event?.type || evt.type)) || "unknown";
  }

  function getEventMeta(type) {
    return EVENT_META[type] || { ...DEFAULT_EVENT_META };
  }

  function shouldRenderEvent(evt) {
    const meta = getEventMeta(getEventType(evt));
    return !meta.hidden;
  }

  function filterRenderableEvents(events) {
    return (events || []).filter(shouldRenderEvent);
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

  async function renderRuns(batchId) {
    viewTitle.textContent = `Runs for ${batchId}`;
    renderBreadcrumbs([
      { label: "Batches", href: "/app" },
      { label: batchId },
    ]);
    setStatus("Loading runs…", "loading");
    clearContent();

    const actions = document.createElement("div");
    actions.className = "actions";
    actions.appendChild(createButtonLink("/app", "Back to batches"));
    contentEl.appendChild(actions);

    try {
      const data = await fetchJSON(
        `/runs?batch_id=${encodeURIComponent(batchId)}&limit=${LIST_LIMIT}`
      );
      const runs = data.runs || [];
      if (!runs.length) {
        contentEl.appendChild(emptyState("No runs for this batch."));
        setStatus("No runs found", "warning");
        return;
      }

      const rows = runs.map((run) => [
        createLink(
          `/app?batch=${encodeURIComponent(batchId)}&run=${encodeURIComponent(
            run.run_id
          )}`,
          run.run_id
        ),
        run.status || "unknown",
        formatDate(run.started_at),
        formatDate(run.finished_at),
      ]);

      contentEl.appendChild(
        buildTable(["Run", "Status", "Started", "Finished"], rows)
      );
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
      ["Epic Ref", run.epic_ref || "—"],
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

      const summary = formatEventSummary(evt, meta);
      if (summary) {
        const summaryEl = document.createElement("div");
        summaryEl.className = "event-summary";
        summaryEl.textContent = summary;
        body.appendChild(summaryEl);
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

  async function renderRunView(runId, batchIdFromQuery) {
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
    renderBreadcrumbs([
      { label: "Batches", href: "/app" },
      batchId ? { label: `Batch ${batchId}`, href: `/app?batch=${encodeURIComponent(batchId)}` } : null,
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
    contentEl.appendChild(actions);

    contentEl.appendChild(renderRunMeta(run));

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
    const totalCount = eventsData.count ?? rawEvents.length;
    const hiddenCount = Math.max(0, totalCount - visibleEvents.length);
    info.textContent =
      hiddenCount > 0
        ? `${visibleEvents.length} of ${totalCount} events shown (streaming deltas hidden)`
        : `${visibleEvents.length} event${visibleEvents.length === 1 ? "" : "s"} loaded`;
    contentEl.appendChild(info);

    contentEl.appendChild(renderEventsList(visibleEvents));

    if (eventsData.events_truncated) {
      const note = document.createElement("div");
      note.className = "status warning";
      note.textContent = `Only showing first ${eventsData.event_limit_used} events.`;
      contentEl.appendChild(note);
    }

    setStatus("Run loaded", "success");
  }

  function start() {
    if (runParam) {
      renderRunView(runParam, batchParam);
      return;
    }
    if (batchParam) {
      renderRuns(batchParam);
      return;
    }
    renderBatches();
  }

  document.addEventListener("DOMContentLoaded", start);
})();
