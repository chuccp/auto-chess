(function () {
  "use strict";

  const $ = (sel, root = document) => root.querySelector(sel);
  const BASE = window.__REL_BASE__ || "./";
  const isEN = () => window.__LANG === "en" || document.documentElement.lang === "en";
  const T = (zh, en) => (isEN() ? en : zh);
  const EN_COLORS = { w: "White", b: "Black", r: "Red", y: "Yellow" };
  const RECORD_KEY = "auto-chess.play.games.v1";
  const API_MODE_KEY = "auto-chess.play.api.v1";
  const RECORD_LIMIT = 12;
  const TIME_BY_DIFF = { easy: 300, normal: 1000, hard: 2500, master: 6000 };
  const DEFAULT_ENGINE = { chess: "chess-3", xiangqi: "godogpaw" };
  const ROW_COUNTS = [1, 2, 3, 4, 13, 12, 11, 10, 9, 10, 11, 12, 13, 4, 3, 2, 1];

  const CHESS_GLYPH = {
    P: "♙", N: "♘", B: "♗", R: "♖", Q: "♕", K: "♔",
    p: "♟", n: "♞", b: "♝", r: "♜", q: "♛", k: "♚",
  };
  const XIANGQI_GLYPH = {
    r: { K: "帅", A: "仕", B: "相", N: "马", R: "车", C: "炮", P: "兵" },
    b: { K: "将", A: "士", B: "象", N: "馬", R: "車", C: "砲", P: "卒" },
  };
  const JUNGLE_GLYPH = { E: "象", L: "狮", T: "虎", P: "豹", W: "狼", D: "狗", C: "猫", R: "鼠" };
  const EN_MOVE_FORMAT = {
    chess: "UCI, e.g. e2e4; append q/r/b/n for promotion",
    xiangqi: "UCCI, e.g. h2e2, b0c2",
    gomoku: "cell index 0–224",
    reversi: "cell index 0–63",
    connect4: "landing cell index 0–41",
    jungle: "from / to flat indices",
    checkers: "from / to hole numbers",
  };

  const state = {
    games: [],
    meta: null,
    position: null,
    humanSide: "",
    difficulty: "normal",
    apiMode: "sse",
    selected: null,
    legal: [],
    history: [],
    undoStack: [],
    engine: "",
    recordId: "",
    busy: false,
    animating: false,
    lastMove: null,
    aiDepth: 0,
  };

  const el = {
    game: $("#play-game"),
    side: $("#play-side"),
    difficulty: $("#play-difficulty"),
    engine: $("#play-engine"),
    apiMode: $("#play-api-mode"),
    newGame: $("#new-game-btn"),
    undo: $("#undo-btn"),
    board: $("#board"),
    caption: $("#board-caption"),
    title: $("#play-game-name"),
    status: $("#play-status"),
    hint: $("#play-hint"),
    infoTurn: $("#info-turn"),
    infoEngine: $("#info-engine"),
    infoStatus: $("#info-status"),
    infoRecord: $("#info-record"),
    moveList: $("#move-list"),
    records: $("#record-list"),
    clearRecords: $("#clear-records"),
    aiCard: $("#ai-progress-card"),
    aiLabel: $("#ai-progress-label"),
    aiMeta: $("#ai-progress-meta"),
    aiBar: $("#ai-progress-bar"),
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
    showToast.timer = setTimeout(() => el.toast.classList.remove("show"), 2200);
  }

  function clone(value) {
    return JSON.parse(JSON.stringify(value));
  }

  function currentMeta() {
    return state.games.find((g) => g.id === (state.meta && state.meta.id));
  }

  function otherSide(side) {
    if (!state.meta) return side;
    const ids = state.meta.colors.map((c) => c.id);
    return ids.find((id) => id !== side) || side;
  }

  function sideName(side) {
    if (isEN()) {
      if (state.meta && state.meta.id === "xiangqi") return side === "w" ? "Red" : "Black";
      return EN_COLORS[side] || side;
    }
    if (!state.meta) return side;
    const color = state.meta.colors.find((c) => c.id === side);
    return color ? color.name : side;
  }

  function gameName(meta) {
    if (!meta) return "";
    return isEN() ? (meta.englishName || meta.name) : meta.name;
  }

  function firstSide() {
    return state.meta ? state.meta.initialPosition.turn : "";
  }

  function isFinished(status) {
    return status && ["checkmate", "stalemate", "win", "draw"].includes(status.state);
  }

  function statusText(status) {
    if (!status) return T("进行中", "In progress");
    switch (status.state) {
      case "check":
        return T("被将军", "In check");
      case "checkmate":
        return isEN() ? `${sideName(status.winner)} wins by checkmate` : `${sideName(status.winner)}将杀获胜`;
      case "stalemate":
        if (isEN()) return status.winner ? `${sideName(status.winner)} wins by stalemate` : "Stalemate";
        return status.winner ? `${sideName(status.winner)}困毙获胜` : "困毙";
      case "win":
        return isEN() ? `${sideName(status.winner)} wins` : `${sideName(status.winner)}获胜`;
      case "draw":
        return T("和棋", "Draw");
      default:
        return T("进行中", "In progress");
    }
  }

  function defaultEngine(gameId) {
    return DEFAULT_ENGINE[gameId] || "builtin";
  }

  function positionPayload(view) {
    if (view.fen) return { fen: view.fen };
    return { board: view.board, turn: view.turn };
  }

  function moveInput(move) {
    if (!move) return {};
    if (move.uci) return { uci: move.uci };
    if (move.index && typeof move.index.index === "number") {
      return { index: move.index.index };
    }
    const out = {};
    if (move.from && typeof move.from.index === "number") out.from = move.from.index;
    if (move.to && typeof move.to.index === "number") out.to = move.to.index;
    if (move.promotion) out.promotion = move.promotion;
    return out;
  }

  async function api(path, body) {
    const res = await fetch(BASE + path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const text = await res.text();
    let payload;
    try {
      payload = JSON.parse(text);
    } catch (_) {
      throw new Error(T("服务返回了非 JSON 响应", "The server returned a non-JSON response"));
    }
    if (!res.ok || payload.ok !== true) {
      const err = payload.error || {};
      throw new Error(`${err.code || res.status}: ${err.message || T("请求失败", "Request failed")}`);
    }
    return payload.data;
  }

  function moveLabel(move) {
    if (!move) return "—";
    if (move.uci) return move.uci;
    if (move.from && move.to) return `${move.from.index}→${move.to.index}`;
    if (move.index) return `#${move.index.index}`;
    return "—";
  }

  function pieceSide(cell) {
    if (!cell) return "";
    if (state.meta.id === "xiangqi") return cell[0] === "r" ? "w" : "b";
    return cell[0];
  }

  function placementGame() {
    return state.meta && ["gomoku", "reversi", "connect4"].includes(state.meta.id);
  }

  function checkersGame() {
    return state.meta && state.meta.id === "checkers";
  }

  function toBoardIndex(displayRow, col) {
    const rows = state.meta.board.rows;
    if (state.meta.id === "chess") return (7 - displayRow) * 8 + col;
    if (state.meta.id === "xiangqi") return (9 - displayRow) * 9 + col;
    return displayRow * state.meta.board.cols + col;
  }

  function legalTarget(index) {
    if (!state.legal || !state.legal.length) return false;
    if (placementGame()) {
      if (state.meta.id === "gomoku") return false;
      if (state.meta.id === "connect4") {
        return state.legal.some((m) => m.index && m.index.index === index);
      }
      return state.legal.some((m) => m.index && m.index.index === index);
    }
    if (state.selected === null) return false;
    return state.legal.some((m) => m.from && m.to && m.from.index === state.selected && m.to.index === index);
  }

  function isLastMove(index) {
    const m = state.lastMove;
    if (!m) return false;
    if (m.index && m.index.index === index) return true;
    return (m.from && m.from.index === index) || (m.to && m.to.index === index);
  }

  function pieceNode(cell) {
    const span = document.createElement("span");
    const id = state.meta.id;
    if (id === "chess") {
      span.className = "piece " + (cell[0] === "w" ? "white-chess" : "black-chess");
      span.textContent = CHESS_GLYPH[cell[1]] || cell[1];
    } else if (id === "xiangqi") {
      span.className = "piece " + (cell[0] === "r" ? "red" : "black");
      const type = (cell[1] || "").toUpperCase();
      const glyph = XIANGQI_GLYPH[cell[0]] && XIANGQI_GLYPH[cell[0]][type];
      span.textContent = glyph || type;
    } else if (id === "jungle") {
      span.className = "piece " + (cell[0] === "r" ? "red" : "black");
      span.textContent = JUNGLE_GLYPH[cell[1]] || cell[1];
    } else if (id === "gomoku" || id === "reversi") {
      span.className = "stone " + (cell[0] === "b" ? "b" : "w");
    } else if (id === "connect4") {
      span.className = "stone " + (cell[0] === "r" ? "r" : "y");
    }
    return span;
  }

  // ---- move animation (slide / capture / placement) ----

  const ANIM = { slide: 240, jump: 320, ghost: 300, pop: 210, drop: 330, flip: 340 };

  function reduceMotion() {
    return typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  function pieceElement(index) {
    if (index === null || index === undefined || !el.board) return null;
    const i = String(index);
    return el.board.querySelector(
      `.board-cell[data-index="${i}"] .piece, .board-cell[data-index="${i}"] .stone, ` +
      `.checkers-hole[data-index="${i}"] .checkers-piece, ` +
      `.gomoku-piece[data-index="${i}"], .xq-piece[data-index="${i}"]`
    );
  }

  // Called before the board re-renders: snapshot the moving piece, the captured
  // piece and (reversi) flipped stones so the new DOM can be animated into place.
  function captureMoveAnim(move, nextBoard) {
    if (!move || reduceMotion()) return null;
    const board = (state.position && state.position.board) || [];
    const fromIdx = move.from && typeof move.from.index === "number" ? move.from.index : null;
    const toIdx = move.to && typeof move.to.index === "number"
      ? move.to.index
      : (move.index && typeof move.index.index === "number" ? move.index.index : null);
    const plan = {
      from: fromIdx,
      to: toIdx,
      jump: Boolean(move.jump),
      fromRect: null,
      ghost: null,
      flipped: [],
    };
    if (fromIdx !== null && toIdx !== null) {
      const fromNode = pieceElement(fromIdx);
      if (fromNode) plan.fromRect = fromNode.getBoundingClientRect();
      if (board[toIdx]) {
        const victimNode = pieceElement(toIdx);
        if (victimNode) plan.ghost = { at: toIdx, node: victimNode.cloneNode(true) };
      }
    }
    if (state.meta && state.meta.id === "reversi" && nextBoard) {
      for (let i = 0; i < board.length; i++) {
        if (board[i] && nextBoard[i] && board[i] !== nextBoard[i]) {
          plan.flipped.push({ index: i, cell: board[i] });
        }
      }
    }
    return plan;
  }

  function playMoveAnimation(plan) {
    if (!plan) return Promise.resolve();
    state.animating = true;
    const tasks = [];
    const svgBoard = el.board.querySelector(".xiangqi-svg");
    const placed = pieceElement(plan.to);
    if (plan.from !== null && plan.to !== null && placed) {
      if (state.meta.id === "xiangqi" && svgBoard) {
        tasks.push(svgSlide(placed, plan.from, plan.to, ANIM.slide));
      } else if (plan.fromRect) {
        tasks.push(flipSlide(placed, plan.fromRect, plan.jump));
      }
    } else if (plan.from === null && plan.to !== null && placed) {
      if (state.meta.id === "gomoku") tasks.push(svgPop(placed));
      else if (state.meta.id === "connect4") tasks.push(dropIn(placed));
      else tasks.push(popIn(placed));
    }
    if (plan.ghost) tasks.push(playGhost(plan.ghost, svgBoard));
    for (const f of plan.flipped) tasks.push(playFlip(f));
    return Promise.all(tasks).then(
      () => { state.animating = false; },
      () => { state.animating = false; }
    );
  }

  function flipSlide(node, fromRect, jump) {
    const to = node.getBoundingClientRect();
    const dx = fromRect.left + fromRect.width / 2 - (to.left + to.width / 2);
    const dy = fromRect.top + fromRect.height / 2 - (to.top + to.height / 2);
    if (!dx && !dy) return Promise.resolve();
    const lift = Math.max(16, Math.abs(dx) * 0.22, Math.abs(dy) * 0.22);
    const frames = jump
      ? [
          { transform: `translate(${dx}px, ${dy}px)` },
          { transform: `translate(${dx / 2}px, ${dy / 2 - lift}px)`, offset: 0.5 },
          { transform: "translate(0, 0)" },
        ]
      : [
          { transform: `translate(${dx}px, ${dy}px)` },
          { transform: "translate(0, 0)" },
        ];
    return node
      .animate(frames, {
        duration: jump ? ANIM.jump : ANIM.slide,
        easing: jump ? "ease-in-out" : "cubic-bezier(0.22, 0.72, 0.28, 1)",
      })
      .finished.catch(() => {});
  }

  // SVG pieces carry their position in a translate() attribute; tween it directly.
  function svgSlide(node, fromIdx, toIdx, ms) {
    const a = xqPoint(fromIdx);
    const b = xqPoint(toIdx);
    return new Promise((resolve) => {
      const start = performance.now();
      const tick = (now) => {
        const t = Math.min(1, (now - start) / ms);
        const e = t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
        node.setAttribute("transform", `translate(${a.x + (b.x - a.x) * e} ${a.y + (b.y - a.y) * e})`);
        if (t < 1) requestAnimationFrame(tick);
        else {
          node.setAttribute("transform", `translate(${b.x} ${b.y})`);
          resolve();
        }
      };
      requestAnimationFrame(tick);
    });
  }

  function svgPop(node) {
    const m = (node.getAttribute("transform") || "").match(/translate\(\s*([-\d.]+)[ ,]+([-\d.]+)\s*\)/);
    if (!m) return Promise.resolve();
    const x = Number(m[1]);
    const y = Number(m[2]);
    return new Promise((resolve) => {
      const start = performance.now();
      const tick = (now) => {
        const t = Math.min(1, (now - start) / (ANIM.pop + 50));
        const c = 1.7; // ease-out-back: slight overshoot
        const e = 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
        node.setAttribute("transform", `translate(${x} ${y}) scale(${Math.max(0.08, e)})`);
        if (t < 1) requestAnimationFrame(tick);
        else {
          node.setAttribute("transform", `translate(${x} ${y})`);
          resolve();
        }
      };
      requestAnimationFrame(tick);
    });
  }

  function popIn(node) {
    return node
      .animate(
        [
          { transform: "scale(0.35)", opacity: 0.35 },
          { transform: "scale(1)", opacity: 1 },
        ],
        { duration: ANIM.pop, easing: "cubic-bezier(0.22, 1.2, 0.36, 1)" }
      )
      .finished.catch(() => {});
  }

  function dropIn(node) {
    return node
      .animate(
        [
          { transform: "translateY(-85%)", opacity: 0.55, easing: "cubic-bezier(0.4, 0, 0.9, 0.6)" },
          { transform: "translateY(5%)", opacity: 1, easing: "cubic-bezier(0.2, 0.7, 0.3, 1)" },
          { transform: "translateY(0)" },
        ],
        { duration: ANIM.drop }
      )
      .finished.catch(() => {});
  }

  function playGhost(ghost, svgBoard) {
    const node = ghost.node;
    if (svgBoard && node.namespaceURI === "http://www.w3.org/2000/svg") {
      svgBoard.appendChild(node);
      const m = (node.getAttribute("transform") || "").match(/translate\(\s*([-\d.]+)[ ,]+([-\d.]+)\s*\)/);
      const x = m ? Number(m[1]) : 0;
      const y = m ? Number(m[2]) : 0;
      return new Promise((resolve) => {
        const start = performance.now();
        const tick = (now) => {
          const t = Math.min(1, (now - start) / ANIM.ghost);
          const e = 1 - Math.pow(1 - t, 2);
          node.setAttribute("transform", `translate(${x} ${y}) scale(${1 - 0.42 * e})`);
          node.setAttribute("opacity", String(1 - e));
          if (t < 1) requestAnimationFrame(tick);
          else {
            node.remove();
            resolve();
          }
        };
        requestAnimationFrame(tick);
      });
    }
    const cell = el.board.querySelector(
      `.board-cell[data-index="${ghost.at}"], .checkers-hole[data-index="${ghost.at}"]`
    );
    if (!cell) return Promise.resolve();
    const wrap = document.createElement("div");
    wrap.className = "board-ghost";
    wrap.appendChild(node);
    cell.appendChild(wrap);
    return wrap
      .animate(
        [
          { opacity: 1, transform: "scale(1)" },
          { opacity: 0, transform: "scale(0.5)" },
        ],
        { duration: ANIM.ghost, easing: "cubic-bezier(0.3, 0, 0.4, 1)" }
      )
      .finished.then(() => wrap.remove())
      .catch(() => wrap.remove());
  }

  // Reversi: old-color stone shrinks away while the flipped stone grows in.
  function playFlip(f) {
    const cell = el.board.querySelector(`.board-cell[data-index="${f.index}"]`);
    const stone = cell && cell.querySelector(".stone");
    if (!cell || !stone) return Promise.resolve();
    const wrap = document.createElement("div");
    wrap.className = "board-ghost";
    wrap.appendChild(pieceNode(f.cell));
    cell.appendChild(wrap);
    const grow = stone.animate(
      [
        { transform: "scaleX(0)", offset: 0 },
        { transform: "scaleX(0)", offset: 0.5 },
        { transform: "scaleX(1)", offset: 1 },
      ],
      { duration: ANIM.flip, easing: "ease-in-out" }
    );
    const shrink = wrap.animate(
      [
        { transform: "scaleX(1)", offset: 0 },
        { transform: "scaleX(1)", offset: 0.5 },
        { transform: "scaleX(0)", offset: 1 },
      ],
      { duration: ANIM.flip, easing: "ease-in-out" }
    );
    return Promise.all([grow.finished, shrink.finished])
      .then(() => wrap.remove())
      .catch(() => wrap.remove());
  }

  function jungleClass(row, col) {
    const idx = row * 7 + col;
    if ((row >= 3 && row <= 5) && [1, 2, 4, 5].includes(col)) return "water";
    if (row === 0 && [2, 4].includes(col)) return "trap-black";
    if (row === 1 && col === 3) return "trap-black";
    if (row === 8 && [2, 4].includes(col)) return "trap-red";
    if (row === 7 && col === 3) return "trap-red";
    if (idx === 3) return "den-black";
    if (idx === 8 * 7 + 3) return "den-red";
    return "";
  }

  function renderGrid() {
    const meta = state.meta;
    const rows = meta.board.rows;
    const cols = meta.board.cols;
    const board = state.position.board || [];
    const grid = document.createElement("div");
    grid.className = `board-grid ${meta.id} cols-${cols}`;
    grid.style.setProperty("--cols", cols);
    grid.style.setProperty("--rows", rows);

    for (let row = 0; row < rows; row++) {
      for (let col = 0; col < cols; col++) {
        const index = toBoardIndex(row, col);
        const cell = board[index] || "";
        const button = document.createElement("button");
        button.type = "button";
        button.className = "board-cell";
        button.dataset.index = String(index);
        if (meta.id === "chess" && (row + col) % 2 === 1) button.classList.add("dark");
        if (meta.id === "xiangqi" && col >= 3 && col <= 5 && (row <= 2 || row >= 7)) button.classList.add("palace");
        if (meta.id === "jungle") {
          const cls = jungleClass(row, col);
          if (cls) {
            button.classList.add(cls);
            if (!cell) {
              const label = document.createElement("span");
              label.className = "terrain-label";
              label.textContent = cls === "water" ? T("水", "W") : cls.startsWith("trap") ? T("陷", "T") : T("穴", "D");
              button.appendChild(label);
            }
          }
        }
        if (meta.id === "gomoku" && isGomokuStar(row, col)) {
          const star = document.createElement("span");
          star.className = "star-dot";
          button.appendChild(star);
        }
        if (cell) {
          button.appendChild(pieceNode(cell));
          button.classList.add("has-piece");
        }
        if (index === state.selected) button.classList.add("selected");
        if (legalTarget(index)) button.classList.add("legal");
        if (isLastMove(index)) button.classList.add("last");
        button.addEventListener("click", () => onBoardClick(index));
        grid.appendChild(button);
      }
    }
    if (meta.id === "xiangqi") {
      const palaceTop = document.createElement("div");
      palaceTop.className = "board-palace palace-top";
      const palaceBottom = document.createElement("div");
      palaceBottom.className = "board-palace palace-bottom";
      const river = document.createElement("div");
      river.className = "board-river";
      river.innerHTML = "<span>楚 河</span><span>汉 界</span>";
      grid.append(palaceTop, palaceBottom, river);
    }
    el.board.replaceChildren(grid);
    el.board.classList.add("is-ready");
    renderCaption();
  }

  const GOMOKU = { view: 800, pad: 50, cell: 50 };

  function gomokuPoint(index) {
    const row = Math.floor(index / 15);
    const col = index % 15;
    return { x: GOMOKU.pad + col * GOMOKU.cell, y: GOMOKU.pad + row * GOMOKU.cell, row, col };
  }

  function renderGomoku() {
    const size = GOMOKU.view;
    const svg = svgNode("svg", {
      class: "gomoku-svg",
      viewBox: `0 0 ${size} ${size}`,
      role: "img",
      "aria-label": T("五子棋棋盘", "Gomoku board"),
    });
    const defs = svgNode("defs");
    defs.innerHTML = `
      <pattern id="gomoku-wood" width="80" height="80" patternUnits="userSpaceOnUse">
        <path d="M 0 40 Q 24 28 42 40 T 80 40" stroke="rgba(125,82,44,0.08)" fill="none" stroke-width="1.2"/>
        <path d="M 12 20 Q 32 10 54 20 T 86 20" stroke="rgba(125,82,44,0.06)" fill="none" stroke-width="1"/>
      </pattern>
      <radialGradient id="gomoku-black" cx="34%" cy="28%" r="76%">
        <stop offset="0" stop-color="#59605c"/>
        <stop offset="0.42" stop-color="#252b28"/>
        <stop offset="1" stop-color="#0d1110"/>
      </radialGradient>
      <radialGradient id="gomoku-white" cx="34%" cy="28%" r="76%">
        <stop offset="0" stop-color="#ffffff"/>
        <stop offset="0.64" stop-color="#f1e9da"/>
        <stop offset="1" stop-color="#cfc3ae"/>
      </radialGradient>
      <filter id="gomoku-shadow" x="-30%" y="-30%" width="160%" height="170%">
        <feDropShadow dx="0" dy="3" stdDeviation="2" flood-color="#2f2416" flood-opacity="0.3"/>
      </filter>`;
    svg.appendChild(defs);
    svg.appendChild(svgNode("rect", { width: size, height: size, rx: 18, fill: "#e4c38b" }));
    svg.appendChild(svgNode("rect", { width: size, height: size, rx: 18, fill: "url(#gomoku-wood)" }));
    svg.appendChild(svgNode("rect", { x: 20, y: 20, width: 760, height: 760, rx: 6, fill: "none", stroke: "rgba(110,72,35,0.55)", "stroke-width": 3 }));
    svg.appendChild(svgNode("rect", { x: 30, y: 30, width: 740, height: 740, rx: 3, fill: "none", stroke: "rgba(110,72,35,0.24)", "stroke-width": 1.5 }));

    for (let i = 0; i < 15; i++) {
      const p = GOMOKU.pad + i * GOMOKU.cell;
      svg.appendChild(svgNode("line", { x1: GOMOKU.pad, y1: p, x2: size - GOMOKU.pad, y2: p, class: "gomoku-line" }));
      svg.appendChild(svgNode("line", { x1: p, y1: GOMOKU.pad, x2: p, y2: size - GOMOKU.pad, class: "gomoku-line" }));
    }
    for (const [row, col] of [[3, 3], [3, 11], [7, 7], [11, 3], [11, 11]]) {
      const p = gomokuPoint(row * 15 + col);
      svg.appendChild(svgNode("circle", { cx: p.x, cy: p.y, r: 5, class: "gomoku-star" }));
    }

    const lastIndex = state.lastMove && state.lastMove.index ? state.lastMove.index.index : null;
    if (typeof lastIndex === "number") {
      const p = gomokuPoint(lastIndex);
      svg.appendChild(svgNode("circle", { cx: p.x, cy: p.y, r: 23, class: "gomoku-last" }));
    }

    const board = state.position.board || [];
    for (let index = 0; index < board.length; index++) {
      const stone = board[index];
      if (!stone) continue;
      const p = gomokuPoint(index);
      const group = svgNode("g", {
        class: `gomoku-piece ${stone}`,
        filter: "url(#gomoku-shadow)",
        transform: `translate(${p.x} ${p.y})`,
        "data-index": String(index),
      });
      group.appendChild(svgNode("circle", { cx: 0, cy: 0, r: 22, fill: stone === "b" ? "url(#gomoku-black)" : "url(#gomoku-white)" }));
      group.appendChild(svgNode("circle", { cx: -6, cy: -7, r: 5.5, fill: "rgba(255,255,255,0.22)" }));
      svg.appendChild(group);
    }

    const shell = document.createElement("div");
    shell.className = "gomoku-shell";
    shell.appendChild(svg);
    const hitLayer = document.createElement("div");
    hitLayer.className = "gomoku-hit-layer";
    for (let index = 0; index < 225; index++) {
      const p = gomokuPoint(index);
      const hit = document.createElement("button");
      hit.type = "button";
      hit.className = "gomoku-hit";
      hit.dataset.index = String(index);
      hit.style.left = `${(p.x / size) * 100}%`;
      hit.style.top = `${(p.y / size) * 100}%`;
      hit.style.width = `${(GOMOKU.cell / size) * 100}%`;
      hit.style.height = `${(GOMOKU.cell / size) * 100}%`;
      hit.setAttribute("aria-label", T(`第 ${p.row + 1} 行第 ${p.col + 1} 列`, `Row ${p.row + 1}, column ${p.col + 1}`));
      hit.addEventListener("click", () => onBoardClick(index));
      hitLayer.appendChild(hit);
    }
    shell.appendChild(hitLayer);
    el.board.replaceChildren(shell);
    el.board.classList.add("is-ready");
    renderCaption();
  }

  function isGomokuStar(row, col) {
    if (state.meta.id !== "gomoku") return false;
    return (row === 3 && col === 3) || (row === 3 && col === 11) ||
      (row === 7 && col === 7) || (row === 11 && col === 3) || (row === 11 && col === 11);
  }

  function checkerGeometry() {
    const holes = [];
    let id = 0;
    for (let row = 0; row < ROW_COUNTS.length; row++) {
      const count = ROW_COUNTS[row];
      for (let i = 0; i < count; i++) {
        const x2 = i * 2 - (count - 1);
        holes.push({
          id,
          row,
          x2,
          cx: 310 + x2 * 22,
          cy: 54 + row * 42,
        });
        id++;
      }
    }
    return holes;
  }

  function renderCheckers() {
    const shell = document.createElement("div");
    shell.className = "checkers-shell";
    const board = document.createElement("div");
    board.className = "checkers-board";
    board.setAttribute("role", "grid");
    board.setAttribute("aria-label", T("中国跳棋棋盘", "Chinese Checkers board"));

    const topLabel = document.createElement("span");
    topLabel.className = "checkers-home-label top";
    topLabel.textContent = T("黑方营", "Black camp");
    const bottomLabel = document.createElement("span");
    bottomLabel.className = "checkers-home-label bottom";
    bottomLabel.textContent = T("红方营", "Red camp");
    board.append(topLabel, bottomLabel);

    const cells = state.position.board || [];
    const lastTo = state.lastMove && state.lastMove.to ? state.lastMove.to.index : null;
    for (const hole of checkerGeometry()) {
      const cell = cells[hole.id] || "";
      const button = document.createElement("button");
      button.type = "button";
      button.className = "checkers-hole";
      button.dataset.index = String(hole.id);
      button.style.left = `${3 + ((hole.x2 + 12) / 24) * 94}%`;
      button.style.top = `${3 + ((hole.row + 0.5) / 17) * 94}%`;
      if (hole.row <= 3) button.classList.add("in-black-home");
      if (hole.row >= 13) button.classList.add("in-red-home");
      if (state.selected === hole.id) button.classList.add("is-selected");
      if (legalTarget(hole.id)) button.classList.add("is-target");
      if (lastTo === hole.id) button.classList.add("is-last");
      button.setAttribute("aria-label", T(`第 ${hole.row + 1} 行，${hole.x2}`, `Row ${hole.row + 1}, ${hole.x2}`));
      button.addEventListener("click", () => onCheckerClick(hole.id));

      if (cell) {
        const piece = document.createElement("span");
        piece.className = `checkers-piece ${cell}`;
        button.appendChild(piece);
      }
      if (legalTarget(hole.id)) {
        const target = document.createElement("span");
        target.className = "checkers-target";
        button.appendChild(target);
      }
      board.appendChild(button);
    }
    shell.appendChild(board);
    el.board.replaceChildren(shell);
    el.board.classList.add("is-ready");
    renderCaption();
  }

  function renderBoard() {
    el.board.classList.remove("is-ready");
    if (!state.position) return;
    el.board.dataset.game = state.meta.id;
    if (state.meta.id === "xiangqi") {
      renderXiangqi();
      return;
    }
    if (state.meta.id === "gomoku") {
      renderGomoku();
      return;
    }
    if (checkersGame()) {
      renderCheckers();
      return;
    }
    renderGrid();
  }

  const XQ = { pad: 50, cell: 100, width: 900, height: 1000 };

  function xqPoint(index) {
    const file = index % 9;
    const rank = Math.floor(index / 9);
    const flipped = state.humanSide === "b";
    const col = flipped ? 8 - file : file;
    const row = flipped ? rank : 9 - rank;
    return {
      x: XQ.pad + col * XQ.cell,
      y: XQ.pad + row * XQ.cell,
      col,
      row,
    };
  }

  function xqSquare(file, rank) {
    return rank * 9 + file;
  }

  function svgNode(name, attrs) {
    const node = document.createElementNS("http://www.w3.org/2000/svg", name);
    Object.entries(attrs || {}).forEach(([key, value]) => node.setAttribute(key, String(value)));
    return node;
  }

  function renderXiangqi() {
    const svg = svgNode("svg", {
      class: "xiangqi-svg",
      viewBox: `0 0 ${XQ.width} ${XQ.height}`,
      role: "img",
      "aria-label": T("中国象棋棋盘", "Xiangqi board"),
    });
    const defs = svgNode("defs");
    defs.innerHTML = `
      <filter id="xq-piece-shadow" x="-30%" y="-30%" width="160%" height="180%">
        <feDropShadow dx="0" dy="5" stdDeviation="3" flood-color="#3f2a1a" flood-opacity="0.24"/>
      </filter>
      <pattern id="xq-wood-grain" width="70" height="70" patternUnits="userSpaceOnUse">
        <path d="M 0 34 Q 20 22 38 34 T 70 34" stroke="rgba(125,82,44,0.09)" fill="none" stroke-width="1.2"/>
        <path d="M 10 18 Q 28 8 50 18 T 82 18" stroke="rgba(125,82,44,0.07)" fill="none" stroke-width="1"/>
      </pattern>
      <radialGradient id="xq-ivory" cx="34%" cy="24%" r="78%">
        <stop offset="0" stop-color="#fffaf0"/>
        <stop offset="0.56" stop-color="#eed7b0"/>
        <stop offset="1" stop-color="#c99a5e"/>
      </radialGradient>`;
    svg.appendChild(defs);

    svg.appendChild(svgNode("rect", { width: 900, height: 1000, rx: 22, fill: "#e8ca92" }));
    svg.appendChild(svgNode("rect", { width: 900, height: 1000, rx: 22, fill: "url(#xq-wood-grain)" }));
    svg.appendChild(svgNode("rect", { x: 25, y: 25, width: 850, height: 950, rx: 8, fill: "none", stroke: "rgba(118,76,41,0.55)", "stroke-width": 3 }));
    svg.appendChild(svgNode("rect", { x: 35, y: 35, width: 830, height: 930, rx: 4, fill: "none", stroke: "rgba(118,76,41,0.28)", "stroke-width": 1.5 }));

    for (let rank = 0; rank < 10; rank++) {
      const y = XQ.pad + rank * XQ.cell;
      svg.appendChild(svgNode("line", { x1: XQ.pad, y1: y, x2: XQ.width - XQ.pad, y2: y, class: "xq-grid-line" }));
    }
    for (let file = 0; file < 9; file++) {
      const x = XQ.pad + file * XQ.cell;
      svg.appendChild(svgNode("line", { x1: x, y1: XQ.pad, x2: x, y2: XQ.pad + 4 * XQ.cell, class: "xq-grid-line" }));
      svg.appendChild(svgNode("line", { x1: x, y1: XQ.pad + 5 * XQ.cell, x2: x, y2: XQ.height - XQ.pad, class: "xq-grid-line" }));
    }

    const palaceLines = [
      [3, 0, 5, 2], [5, 0, 3, 2],
      [3, 7, 5, 9], [5, 7, 3, 9],
    ];
    for (const [f1, r1, f2, r2] of palaceLines) {
      const a = xqPoint(xqSquare(f1, r1));
      const b = xqPoint(xqSquare(f2, r2));
      svg.appendChild(svgNode("line", { x1: a.x, y1: a.y, x2: b.x, y2: b.y, class: "xq-palace-line" }));
    }

    const riverLeft = svgNode("text", { x: 265, y: 513, "text-anchor": "middle", class: "xq-river-text" });
    riverLeft.textContent = "楚 河";
    const riverRight = svgNode("text", { x: 635, y: 513, "text-anchor": "middle", class: "xq-river-text" });
    riverRight.textContent = "汉 界";
    svg.append(riverLeft, riverRight);

    if (state.lastMove) {
      for (const [idx, radius, cls] of [
        [state.lastMove.from && state.lastMove.from.index, 15, "xq-last-from"],
        [state.lastMove.to && state.lastMove.to.index, 43, "xq-last-to"],
      ]) {
        if (typeof idx !== "number") continue;
        const p = xqPoint(idx);
        svg.appendChild(svgNode("circle", { cx: p.x, cy: p.y, r: radius, class: cls }));
      }
    }

    if (state.selected !== null) {
      const targets = state.legal.filter((m) => m.from && m.to && m.from.index === state.selected);
      for (const target of targets) {
        const p = xqPoint(target.to.index);
        const occupied = Boolean(state.position.board[target.to.index]);
        svg.appendChild(svgNode("circle", {
          cx: p.x, cy: p.y, r: occupied ? 44 : 10,
          class: occupied ? "xq-legal-capture" : "xq-legal-dot",
        }));
      }
    }

    const board = state.position.board || [];
    for (let index = 0; index < board.length; index++) {
      const cell = board[index];
      if (!cell) continue;
      const p = xqPoint(index);
      const group = svgNode("g", {
        class: `xq-piece ${cell[0] === "r" ? "red" : "black"}${state.selected === index ? " selected" : ""}`,
        transform: `translate(${p.x} ${p.y})`,
        filter: "url(#xq-piece-shadow)",
        "data-index": String(index),
      });
      group.appendChild(svgNode("circle", { r: 43, class: "xq-piece-disc" }));
      group.appendChild(svgNode("circle", { r: 36, class: "xq-piece-ring" }));
      const type = (cell[1] || "").toUpperCase();
      const glyph = svgNode("text", { class: "xq-piece-glyph", "text-anchor": "middle", "dominant-baseline": "central" });
      glyph.textContent = (XIANGQI_GLYPH[cell[0]] && XIANGQI_GLYPH[cell[0]][type]) || type;
      group.appendChild(glyph);
      svg.appendChild(group);
    }

    const shell = document.createElement("div");
    shell.className = "xiangqi-shell";
    shell.appendChild(svg);

    const hitLayer = document.createElement("div");
    hitLayer.className = "xiangqi-hit-layer";
    for (let index = 0; index < 90; index++) {
      const p = xqPoint(index);
      const hit = document.createElement("button");
      hit.type = "button";
      hit.className = "xiangqi-hit";
      hit.dataset.index = String(index);
      hit.style.left = `${(p.x / XQ.width) * 100}%`;
      hit.style.top = `${(p.y / XQ.height) * 100}%`;
      hit.style.width = `${(XQ.cell / XQ.width) * 100}%`;
      hit.style.height = `${(XQ.cell / XQ.height) * 100}%`;
      const cell = board[index] || "";
      hit.setAttribute("aria-label", cell ? `${cell[0] === "r" ? "红" : "黑"}${cell[1]}` : T("空位", "Empty"));
      hit.addEventListener("click", () => onBoardClick(index));
      hitLayer.appendChild(hit);
    }
    shell.appendChild(hitLayer);
    el.board.replaceChildren(shell);
    el.board.classList.add("is-ready");
    renderCaption();
  }

  function renderCaption() {
    if (!el.caption || !state.position) return;
    const turn = state.position.turn;
    const status = state.position.status || {};
    let text = T(`轮到 ${sideName(turn)}`, `${sideName(turn)} to move`);
    if (isFinished(status)) text = statusText(status);
    else if (status.state === "check") text = T(`${sideName(turn)}被将军`, `${sideName(turn)} is in check`);
    el.caption.innerHTML = `<span><strong>${escapeHTML(text)}</strong></span><span>${escapeHTML(gameName(state.meta))} · ${escapeHTML(isEN() ? (EN_MOVE_FORMAT[state.meta.id] || state.meta.moveFormat) : state.meta.moveFormat)}</span>`;
  }

  function renderStatus() {
    if (!el.status) return;
    let label = T("进行中", "In progress");
    let cls = "playing";
    if (state.busy) {
      label = T("AI 思考中", "AI is thinking");
      cls = "thinking";
    } else if (isFinished(state.position && state.position.status)) {
      label = statusText(state.position.status);
      cls = "over";
    } else if (state.position) {
      label = T(`轮到 ${sideName(state.position.turn)}`, `${sideName(state.position.turn)} to move`);
    }
    el.status.textContent = label;
    el.status.className = "status-pill " + cls;
  }

  function renderInfo() {
    if (!state.meta || !state.position) return;
    el.title.textContent = gameName(state.meta);
    if (el.infoTurn) el.infoTurn.textContent = sideName(state.position.turn) + (state.busy ? T("（AI）", " (AI)") : "");
    if (el.infoEngine) el.infoEngine.textContent = state.engine || defaultEngine(state.meta.id);
    if (el.infoStatus) el.infoStatus.textContent = statusText(state.position.status);
    if (el.infoRecord) el.infoRecord.textContent = state.history.length && state.recordId ? state.recordId.slice(-6) : "—";
    el.undo.disabled = state.busy || state.undoStack.length === 0 || state.history.length < 2;
    el.newGame.disabled = state.busy;
    el.game.disabled = state.busy;
    el.side.disabled = state.busy;
    el.difficulty.disabled = state.busy;
    if (el.engine) el.engine.disabled = state.busy;
    if (el.apiMode) el.apiMode.disabled = state.busy;
    if (el.hint) {
      el.hint.textContent = isFinished(state.position.status)
        ? T("对局结束。可以新开一局。", "Game over. Start a new game.")
        : T("点击棋盘落子。", "Click the board to move.");
    }
  }

  function renderMoves() {
    if (!el.moveList) return;
    if (!state.history.length) {
      el.moveList.innerHTML = `<li class="muted">${escapeHTML(T("还没有走子", "No moves yet"))}</li>`;
      return;
    }
    el.moveList.innerHTML = state.history
      .map((entry, index) => `<li><span>${String(index + 1).padStart(2, "0")}.</span> <b>${entry.by === "human" ? T("你", "You") : "AI"}</b> ${escapeHTML(moveLabel(entry.move))}</li>`)
      .join("");
    el.moveList.scrollTop = el.moveList.scrollHeight;
  }

  function renderProgress() {
    if (!el.aiCard) return;
    if (state.busy) {
      el.aiCard.style.display = "";
      el.aiLabel.textContent = T("AI 思考中", "AI is thinking");
      if (state.aiDepth > 0) {
        el.aiMeta.textContent = T(`深度 ${state.aiDepth}`, `Depth ${state.aiDepth}`);
        el.aiBar.classList.remove("indeterminate");
        el.aiBar.style.width = `${Math.min(92, 18 + state.aiDepth * 6)}%`;
      } else {
        el.aiMeta.textContent = T("正在连接引擎…", "Connecting to engine...");
        el.aiBar.classList.add("indeterminate");
        el.aiBar.style.width = "";
      }
    } else if (isFinished(state.position && state.position.status)) {
      el.aiBar.classList.remove("indeterminate");
      el.aiBar.style.width = "100%";
      el.aiLabel.textContent = T("对局结束", "Game over");
      el.aiMeta.textContent = statusText(state.position.status);
    } else {
      el.aiBar.classList.remove("indeterminate");
      el.aiBar.style.width = "0%";
      el.aiLabel.textContent = T("轮到你走", "Your move");
      el.aiMeta.textContent = state.selected !== null
        ? T("点击绿色落点", "Click a highlighted destination")
        : T("选择一枚己方棋子", "Select one of your pieces");
    }
  }

  function render() {
    if (!state.meta || !state.position) return;
    renderStatus();
    renderInfo();
    renderMoves();
    renderProgress();
    renderBoard();
    renderRecords();
  }

  function engineSelectOptions(meta) {
    if (!el.engine) return;
    const engines = Array.isArray(meta.engines) && meta.engines.length ? meta.engines : ["builtin"];
    const keep = engines.includes(el.engine.value) ? el.engine.value : "";
    el.engine.innerHTML = "";
    const def = document.createElement("option");
    def.value = "";
    def.textContent = T(`默认（${engines[0]}）`, `Default (${engines[0]})`);
    el.engine.appendChild(def);
    for (const name of engines) {
      const opt = document.createElement("option");
      opt.value = name;
      opt.textContent = name;
      el.engine.appendChild(opt);
    }
    el.engine.value = keep;
  }

  function sideSelectOptions(meta, selected) {
    if (!el.side) return;
    const first = meta.initialPosition.turn;
    el.side.innerHTML = meta.colors
      .map((c) => {
        const suffix = c.id === first ? T("（先手）", " (first)") : T("（后手）", " (second)");
        const label = isEN()
          ? (meta.id === "xiangqi" ? (c.id === "w" ? "Red" : "Black") : (EN_COLORS[c.id] || c.name))
          : c.name;
        return `<option value="${escapeHTML(c.id)}">${escapeHTML(label)}${suffix}</option>`;
      })
      .join("");
    el.side.value = selected || first;
  }

  function newGame(options = {}) {
    if (!state.games.length) return;
    const gameId = options.gameId || (el.game && el.game.value) || state.games[0].id;
    const meta = state.games.find((g) => g.id === gameId) || state.games[0];
    state.meta = meta;
    state.difficulty = (el.difficulty && el.difficulty.value) || "normal";
    state.humanSide = options.humanSide || (el.side && el.side.value) || meta.initialPosition.turn;
    if (!meta.colors.some((c) => c.id === state.humanSide)) {
      state.humanSide = meta.initialPosition.turn;
    }
    state.position = clone(meta.initialPosition);
    state.legal = state.position.legalMoves || [];
    state.history = [];
    state.undoStack = [];
    state.selected = null;
    state.busy = false;
    state.lastMove = null;
    state.aiDepth = 0;
    state.engine = defaultEngine(meta.id);
    state.recordId = newRecordId();
    el.game.value = meta.id;
    sideSelectOptions(meta, state.humanSide);
    engineSelectOptions(meta);
    el.difficulty.value = state.difficulty;
    saveRecord();
    render();
    if (state.position.turn !== state.humanSide) {
      aiTurn();
    }
  }

  function resumeRecord(record) {
    if (!record || state.busy) return;
    const meta = state.games.find((g) => g.id === record.game);
    if (!meta) return;
    state.meta = meta;
    state.position = clone(record.position);
    state.legal = state.position.legalMoves || [];
    state.history = Array.isArray(record.history) ? record.history : [];
    state.undoStack = [];
    state.humanSide = record.humanSide || meta.initialPosition.turn;
    state.difficulty = record.difficulty || "normal";
    state.engine = record.engine || defaultEngine(meta.id);
    state.recordId = record.id;
    state.selected = null;
    state.busy = false;
    state.lastMove = state.history.length ? state.history[state.history.length - 1].move : null;
    state.aiDepth = 0;
    el.game.value = meta.id;
    sideSelectOptions(meta, state.humanSide);
    engineSelectOptions(meta);
    el.difficulty.value = state.difficulty;
    saveRecord();
    render();
    if (state.position.turn !== state.humanSide && !isFinished(state.position.status)) {
      aiTurn();
    }
  }

  function loadRecords() {
    try {
      const list = JSON.parse(localStorage.getItem(RECORD_KEY) || "[]");
      return Array.isArray(list) ? list : [];
    } catch (_) {
      return [];
    }
  }

  function writeRecords(records) {
    try {
      localStorage.setItem(RECORD_KEY, JSON.stringify(records.slice(0, RECORD_LIMIT)));
    } catch (_) {
      // ignore quota/privacy failures
    }
  }

  function newRecordId() {
    return `${state.meta ? state.meta.id : "game"}-${Date.now().toString(36)}`;
  }

  function syncPlayURL() {
    if (!state.meta) return;
    try {
      const url = new URL(location.href);
      url.searchParams.set("game", state.meta.id);
      if (state.recordId && state.history.length) url.searchParams.set("record", state.recordId);
      else url.searchParams.delete("record");
      history.replaceState(null, "", url);
      const langLink = document.querySelector(".lang-btn");
      if (langLink) {
        const target = new URL(url.toString());
        target.searchParams.set("lang", isEN() ? "zh" : "en");
        langLink.href = target.toString();
      }
    } catch (_) {
      // ignore history failures
    }
  }

  function saveRecord() {
    if (!state.meta || !state.position) return;
    if (!state.recordId) state.recordId = newRecordId();
    syncPlayURL();
    const records = loadRecords();
    if (state.history.length === 0) {
      writeRecords(records.filter((r) => r.id !== state.recordId));
      return;
    }
    const record = {
      id: state.recordId,
      game: state.meta.id,
      humanSide: state.humanSide,
      difficulty: state.difficulty,
      position: state.position,
      history: state.history,
      engine: state.engine,
      updatedAt: Date.now(),
    };
    const index = records.findIndex((r) => r.id === record.id);
    if (index >= 0) records[index] = record;
    else records.unshift(record);
    records.sort((a, b) => b.updatedAt - a.updatedAt);
    writeRecords(records);
  }

  function renderRecords() {
    if (!el.records) return;
    const records = loadRecords();
    const card = el.records.closest(".doc-card");
    if (card) card.hidden = records.length === 0;
    if (!records.length) {
      el.records.innerHTML = `<span class="muted">${T("暂无记录", "No saved games yet")}</span>`;
      return;
    }
    el.records.innerHTML = records
      .map((record) => {
        const meta = state.games.find((g) => g.id === record.game);
        const name = meta ? gameName(meta) : record.game;
        const side = meta ? sideNameFor(meta, record.humanSide) : record.humanSide;
        const time = new Date(record.updatedAt).toLocaleString();
        return `<div class="record-item">
          <span class="rec-game">${escapeHTML(name)}</span>
          <span class="rec-side">${escapeHTML(side)}</span>
          <span class="rec-time">${escapeHTML(time)}</span>
          <button type="button" data-resume="${escapeHTML(record.id)}">${T("继续", "Resume")}</button>
          <button type="button" data-delete="${escapeHTML(record.id)}" title="${T("删除", "Delete")}">×</button>
        </div>`;
      })
      .join("");
    el.records.querySelectorAll("[data-resume]").forEach((button) => {
      button.addEventListener("click", () => {
        const record = loadRecords().find((r) => r.id === button.dataset.resume);
        if (record) resumeRecord(record);
      });
    });
    el.records.querySelectorAll("[data-delete]").forEach((button) => {
      button.addEventListener("click", () => {
        const records = loadRecords().filter((r) => r.id !== button.dataset.delete);
        writeRecords(records);
        if (button.dataset.delete === state.recordId) {
          state.recordId = "";
          syncPlayURL();
          renderInfo();
        }
        renderRecords();
      });
    });
  }

  function sideNameFor(meta, id) {
    if (isEN()) {
      if (meta.id === "xiangqi") return id === "w" ? "Red" : "Black";
      return EN_COLORS[id] || id;
    }
    const color = meta.colors.find((c) => c.id === id);
    return color ? color.name : id;
  }

  async function onBoardClick(index) {
    if (state.busy || state.animating || !state.position || isFinished(state.position.status)) return;
    const cell = state.position.board[index] || "";
    if (placementGame()) {
      let move = null;
      if (state.meta.id === "connect4") {
        const col = index % state.meta.board.cols;
        move = state.legal.find((m) => m.index && m.index.col === col);
      } else {
        move = state.legal.find((m) => m.index && m.index.index === index);
      }
      if (move) await applyHuman(move);
      return;
    }
    if (legalTarget(index)) {
      const move = state.legal.find((m) => m.from && m.to && m.from.index === state.selected && m.to.index === index);
      if (move) await applyHuman(move);
      return;
    }
    if (cell && pieceSide(cell) === state.humanSide) {
      state.selected = index;
      renderBoard();
      renderProgress();
      return;
    }
    state.selected = null;
    renderBoard();
    renderProgress();
  }

  async function onCheckerClick(index) {
    if (state.busy || state.animating || !state.position || isFinished(state.position.status)) return;
    const cell = state.position.board[index] || "";
    if (legalTarget(index)) {
      const move = state.legal.find((m) => m.from && m.to && m.from.index === state.selected && m.to.index === index);
      if (move) await applyHuman(move);
      return;
    }
    if (cell && pieceSide(cell) === state.humanSide) {
      state.selected = index;
      renderCheckers();
      renderProgress();
      return;
    }
    state.selected = null;
    renderCheckers();
    renderProgress();
  }

  function pushSnapshot() {
    state.undoStack.push({
      position: clone(state.position),
      history: state.history.slice(),
      engine: state.engine,
    });
    if (state.undoStack.length > 40) state.undoStack.shift();
  }

  async function applyHuman(move) {
    if (state.busy) return;
    state.busy = true;
    state.selected = null;
    pushSnapshot();
    renderStatus();
    try {
      const data = await api("api/v1/apply-move", {
        game: state.meta.id,
        position: positionPayload(state.position),
        move: moveInput(move),
      });
      const anim = captureMoveAnim(data.move, data.position.board);
      state.position = data.position;
      state.legal = data.position.legalMoves || [];
      state.lastMove = data.move;
      state.history.push({ by: "human", move: data.move });
      state.busy = false;
      saveRecord();
      render();
      await playMoveAnimation(anim);
      if (!isFinished(data.position.status)) await aiTurn();
    } catch (err) {
      state.undoStack.pop();
      state.busy = false;
      showToast(err.message);
      render();
    }
  }

  async function aiTurn() {
    if (state.busy || !state.position || isFinished(state.position.status)) return;
    if (state.position.turn === state.humanSide) return;
    state.busy = true;
    state.selected = null;
    state.aiDepth = 0;
    render();
    try {
      const result = await requestAIMove();
      state.engine = result.engine || state.engine;
      if (!result.move) {
        state.position = { ...state.position, status: result.status || state.position.status };
        state.busy = false;
        saveRecord();
        render();
        return;
      }
      pushSnapshot();
      const data = await api("api/v1/apply-move", {
        game: state.meta.id,
        position: positionPayload(state.position),
        move: moveInput(result.move),
      });
      const anim = captureMoveAnim(data.move, data.position.board);
      state.position = data.position;
      state.legal = data.position.legalMoves || [];
      state.lastMove = data.move;
      state.history.push({ by: "ai", move: data.move });
      state.busy = false;
      state.aiDepth = 0;
      saveRecord();
      render();
      await playMoveAnimation(anim);
      if (isFinished(data.position.status)) {
        showToast(statusText(data.position.status));
      }
    } catch (err) {
      state.busy = false;
      state.aiDepth = 0;
      showToast(T("AI 走子失败：", "AI move failed: ") + err.message);
      render();
    }
  }

  async function requestAIMove() {
    const body = {
      game: state.meta.id,
      position: positionPayload(state.position),
      difficulty: state.difficulty,
      timeLimitMs: TIME_BY_DIFF[state.difficulty] || 1000,
    };
    if (el.engine && el.engine.value) body.engine = el.engine.value;
    if (state.apiMode === "normal") return aiMoveNormal(body);
    return aiMoveStream(body);
  }

  async function aiMoveNormal(body) {
    setAIProgress(T("AI 思考中", "AI is thinking"), T("普通接口请求中…", "Normal request in flight..."), true);
    return api("api/v1/move", body);
  }

  async function aiMoveStream(body) {
    setAIProgress(T("AI 思考中", "AI is thinking"), T("连接 SSE…", "Connecting to SSE..."), true);
    const res = await fetch(BASE + "api/v1/move/stream", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok || !res.body) throw new Error(T("SSE 连接失败", "SSE connection failed"));
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let result = null;
    let failure = null;

    const handle = (frame) => {
      let event = "";
      const dataLines = [];
      for (const line of frame.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) dataLines.push(line.slice(5).trimStart());
      }
      if (!event || !dataLines.length) return;
      let payload;
      try {
        payload = JSON.parse(dataLines.join("\n"));
      } catch (_) {
        return;
      }
      if (event === "start") {
        state.engine = payload.engine || state.engine;
        setAIProgress(T(`AI 思考中 · ${state.engine}`, `AI thinking · ${state.engine}`), T("迭代加深…", "Iterative deepening..."), true);
      } else if (event === "depth") {
        state.aiDepth = payload.depth || 0;
        const pct = payload.depth ? Math.min(92, 16 + payload.depth * 6) : 0;
        setAIProgress(
          T(`深度 ${payload.depth || "—"}`, `Depth ${payload.depth || "—"}`),
          T(`${payload.nodes || 0} 节点 · ${payload.timeMs || 0} ms`, `${payload.nodes || 0} nodes · ${payload.timeMs || 0} ms`),
          false,
          pct
        );
      } else if (event === "result") {
        result = payload;
      } else if (event === "error") {
        failure = new Error((payload.error && payload.error.message) || T("AI 思考失败", "AI thinking failed"));
      }
    };

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
      let cut;
      while ((cut = buffer.indexOf("\n\n")) >= 0) {
        const frame = buffer.slice(0, cut);
        buffer = buffer.slice(cut + 2);
        handle(frame);
      }
      if (result) {
        await reader.cancel().catch(() => {});
        break;
      }
    }
    if (failure) throw failure;
    if (!result) throw new Error(T("SSE 没有返回结果", "SSE returned no result"));
    return result;
  }

  function setAIProgress(label, meta, indeterminate, pct) {
    if (!el.aiLabel) return;
    el.aiLabel.textContent = label;
    el.aiMeta.textContent = meta;
    if (indeterminate) {
      el.aiBar.classList.add("indeterminate");
      el.aiBar.style.width = "42%";
    } else {
      el.aiBar.classList.remove("indeterminate");
      el.aiBar.style.width = `${Math.max(4, Math.min(100, pct || 0))}%`;
    }
  }

  function undo() {
    if (state.busy || state.animating) return;
    if (!state.undoStack.length) return;
    let snap = state.undoStack.pop();
    state.position = snap.position;
    state.history = snap.history;
    state.engine = snap.engine || state.engine;
    while (state.undoStack.length && state.position.turn !== state.humanSide) {
      snap = state.undoStack.pop();
      state.position = snap.position;
      state.history = snap.history;
      state.engine = snap.engine || state.engine;
    }
    state.legal = state.position.legalMoves || [];
    state.selected = null;
    state.lastMove = state.history.length ? state.history[state.history.length - 1].move : null;
    state.aiDepth = 0;
    saveRecord();
    render();
    showToast(T("已悔棋", "Move undone"));
  }

  function populateGameSelect() {
    if (!el.game) return;
    el.game.innerHTML = state.games
      .map((g) => {
        const label = isEN() ? (g.englishName || g.name) : `${g.name} · ${g.englishName}`;
        return `<option value="${escapeHTML(g.id)}">${escapeHTML(label)}</option>`;
      })
      .join("");
  }

  function bindEvents() {
    el.game?.addEventListener("change", () => {
      const meta = state.games.find((g) => g.id === el.game.value);
      if (!meta) return;
      sideSelectOptions(meta, meta.initialPosition.turn);
      newGame({ gameId: meta.id, humanSide: meta.initialPosition.turn });
    });
    el.side?.addEventListener("change", () => {
      newGame({ gameId: state.meta.id, humanSide: el.side.value });
    });
    el.difficulty?.addEventListener("change", () => {
      state.difficulty = el.difficulty.value;
      saveRecord();
      renderInfo();
    });
    el.apiMode?.addEventListener("change", () => {
      state.apiMode = el.apiMode.value === "normal" ? "normal" : "sse";
      try {
        localStorage.setItem(API_MODE_KEY, state.apiMode);
      } catch (_) {}
    });
    el.newGame?.addEventListener("click", () => newGame());
    el.undo?.addEventListener("click", undo);
    el.clearRecords?.addEventListener("click", () => {
      if (!confirm(T("清空本机所有对局记录？", "Clear all locally saved games?"))) return;
      writeRecords([]);
      state.recordId = "";
      syncPlayURL();
      renderInfo();
      renderRecords();
      showToast(T("已清空本机对局记录", "Local game records cleared"));
    });
  }

  function loadGames() {
    const node = document.getElementById("games-data");
    if (!node) return;
    try {
      const games = JSON.parse(node.textContent);
      if (Array.isArray(games)) state.games = games;
    } catch (_) {
      state.games = [];
    }
  }

  function init() {
    loadGames();
    if (!state.games.length) {
      showToast(T("读取棋种目录失败", "Failed to load the game catalog"));
      return;
    }
    populateGameSelect();
    bindEvents();

    const params = new URLSearchParams(location.search);
    try {
      const storedMode = localStorage.getItem(API_MODE_KEY);
      if (storedMode === "normal" || storedMode === "sse") state.apiMode = storedMode;
    } catch (_) {}
    const paramMode = params.get("api");
    if (paramMode === "normal" || paramMode === "sse") state.apiMode = paramMode;
    if (el.apiMode) el.apiMode.value = state.apiMode;
    const wantedGame = params.get("game");
    const meta = state.games.find((g) => g.id === wantedGame)
      || state.games.find((g) => g.id === "xiangqi")
      || state.games[0];
    el.game.value = meta.id;
    sideSelectOptions(meta, meta.initialPosition.turn);

    const recordId = params.get("record");
    if (recordId) {
      const record = loadRecords().find((r) => r.id === recordId);
      if (record) {
        resumeRecord(record);
        return;
      }
    }
    newGame({ gameId: meta.id, humanSide: meta.initialPosition.turn });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
