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

    if (!events.length) {
      wrapper.appendChild(emptyState("No events recorded for this run."));
      return wrapper;
    }

    events.forEach((evt, idx) => {
      const item = document.createElement("div");
      item.className = "event";

      const header = document.createElement("div");
      header.className = "event-header";
      header.textContent = `Event ${idx + 1}`;

      const ts = document.createElement("span");
      ts.textContent = formatDate(evt.ingested_at);
      header.appendChild(ts);

      const pre = document.createElement("pre");
      pre.textContent = JSON.stringify(evt.event ?? evt, null, 2);

      item.appendChild(header);
      item.appendChild(pre);
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
    const count = eventsData.count ?? (eventsData.events ? eventsData.events.length : 0);
    info.textContent = `${count} event${count === 1 ? "" : "s"} loaded`;
    contentEl.appendChild(info);

    contentEl.appendChild(renderEventsList(eventsData.events || []));

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
