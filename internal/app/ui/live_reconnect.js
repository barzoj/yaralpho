/* Reconnect controller for live event streams with jittered backoff. */
(function (global) {
  function computeBackoffDelay(attempt, options = {}) {
    const baseDelayMs = Number.isFinite(options.baseDelayMs) ? options.baseDelayMs : 1000;
    const maxDelayMs = Number.isFinite(options.maxDelayMs) ? options.maxDelayMs : 15000;
    const jitterFactor =
      typeof options.jitterFactor === "number" && options.jitterFactor >= 0
        ? options.jitterFactor
        : 0.2;
    const rand = typeof options.rand === "function" ? options.rand : Math.random;

    const expDelay = baseDelayMs * Math.pow(2, Math.max(0, attempt - 1));
    const capped = Math.min(expDelay, maxDelayMs);
    const jitter = capped * jitterFactor * rand();
    return Math.round(capped + jitter);
  }

  function createReconnectController(opts = {}) {
    const maxAttempts = Number.isFinite(opts.maxAttempts) ? opts.maxAttempts : 5;
    const setStatus = typeof opts.setStatus === "function" ? opts.setStatus : () => {};
    const onRetry = typeof opts.onRetry === "function" ? opts.onRetry : () => {};
    const onExhausted = typeof opts.onExhausted === "function" ? opts.onExhausted : () => {};
    const schedule = typeof opts.schedule === "function" ? opts.schedule : setTimeout;
    const cancel = typeof opts.cancel === "function" ? opts.cancel : clearTimeout;
    const rand = typeof opts.rand === "function" ? opts.rand : Math.random;

    let attempts = 0;
    let timerId = null;

    function reset() {
      if (timerId !== null) {
        cancel(timerId);
        timerId = null;
      }
      attempts = 0;
    }

    function handleConnected() {
      reset();
      setStatus("Live updates connected", "success");
    }

    function scheduleReconnect(reason) {
      if (timerId !== null) {
        cancel(timerId);
        timerId = null;
      }

      if (attempts >= maxAttempts) {
        onExhausted(reason, attempts);
        return { scheduled: false, attempts, delay: 0 };
      }

      attempts += 1;
      const delay = computeBackoffDelay(attempts, {
        baseDelayMs: opts.baseDelayMs,
        maxDelayMs: opts.maxDelayMs,
        jitterFactor: opts.jitterFactor,
        rand,
      });

      const attemptLabel = `${attempts}/${maxAttempts}`;
      const seconds = (delay / 1000).toFixed(1);
      setStatus(
        `Live updates disconnected${reason ? ` (${reason})` : ""}; retrying in ${seconds}s (attempt ${attemptLabel})`,
        "warning"
      );

      timerId = schedule(() => {
        timerId = null;
        onRetry(attempts);
      }, delay);

      return { scheduled: true, attempts, delay };
    }

    return {
      scheduleReconnect,
      handleConnected,
      reset,
      getAttempts() {
        return attempts;
      },
      hasTimer() {
        return timerId !== null;
      },
    };
  }

  const api = { computeBackoffDelay, createReconnectController };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }

  if (global) {
    global.LiveReconnect = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : this);
