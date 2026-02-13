(function () {
  const statusEl = document.getElementById("status");
  const contentEl = document.getElementById("content");
  const viewTitle = document.getElementById("view-title");
  const breadcrumbsEl = document.getElementById("breadcrumbs");
  const runLayoutHelpers = typeof RunLayout !== "undefined" ? RunLayout : {};

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
