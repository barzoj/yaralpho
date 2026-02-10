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

  function getEventData(evt) {
    return (evt && (evt.event?.data || evt.data)) || {};
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
    const totalCountRaw = eventsData.count ?? rawEvents.length;
    const totalCount = Number.isFinite(Number(totalCountRaw))
      ? Number(totalCountRaw)
      : rawEvents.length;
    const streamingHiddenCount = Math.max(0, rawEvents.length - visibleEvents.length);
    const truncatedCount = Math.max(0, totalCount - rawEvents.length);
    const hiddenMessages = [];
    if (streamingHiddenCount > 0) {
      hiddenMessages.push(`${streamingHiddenCount} streaming deltas hidden`);
    }
    if (truncatedCount > 0) {
      hiddenMessages.push(`${truncatedCount} truncated by server`);
    }
    const baseLabel = `${visibleEvents.length} of ${totalCount} events shown`;
    info.textContent =
      hiddenMessages.length > 0
        ? `${baseLabel} (${hiddenMessages.join("; ")})`
        : `${visibleEvents.length} event${visibleEvents.length === 1 ? "" : "s"} loaded`;
    contentEl.appendChild(info);

    contentEl.appendChild(renderEventsList(visibleEvents));

    if (eventsData.events_truncated) {
      const limitUsedRaw =
        eventsData.event_limit_used ?? eventsData.count ?? EVENTS_LIMIT;
      const limitUsed = Number.isFinite(Number(limitUsedRaw))
        ? Number(limitUsedRaw)
        : limitUsedRaw;
      const note = document.createElement("div");
      note.className = "status warning";
      note.textContent = `Only showing first ${limitUsed} events (server truncated).`;
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
