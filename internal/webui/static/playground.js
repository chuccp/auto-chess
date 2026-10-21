(function () {
  "use strict";

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
  const BASE = location.origin;
  const isEN = () => window.__LANG === "en" || document.documentElement.lang === "en";
  const T = (zh, en) => (isEN() ? en : zh);
  const HISTORY_KEY = "auto-chess.playground.history.v1";
  const HISTORY_LIMIT = 10;
  const DIFFICULTY_TIME = { easy: 300, normal: 1000, hard: 2500, master: 6000 };
  const DIFFICULTY_DEPTH = { easy: 2, normal: 4, hard: 5, master: 7 };

  const state = {
    games: [],
    currentId: null,
    history: [],
    sending: false,
    streaming: false,
  };

  const el = {
    gameSelect: $("#game-select"),
    difficulty: $("#difficulty"),
    timeLimit: $("#time-limit"),
    maxDepth: $("#max-depth"),
    requestJSON: $("#request-json"),
    gameHint: $("#game-hint"),
    sendBtn: $("#send-btn"),
    streamBtn: $("#stream-btn"),
    resetBtn: $("#reset-btn"),
    curlBtn: $("#curl-btn"),
    responseStatus: $("#response-status"),
    responseTime: $("#response-time"),
    responseMove: $("#response-move"),
    responseJSON: $("#response-json"),
    progressLog: $("#progress-log"),
    progressSummary: $("#progress-summary"),
    history: $("#history"),
    clearHistory: $("#clear-history"),
    toast: $("#toast"),
  };

  function escapeHTML(value) {
    return String(value)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#39;");
  }

  function showToast(message) {
    if (!el.toast) return;
    el.toast.textContent = message;
    el.toast.classList.add("show");
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => el.toast.classList.remove("show"), 1900);
  }

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      showToast(T("已复制到剪贴板", "Copied to clipboard"));
    } catch (_) {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      try {
        document.execCommand("copy");
        showToast(T("已复制到剪贴板", "Copied to clipboard"));
      } catch (e) {
        showToast(T("复制失败，请手动选择", "Copy failed - select and copy manually"));
      }
      area.remove();
    }
  }

  // Keep primitive arrays (board arrays) on one line.
  function pretty(value) {
    const text = JSON.stringify(value, null, 2);
    return text.replace(
      /\[\n\s+((?:"(?:[^"\\]|\\.)*"|-?\d+|true|false|null)(?:,\n\s+(?:"(?:[^"\\]|\\.)*"|-?\d+|true|false|null))*)\n\s+\]/g,
      (_match, inner) => "[" + inner.replace(/\n\s+/g, " ") + "]"
    );
  }

  function currentGame() {
    return state.games.find((g) => g.id === state.currentId) || state.games[0] || null;
  }

  function defaultPosition(meta) {
    if (meta.board.type === "fen") {
      return { fen: meta.initialPosition.fen };
    }
    return {
      board: meta.initialPosition.board,
      turn: meta.initialPosition.turn,
    };
  }

  function buildRequest(meta) {
    const request = {
      game: meta.id,
      position: defaultPosition(meta),
      difficulty: el.difficulty.value || "normal",
      timeLimitMs: Number(el.timeLimit.value) || DIFFICULTY_TIME[el.difficulty.value] || 1000,
    };
    const depth = Number(el.maxDepth.value);
    if (depth > 0) request.maxDepth = depth;
    return request;
  }

  function gameHint(meta) {
    if (isEN()) {
      if (meta.board.type === "fen") {
        return `${meta.englishName} · FEN position; see the response example for move fields.`;
      }
      return `${meta.englishName} · flat board[${meta.board.cells}] array plus turn; see the response example for move fields.`;
    }
    if (meta.board.type === "fen") {
      return `${meta.name} · 使用 FEN 描述局面，走法为 ${String(meta.moveFormat).split("；")[0]}。`;
    }
    return `${meta.name} · 使用 board[${meta.board.cells}] 扁平数组与 turn；走法见右侧响应示例。`;
  }

  function setGame(id, options = {}) {
    const meta = state.games.find((g) => g.id === id);
    if (!meta) return;
    state.currentId = meta.id;
    if (el.gameSelect) el.gameSelect.value = meta.id;
    if (el.gameHint) el.gameHint.textContent = gameHint(meta);
    if (options.resetControls !== false) {
      const diff = el.difficulty.value || "normal";
      el.timeLimit.value = String(DIFFICULTY_TIME[diff] || 1000);
      el.maxDepth.value = String(DIFFICULTY_DEPTH[diff] || 4);
    }
    if (el.requestJSON) el.requestJSON.value = pretty(buildRequest(meta));
  }

  function syncControlsToRequest() {
    if (!el.requestJSON) return;
    try {
      const obj = JSON.parse(el.requestJSON.value);
      if (!obj || typeof obj !== "object" || Array.isArray(obj)) return;
      obj.difficulty = el.difficulty.value;
      obj.timeLimitMs = Number(el.timeLimit.value) || DIFFICULTY_TIME[el.difficulty.value] || 1000;
      const depth = Number(el.maxDepth.value);
      if (depth > 0) obj.maxDepth = depth;
      el.requestJSON.value = pretty(obj);
    } catch (_) {
      // Keep the user's in-progress text intact.
    }
  }

  function moveLabel(move) {
    if (!move) return "—";
    if (move.uci) return move.uci;
    if (move.index && typeof move.index.index === "number") return "index " + move.index.index;
    if (move.from && move.to) return `${move.from.index} → ${move.to.index}`;
    return "—";
  }

  function moveSub(move, data) {
    if (!move) return "";
    const parts = [];
    if (move.from && move.to) {
      parts.push(`row ${move.from.row}, col ${move.from.col} → row ${move.to.row}, col ${move.to.col}`);
    } else if (move.index) {
      parts.push(`row ${move.index.row}, col ${move.index.col}`);
    }
    if (move.promotion) parts.push(T("升变 ", "promotion ") + move.promotion);
    if (move.captured) parts.push(T("吃 ", "captures ") + move.captured);
    if (move.flipped) parts.push(T("翻 " + move.flipped + " 子", "flips " + move.flipped));
    if (move.jump) parts.push(T("跳跃", "jump"));
    if (data && data.engine) parts.push("engine " + data.engine);
    if (data && data.gameOver) parts.push(T("终局", "game over"));
    return parts.join(" · ");
  }

  function setResponseStatus(text, kind) {
    if (!el.responseStatus) return;
    el.responseStatus.textContent = text;
    el.responseStatus.className = "resp-status " + (kind || "idle");
  }

  function renderResponse(payload, httpStatus, elapsed, request) {
    const ok = payload && payload.ok === true;
    const statusText = typeof httpStatus === "string" ? httpStatus : `HTTP ${httpStatus}`;
    setResponseStatus(statusText, ok ? "ok" : "err");
    if (el.responseTime) el.responseTime.textContent = `${elapsed} ms · ${new Date().toLocaleTimeString()}`;
    if (el.responseJSON) el.responseJSON.textContent = pretty(payload);
    if (el.responseMove) {
      if (ok && payload.data && payload.data.move) {
        const move = payload.data.move;
        el.responseMove.innerHTML = `<span class="move-label">${T("AI 走法", "AI move")}</span><strong>${escapeHTML(moveLabel(move))}</strong><small>${escapeHTML(moveSub(move, payload.data))}</small>`;
      } else if (ok && payload.data && payload.data.gameOver) {
        el.responseMove.innerHTML = `<span class="move-label">${T("对局状态", "Game status")}</span><strong>${T("已结束", "Game over")}</strong><small>${T("没有可走的下一步", "No next move")}</small>`;
      } else {
        const err = (payload && payload.error) || {};
        el.responseMove.innerHTML = `<span class="move-label">${T("请求失败", "Request failed")}</span><strong>${escapeHTML(err.code || "ERROR")}</strong><small>${escapeHTML(err.message || T("请检查请求 JSON", "Check the request JSON"))}</small>`;
      }
    }
    pushHistory({
      game: (request && request.game) || state.currentId || "?",
      request,
      ok,
      httpStatus,
      elapsed,
      move: ok && payload.data ? moveLabel(payload.data.move) : (payload && payload.error ? payload.error.code : "ERR"),
    });
  }

  function resetProgress(message) {
    if (el.progressSummary) el.progressSummary.textContent = T("流式计算时显示", "Shown during streaming");
    if (!el.progressLog) return;
    el.progressLog.innerHTML = `<span class="muted">${escapeHTML(message)}</span>`;
  }

  function appendProgress(p, kind) {
    if (!el.progressLog) return;
    if (el.progressLog.querySelector(".muted")) el.progressLog.innerHTML = "";
    const row = document.createElement("div");
    row.className = "progress-row" + (kind === "result" ? " stream-result" : "");
    const depth = Number.isFinite(p.depth) ? p.depth : "—";
    const score = Number.isFinite(p.score) ? p.score : "—";
    const nodes = Number.isFinite(p.nodes) ? p.nodes : 0;
    const timeMs = Number.isFinite(p.timeMs) ? p.timeMs : 0;
    const best = p.best ? moveLabel(p.best) : "—";
    row.innerHTML = `<span class="p-depth">D${escapeHTML(depth)}</span>
      <span class="p-score">${escapeHTML(score)}</span>
      <span class="p-nodes">${escapeHTML(nodes)} nodes</span>
      <span class="p-time">${escapeHTML(timeMs)} ms</span>
      <span class="p-best">${escapeHTML(best)}</span>`;
    el.progressLog.appendChild(row);
    el.progressLog.scrollTop = el.progressLog.scrollHeight;
    if (el.progressSummary) {
      el.progressSummary.textContent = T(`深度 ${depth} · ${nodes} 节点 · ${timeMs} ms`, `Depth ${depth} · ${nodes} nodes · ${timeMs} ms`);
    }
  }

  function resultProgress(result) {
    if (!result || !result.search) return;
    appendProgress({
      depth: result.search.depth,
      score: result.evaluation ? result.evaluation.score : 0,
      nodes: result.search.nodes,
      timeMs: result.search.timeMs,
      best: result.move,
    }, "result");
    if (el.progressSummary) {
      el.progressSummary.textContent = T(
        `完成 · 深度 ${result.search.depth} · ${result.search.nodes} 节点 · ${result.search.timeMs} ms`,
        `Done · depth ${result.search.depth} · ${result.search.nodes} nodes · ${result.search.timeMs} ms`
      );
    }
  }

  function parseSSEFrame(frame) {
    let event = "";
    const data = [];
    for (const line of frame.split("\n")) {
      if (line.startsWith("event:")) event = line.slice(6).trim();
      else if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
    }
    return { event, data: data.join("\n") };
  }

  async function sendRequest() {
    if (state.sending || state.streaming) return;
    let request;
    try {
      request = JSON.parse(el.requestJSON.value);
    } catch (err) {
      setResponseStatus(T("JSON 错误", "JSON error"), "err");
      if (el.responseJSON) el.responseJSON.textContent = T("请求 JSON 解析失败：\n", "Failed to parse request JSON:\n") + err.message;
      return;
    }
    state.sending = true;
    el.sendBtn.disabled = true;
    el.sendBtn.textContent = T("计算中…", "Computing...");
    resetProgress(T("普通请求一次性返回；点击“流式计算”查看逐层思考过程。", "Regular requests return at once; use Stream to watch the thinking process."));
    setResponseStatus(T("请求中…", "Requesting..."), "busy");
    const started = performance.now();
    let httpStatus = 0;
    let payload;
    try {
      const res = await fetch(BASE + "/api/v1/move", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      });
      httpStatus = res.status;
      const text = await res.text();
      try {
        payload = JSON.parse(text);
      } catch (_) {
        payload = { ok: false, error: { code: "BAD_RESPONSE", message: text.slice(0, 300) } };
      }
    } catch (err) {
      payload = { ok: false, error: { code: "NETWORK_ERROR", message: err.message } };
    }
    const elapsed = Math.round(performance.now() - started);
    renderResponse(payload, httpStatus, elapsed, request);
    state.sending = false;
    el.sendBtn.disabled = false;
    el.sendBtn.innerHTML = `${T("发送请求", "Send request")} <span>→</span>`;
  }

  async function sendStream() {
    if (state.streaming || state.sending) return;
    let request;
    try {
      request = JSON.parse(el.requestJSON.value);
    } catch (err) {
      setResponseStatus(T("JSON 错误", "JSON error"), "err");
      if (el.responseJSON) el.responseJSON.textContent = T("请求 JSON 解析失败：\n", "Failed to parse request JSON:\n") + err.message;
      return;
    }
    state.streaming = true;
    el.streamBtn.disabled = true;
    el.streamBtn.textContent = T("流式计算中…", "Streaming...");
    el.sendBtn.disabled = true;
    resetProgress(T("正在连接 SSE，等待第一层思考结果…", "Connecting to SSE, waiting for the first thinking step..."));
    setResponseStatus(T("SSE · 连接中", "SSE · connecting"), "busy");
    const started = performance.now();
    let finished = false;
    try {
      const res = await fetch(BASE + "/api/v1/move/stream", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      });
      if (!res.ok || !res.body) {
        const text = await res.text();
        let payload;
        try {
          payload = JSON.parse(text);
        } catch (_) {
          payload = { ok: false, error: { code: "BAD_RESPONSE", message: text.slice(0, 300) } };
        }
        renderResponse(payload, res.status, Math.round(performance.now() - started), request);
        return;
      }
      setResponseStatus(T("SSE · 思考中", "SSE · thinking"), "busy");
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
        let cut;
        while ((cut = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0, cut);
          buffer = buffer.slice(cut + 2);
          const { event, data } = parseSSEFrame(frame);
          if (!event || !data) continue;
          let payload;
          try {
            payload = JSON.parse(data);
          } catch (_) {
            continue;
          }
          const elapsed = Math.round(performance.now() - started);
          if (event === "start") {
            if (el.responseTime) el.responseTime.textContent = T(`SSE 已连接 · ${new Date().toLocaleTimeString()}`, `SSE connected · ${new Date().toLocaleTimeString()}`);
            if (el.progressSummary) {
              el.progressSummary.textContent = T(
                `${payload.game || state.currentId} · ${payload.engine || "engine"} · ${payload.difficulty || ""} · 思考中`,
                `${payload.game || state.currentId} · ${payload.engine || "engine"} · ${payload.difficulty || ""} · thinking`
              );
            }
          } else if (event === "depth") {
            appendProgress(payload);
          } else if (event === "result") {
            resultProgress(payload);
            renderResponse({ ok: true, data: payload }, "SSE · OK", elapsed, request);
            finished = true;
          } else if (event === "error") {
            const error = payload.error || { code: "ERROR", message: T("SSE 错误", "SSE error") };
            if (el.progressSummary) el.progressSummary.textContent = T(`错误 · ${error.code}`, `Error · ${error.code}`);
            renderResponse({ ok: false, error }, "SSE · ERROR", elapsed, request);
            finished = true;
          } else if (event === "done") {
            if (!finished && payload.ok !== false) {
              setResponseStatus(T("SSE · 完成", "SSE · complete"), "ok");
            }
          }
        }
      }
      if (!finished) setResponseStatus(T("SSE · 已结束", "SSE · ended"), "err");
    } catch (err) {
      setResponseStatus(T("SSE · 网络错误", "SSE · network error"), "err");
      if (el.responseJSON) el.responseJSON.textContent = T("SSE 请求失败：\n", "SSE request failed:\n") + err.message;
    } finally {
      state.streaming = false;
      el.streamBtn.disabled = false;
      el.streamBtn.textContent = T("流式计算", "Stream");
      el.sendBtn.disabled = false;
    }
  }

  function readHistory() {
    try {
      const raw = localStorage.getItem(HISTORY_KEY);
      const list = raw ? JSON.parse(raw) : [];
      return Array.isArray(list) ? list.slice(0, HISTORY_LIMIT) : [];
    } catch (_) {
      return [];
    }
  }

  function writeHistory() {
    try {
      localStorage.setItem(HISTORY_KEY, JSON.stringify(state.history.slice(0, HISTORY_LIMIT)));
    } catch (_) {
      // localStorage may be disabled.
    }
  }

  function pushHistory(entry) {
    entry.time = Date.now();
    state.history.unshift(entry);
    state.history = state.history.slice(0, HISTORY_LIMIT);
    writeHistory();
    renderHistory();
  }

  function renderHistory() {
    if (!el.history) return;
    if (!state.history.length) {
      el.history.innerHTML = `<span class="muted">${T("暂无记录", "No history yet")}</span>`;
      return;
    }
    el.history.innerHTML = state.history
      .map((entry, index) => `
        <div class="history-item" data-history="${index}" title="${T("点击恢复这次请求", "Click to restore this request")}">
          <span class="h-game">${escapeHTML(entry.game)}</span>
          <span>${escapeHTML(entry.move || "—")}</span>
          <span class="h-time">${entry.ok ? "OK" : "ERR"} · ${entry.elapsed}ms</span>
          <span class="h-move">${escapeHTML(new Date(entry.time).toLocaleTimeString())}</span>
        </div>`)
      .join("");
    $$("[data-history]", el.history).forEach((node) => {
      node.addEventListener("click", () => {
        const entry = state.history[Number(node.dataset.history)];
        if (!entry || !entry.request) return;
        if (entry.game && state.games.some((g) => g.id === entry.game)) {
          setGame(entry.game, { resetControls: false });
        }
        el.requestJSON.value = pretty(entry.request);
        if (entry.request.difficulty) el.difficulty.value = entry.request.difficulty;
        if (entry.request.timeLimitMs) el.timeLimit.value = String(entry.request.timeLimitMs);
        if (entry.request.maxDepth) el.maxDepth.value = String(entry.request.maxDepth);
        showToast(T("已恢复这次请求", "Request restored"));
      });
    });
  }

  function bindEvents() {
    el.gameSelect?.addEventListener("change", () => setGame(el.gameSelect.value));
    el.difficulty?.addEventListener("change", () => {
      const diff = el.difficulty.value;
      el.timeLimit.value = String(DIFFICULTY_TIME[diff] || 1000);
      el.maxDepth.value = String(DIFFICULTY_DEPTH[diff] || 4);
      const meta = currentGame();
      if (meta) el.gameHint.textContent = gameHint(meta);
      syncControlsToRequest();
    });
    el.timeLimit?.addEventListener("change", syncControlsToRequest);
    el.maxDepth?.addEventListener("change", syncControlsToRequest);
    el.sendBtn?.addEventListener("click", sendRequest);
    el.streamBtn?.addEventListener("click", sendStream);
    el.resetBtn?.addEventListener("click", () => {
      if (currentGame()) setGame(currentGame().id);
    });
    el.curlBtn?.addEventListener("click", () => {
      let body;
      try {
        body = JSON.stringify(JSON.parse(el.requestJSON.value));
      } catch (_) {
        showToast(T("请求 JSON 尚不合法", "Request JSON is not valid yet"));
        return;
      }
      copyText(`curl -X POST ${BASE}/api/v1/move \\\n  -H "Content-Type: application/json" \\\n  -d '${body}'`);
    });
    el.clearHistory?.addEventListener("click", () => {
      state.history = [];
      writeHistory();
      renderHistory();
      showToast(T("已清空本机调试记录", "Local debug history cleared"));
    });
    document.addEventListener("keydown", (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
        event.preventDefault();
        sendRequest();
      }
    });
  }

  function loadGames() {
    const node = document.getElementById("games-data");
    if (node) {
      try {
        const parsed = JSON.parse(node.textContent);
        if (Array.isArray(parsed)) state.games = parsed;
      } catch (_) {
        state.games = [];
      }
    }
    if (!state.games.length) {
      showToast(T("读取棋种目录失败", "Failed to load the game catalog"));
      return;
    }
    const wanted = new URLSearchParams(location.search).get("game");
    const preferred = state.games.find((g) => g.id === wanted)
      || state.games.find((g) => g.id === "xiangqi")
      || state.games[0];
    setGame(preferred.id);
  }

  function init() {
    state.history = readHistory();
    renderHistory();
    bindEvents();
    loadGames();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
