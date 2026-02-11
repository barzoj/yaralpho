(function (global) {
  const ENVELOPE_TYPE_EVENT = "event";
  const ENVELOPE_TYPE_HEARTBEAT = "heartbeat";

  function getIngestedAt(evt) {
    return evt?.ingested_at || evt?.ingestedAt || "";
  }

  function eventKeyFromEvent(evt) {
    return [
      evt?.session_id || evt?.sessionId || "",
      evt?.run_id || evt?.runId || "",
      evt?.batch_id || evt?.batchId || "",
      getIngestedAt(evt) || "",
    ].join("|");
  }

  function deriveLatestIngested(events) {
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
  }

  function eventTimestamp(evt) {
    const ts = getIngestedAt(evt);
    const parsed = Date.parse(ts);
    return Number.isNaN(parsed) ? null : parsed;
  }

  function sortEvents(events) {
    return events.slice().sort((a, b) => {
      const ta = eventTimestamp(a);
      const tb = eventTimestamp(b);
      if (ta !== null && tb !== null && ta !== tb) return ta - tb;
      if (ta === null && tb !== null) return 1;
      if (ta !== null && tb === null) return -1;
      if (ta === null && tb === null) return 0;
      const ka = eventKeyFromEvent(a);
      const kb = eventKeyFromEvent(b);
      return ka.localeCompare(kb);
    });
  }

  function mergeLiveEnvelope(state, envelope) {
    if (!state || !envelope) return { changed: false, cursorChanged: false, ...state };

    if (envelope.type === ENVELOPE_TYPE_HEARTBEAT) {
      const cursor = envelope.cursor || state.cursor || "";
      return {
        ...state,
        cursor,
        changed: false,
        cursorChanged: cursor !== state.cursor && Boolean(cursor),
      };
    }

    if (envelope.type !== ENVELOPE_TYPE_EVENT || !envelope.event) {
      return { changed: false, cursorChanged: false, ...state };
    }

    const currentEvents = Array.isArray(state.events) ? state.events : [];
    const seenKeys = state.seenKeys instanceof Set ? state.seenKeys : new Set();
    const key = eventKeyFromEvent(envelope.event);

    if (seenKeys.has(key)) {
      const cursor = envelope.cursor || state.cursor;
      return {
        ...state,
        cursor: cursor || state.cursor,
        changed: false,
        cursorChanged: Boolean(cursor && cursor !== state.cursor),
      };
    }

    const nextEvents = sortEvents([...currentEvents, envelope.event]);
    const nextSeen = new Set(seenKeys);
    nextSeen.add(key);

    const cursor = envelope.cursor || deriveLatestIngested(nextEvents) || state.cursor || "";
    const baseTotal = Number.isFinite(Number(state.totalCount))
      ? Number(state.totalCount)
      : currentEvents.length;
    const nextTotal = Math.max(baseTotal + 1, nextEvents.length);

    return {
      ...state,
      events: nextEvents,
      seenKeys: nextSeen,
      cursor,
      totalCount: nextTotal,
      changed: true,
      cursorChanged: cursor !== state.cursor && Boolean(cursor),
    };
  }

  const api = {
    mergeLiveEnvelope,
    getIngestedAt,
    eventKeyFromEvent,
    deriveLatestIngested,
  };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }

  if (global) {
    global.LiveEventsMerge = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : this);
