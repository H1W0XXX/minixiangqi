const ROWS = 7;
const COLS = 7;
const BOARD_ORIGIN = 84;
const STEP = 92;
const PIECE_SIZE = 76;
const MOVE_ANIM_MS = 220;

let gameId = "";
let board = new Array(ROWS * COLS).fill(null);
let sideToMove = 0;
let legalMoves = [];
let selectedSq = null;
let lastMove = null;
let currentStatus = "ongoing";
let selectedModelKey = "";
let isAnimating = false;

function rowColToSquare(row, col) {
  return row * COLS + col;
}

function squareToRowCol(square) {
  return { row: Math.floor(square / COLS), col: square % COLS };
}

function squareCenter(square) {
  const { row, col } = squareToRowCol(square);
  return {
    x: BOARD_ORIGIN + col * STEP,
    y: BOARD_ORIGIN + row * STEP,
  };
}

function spriteForChar(ch) {
  const map = {
    C: "r-chariot.svg",
    P: "r-cannon.svg",
    M: "r-horse.svg",
    W: "r-king.svg",
    B: "r-pawn.svg",
    c: "b-chariot.svg",
    p: "b-cannon.svg",
    m: "b-horse.svg",
    w: "b-king.svg",
    b: "b-pawn.svg",
  };
  return map[ch] ? `svg/pieces/${map[ch]}` : "";
}

function parseFEN(fen) {
  const [boardPart, stm] = fen.trim().split(" ");
  const rows = boardPart.split("/");
  board = new Array(ROWS * COLS).fill(null);
  rows.forEach((rowText, row) => {
    let col = 0;
    for (const ch of rowText) {
      if (/\d/.test(ch)) {
        col += Number(ch);
      } else {
        board[rowColToSquare(row, col)] = ch;
        col += 1;
      }
    }
  });
  sideToMove = stm === "b" ? 1 : 0;
}

function buildBoard() {
  const layer = document.getElementById("boardLayer");
  layer.innerHTML = "";

  const bg = document.createElementNS("http://www.w3.org/2000/svg", "rect");
  bg.setAttribute("x", 24);
  bg.setAttribute("y", 24);
  bg.setAttribute("width", 672);
  bg.setAttribute("height", 732);
  bg.setAttribute("rx", 28);
  bg.setAttribute("fill", "#e5bf86");
  bg.setAttribute("stroke", "#6f4f31");
  bg.setAttribute("stroke-width", "3");
  layer.appendChild(bg);

  for (let i = 0; i < ROWS; i++) {
    const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
    line.setAttribute("x1", BOARD_ORIGIN);
    line.setAttribute("y1", BOARD_ORIGIN + i * STEP);
    line.setAttribute("x2", BOARD_ORIGIN + (COLS - 1) * STEP);
    line.setAttribute("y2", BOARD_ORIGIN + i * STEP);
    line.setAttribute("stroke", "#35261d");
    line.setAttribute("stroke-width", "4");
    layer.appendChild(line);
  }

  for (let i = 0; i < COLS; i++) {
    const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
    line.setAttribute("x1", BOARD_ORIGIN + i * STEP);
    line.setAttribute("y1", BOARD_ORIGIN);
    line.setAttribute("x2", BOARD_ORIGIN + i * STEP);
    line.setAttribute("y2", BOARD_ORIGIN + (ROWS - 1) * STEP);
    line.setAttribute("stroke", "#35261d");
    line.setAttribute("stroke-width", "4");
    layer.appendChild(line);
  }

  addPalace(layer, 0);
  addPalace(layer, 4);

  for (let sq = 0; sq < ROWS * COLS; sq++) {
    const { x, y } = squareCenter(sq);
    const dot = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    dot.setAttribute("cx", x);
    dot.setAttribute("cy", y);
    dot.setAttribute("r", "4.5");
    dot.setAttribute("fill", "#35261d");
    layer.appendChild(dot);
  }
}

function addPalace(layer, startRow) {
  const lines = [
    [2, startRow, 4, startRow + 2],
    [4, startRow, 2, startRow + 2],
  ];
  lines.forEach(([c1, r1, c2, r2]) => {
    const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
    line.setAttribute("x1", BOARD_ORIGIN + c1 * STEP);
    line.setAttribute("y1", BOARD_ORIGIN + r1 * STEP);
    line.setAttribute("x2", BOARD_ORIGIN + c2 * STEP);
    line.setAttribute("y2", BOARD_ORIGIN + r2 * STEP);
    line.setAttribute("stroke", "#35261d");
    line.setAttribute("stroke-width", "4");
    layer.appendChild(line);
  });
}

function render(boardState = board) {
  renderHighlights();
  renderPieces(boardState);
  updateMeta();
}

function renderPieces(boardState = board, hiddenSquares = new Set()) {
  const layer = document.getElementById("piecesLayer");
  layer.innerHTML = "";
  for (let sq = 0; sq < boardState.length; sq++) {
    const ch = boardState[sq];
    if (!ch || hiddenSquares.has(sq)) {
      continue;
    }
    const { x, y } = squareCenter(sq);
    const image = document.createElementNS("http://www.w3.org/2000/svg", "image");
    image.setAttribute("href", spriteForChar(ch));
    image.setAttribute("x", x - PIECE_SIZE / 2);
    image.setAttribute("y", y - PIECE_SIZE / 2);
    image.setAttribute("width", PIECE_SIZE);
    image.setAttribute("height", PIECE_SIZE);
    image.setAttribute("filter", "url(#pieceShadow)");
    layer.appendChild(image);
  }
}

function clearAnimationLayer() {
  const layer = document.getElementById("animLayer");
  if (layer) {
    layer.innerHTML = "";
  }
}

function motionProgress(t) {
  const accelRatio = 0.32;
  const vmax = 1 / (1 - 0.5 * accelRatio);
  if (t <= 0) {
    return 0;
  }
  if (t >= 1) {
    return 1;
  }
  if (t < accelRatio) {
    return 0.5 * vmax * t * t / accelRatio;
  }
  return Math.min(1, vmax * (t - 0.5 * accelRatio));
}

function animateMoveSprite(pieceChar, fromSquare, toSquare) {
  const layer = document.getElementById("animLayer");
  if (!layer || !pieceChar) {
    return Promise.resolve();
  }

  clearAnimationLayer();
  const sprite = document.createElementNS("http://www.w3.org/2000/svg", "image");
  sprite.setAttribute("href", spriteForChar(pieceChar));
  sprite.setAttribute("width", PIECE_SIZE);
  sprite.setAttribute("height", PIECE_SIZE);
  sprite.setAttribute("filter", "url(#pieceShadow)");
  layer.appendChild(sprite);

  const start = squareCenter(fromSquare);
  const end = squareCenter(toSquare);
  const startedAt = performance.now();
  isAnimating = true;

  return new Promise((resolve) => {
    const tick = (now) => {
      const raw = Math.min(1, (now - startedAt) / MOVE_ANIM_MS);
      const p = motionProgress(raw);
      const x = start.x + (end.x - start.x) * p;
      const y = start.y + (end.y - start.y) * p;
      sprite.setAttribute("x", x - PIECE_SIZE / 2);
      sprite.setAttribute("y", y - PIECE_SIZE / 2);
      if (raw < 1) {
        requestAnimationFrame(tick);
        return;
      }
      clearAnimationLayer();
      isAnimating = false;
      resolve();
    };
    requestAnimationFrame(tick);
  });
}

function renderHighlights() {
  const layer = document.getElementById("highlightLayer");
  layer.innerHTML = "";

  if (lastMove) {
    [lastMove.from, lastMove.to].forEach((sq, index) => {
      const { x, y } = squareCenter(sq);
      const circle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      circle.setAttribute("cx", x);
      circle.setAttribute("cy", y);
      circle.setAttribute("r", index === 0 ? 24 : 36);
      circle.setAttribute("fill", "none");
      circle.setAttribute("stroke", "#135e95");
      circle.setAttribute("stroke-width", "5");
      circle.setAttribute("opacity", index === 0 ? "0.55" : "0.85");
      layer.appendChild(circle);
    });
  }

  if (selectedSq !== null) {
    const { x, y } = squareCenter(selectedSq);
    const circle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    circle.setAttribute("cx", x);
    circle.setAttribute("cy", y);
    circle.setAttribute("r", "34");
    circle.setAttribute("fill", "none");
    circle.setAttribute("stroke", "#f3b500");
    circle.setAttribute("stroke-width", "6");
    layer.appendChild(circle);
  }

  legalMoves
    .filter((mv) => selectedSq !== null && mv.from === selectedSq)
    .forEach((mv) => {
      const { x, y } = squareCenter(mv.to);
      if (board[mv.to]) {
        const outer = document.createElementNS("http://www.w3.org/2000/svg", "circle");
        outer.setAttribute("cx", x);
        outer.setAttribute("cy", y);
        outer.setAttribute("r", "38");
        outer.setAttribute("fill", "none");
        outer.setAttribute("stroke", "#d92b1f");
        outer.setAttribute("stroke-width", "6");
        outer.setAttribute("opacity", "0.92");
        layer.appendChild(outer);

        const inner = document.createElementNS("http://www.w3.org/2000/svg", "circle");
        inner.setAttribute("cx", x);
        inner.setAttribute("cy", y);
        inner.setAttribute("r", "28");
        inner.setAttribute("fill", "none");
        inner.setAttribute("stroke", "#fff0d9");
        inner.setAttribute("stroke-width", "3");
        inner.setAttribute("opacity", "0.9");
        layer.appendChild(inner);
      } else {
        const marker = document.createElementNS("http://www.w3.org/2000/svg", "circle");
        marker.setAttribute("cx", x);
        marker.setAttribute("cy", y);
        marker.setAttribute("r", "14");
        marker.setAttribute("fill", "#0b7d74");
        marker.setAttribute("opacity", "0.78");
        layer.appendChild(marker);
      }
    });
}

function updateMeta() {
  const statusMap = {
    ongoing: "进行中",
    red_win: "红方胜",
    black_win: "黑方胜",
    draw: "和棋",
  };
  document.getElementById("statusText").innerText = statusMap[currentStatus] || currentStatus;
  document.getElementById("turnText").innerText = sideToMove === 0 ? "红方（下方）" : "黑方（上方）";
  document.getElementById("btnAi").disabled = currentStatus !== "ongoing";
}

function squareFromEvent(evt) {
  const svg = document.getElementById("boardSvg");
  const point = svg.createSVGPoint();
  point.x = evt.clientX;
  point.y = evt.clientY;
  const p = point.matrixTransform(svg.getScreenCTM().inverse());
  const col = Math.round((p.x - BOARD_ORIGIN) / STEP);
  const row = Math.round((p.y - BOARD_ORIGIN) / STEP);
  if (row < 0 || row >= ROWS || col < 0 || col >= COLS) {
    return null;
  }
  return rowColToSquare(row, col);
}

function ownPieceAt(square) {
  const ch = board[square];
  if (!ch) {
    return false;
  }
  return sideToMove === 0 ? ch === ch.toUpperCase() : ch === ch.toLowerCase();
}

async function postJSON(url, payload) {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = (await res.text()).trim();
    const known = {
      "bad json": "请求格式不正确",
      "game not found": "当前对局不存在，已失效",
      "illegal move": "这步棋不合法",
      "cannot undo": "当前局面不能悔棋",
      "model not found": "所选模型不存在，请重新选择",
      "nn failed": "AI 推理失败",
    };
    throw new Error(known[text] || text || `${url} failed`);
  }
  return res.json();
}

async function syncState(data, options = {}) {
  const prevBoard = board.slice();
  parseFEN(data.position);
  legalMoves = data.legal_moves || [];
  currentStatus = data.status || "ongoing";
  if (data.model_key) {
    selectedModelKey = data.model_key;
    const select = document.getElementById("modelSelect");
    if (select) {
      select.value = data.model_key;
      localStorage.setItem("minixiangqi-model-key", data.model_key);
    }
    updateModelHint(data.model_label || "");
    refreshSearchControl();
  }
  selectedSq = null;
  const animatedMove = options.animateMove;
  const movingPiece = animatedMove ? prevBoard[animatedMove.from] : null;
  if (animatedMove && movingPiece) {
    const tempBoard = prevBoard.slice();
    tempBoard[animatedMove.from] = null;
    render(tempBoard);
    await animateMoveSprite(movingPiece, animatedMove.from, animatedMove.to);
  }
  render();
}

function updateModelHint(label) {
  const hint = document.getElementById("modelHint");
  if (!hint) {
    return;
  }
  hint.innerText = label ? `当前模型：${label}。切换模型后，重新开始生效。` : "切换模型后，重新开始生效。";
}

function getSelectedModelBackend() {
  const select = document.getElementById("modelSelect");
  if (!select) {
    return "none";
  }
  const option = select.options[select.selectedIndex];
  return option?.dataset?.backend || (select.value.startsWith("nnue|") ? "nnue" : select.value.startsWith("onnx|") ? "onnx" : "none");
}

function refreshSearchControl() {
  const label = document.getElementById("simLabel");
  const range = document.getElementById("simRange");
  const value = document.getElementById("simText");
  if (!label || !range || !value) {
    return;
  }
  range.dataset.mctsMin ||= range.min;
  range.dataset.mctsMax ||= range.max;
  range.dataset.mctsStep ||= range.step;
  range.dataset.mctsDefault ||= range.value;

  if (getSelectedModelBackend() === "nnue") {
    label.innerText = "Alpha-Beta 搜索深度";
    range.min = "1";
    range.max = "6";
    range.step = "1";
    if (Number(range.value) < 1 || Number(range.value) > 6) {
      range.value = "3";
    }
  } else {
    label.innerText = "MCTS 模拟次数";
    range.min = range.dataset.mctsMin;
    range.max = range.dataset.mctsMax;
    range.step = range.dataset.mctsStep;
    if (Number(range.value) < Number(range.min)) {
      range.value = range.dataset.mctsDefault;
    }
  }
  value.innerText = range.value;
}

async function loadModels() {
  const data = await postJSON("/api/models", {});
  const select = document.getElementById("modelSelect");
  const saved = localStorage.getItem("minixiangqi-model-key");
  const list = data.models || [];
  const savedExists = list.some((model) => model.key === saved);
  selectedModelKey = savedExists ? saved : (data.current_key || (list[0] ? list[0].key : ""));
  if (!select) {
    return;
  }
  select.innerHTML = "";
  list.forEach((model) => {
    const option = document.createElement("option");
    option.value = model.key;
    option.textContent = model.label;
    option.dataset.backend = model.backend || "";
    select.appendChild(option);
  });
  if (selectedModelKey) {
    select.value = selectedModelKey;
  }
  const current = list.find((model) => model.key === selectedModelKey);
  updateModelHint(current ? current.label : "");
  refreshSearchControl();
}

async function newGame() {
  if (isAnimating) {
    return;
  }
  const data = await postJSON("/api/new_game", { model_key: selectedModelKey });
  gameId = data.game_id;
  localStorage.setItem("minixiangqi-game-id", gameId);
  if (data.model_key) {
    selectedModelKey = data.model_key;
    localStorage.setItem("minixiangqi-model-key", data.model_key);
  }
  lastMove = null;
  await syncState(data);
  document.getElementById("searchText").innerText = "- / -";
  document.getElementById("winProbText").innerText = "50.0%";
}

async function loadState() {
  const saved = localStorage.getItem("minixiangqi-game-id");
  if (!saved) {
    await newGame();
    return;
  }
  try {
    const data = await postJSON("/api/state", { game_id: saved });
    gameId = saved;
    await syncState(data);
  } catch (_) {
    await newGame();
  }
}

async function playMove(move) {
  const data = await postJSON("/api/play", { game_id: gameId, move });
  lastMove = move;
  await syncState(data, { animateMove: move });
}

async function requestAIMove() {
  if (isAnimating) {
    return;
  }
  const button = document.getElementById("btnAi");
  button.disabled = true;
  button.innerText = "思考中...";
  try {
    const searchValue = Number(document.getElementById("simRange").value);
    const timeMs = Number(document.getElementById("timeRange").value) * 1000;
    const backend = getSelectedModelBackend();
    const data = await postJSON("/api/ai_move", {
      game_id: gameId,
      position: sideToMove === 0 ? `${encodeBoard()} w` : `${encodeBoard()} b`,
      to_move: sideToMove,
      time_ms: timeMs,
      use_mcts: backend === "nnue" ? false : true,
      mcts_simulations: searchValue,
    });
    document.getElementById("searchText").innerText = `${data.nodes} / ${data.time_ms}ms`;
    document.getElementById("winProbText").innerText = `${(data.win_prob * 100).toFixed(1)}%`;
    if (data.best_move && data.best_move.from >= 0 && data.best_move.to >= 0) {
      await playMove(data.best_move);
    }
  } finally {
    button.disabled = false;
    button.innerText = "AI 落子";
  }
}

function encodeBoard() {
  const rows = [];
  for (let row = 0; row < ROWS; row++) {
    let text = "";
    let empty = 0;
    for (let col = 0; col < COLS; col++) {
      const ch = board[rowColToSquare(row, col)];
      if (!ch) {
        empty += 1;
      } else {
        if (empty > 0) {
          text += String(empty);
          empty = 0;
        }
        text += ch;
      }
    }
    if (empty > 0) {
      text += String(empty);
    }
    rows.push(text);
  }
  return rows.join("/");
}

async function undo() {
  if (isAnimating) {
    return;
  }
  const data = await postJSON("/api/undo", { game_id: gameId });
  lastMove = null;
  await syncState(data);
}

function onBoardClick(evt) {
  if (currentStatus !== "ongoing" || isAnimating) {
    return;
  }
  const square = squareFromEvent(evt);
  if (square === null) {
    return;
  }
  if (selectedSq !== null) {
    const targetMove = legalMoves.find((mv) => mv.from === selectedSq && mv.to === square);
    if (targetMove) {
      playMove(targetMove).catch((err) => alert(err.message));
      return;
    }
  }
  if (ownPieceAt(square)) {
    selectedSq = square;
  } else {
    selectedSq = null;
  }
  renderHighlights();
}

document.addEventListener("DOMContentLoaded", () => {
  buildBoard();
  document.getElementById("boardSvg")?.addEventListener("click", onBoardClick);
  document.getElementById("btnNew")?.addEventListener("click", () => newGame().catch((err) => alert(err.message)));
  document.getElementById("btnUndo")?.addEventListener("click", () => undo().catch((err) => alert(err.message)));
  document.getElementById("btnAi")?.addEventListener("click", () => requestAIMove().catch((err) => alert(err.message)));
  document.getElementById("modelSelect")?.addEventListener("change", (evt) => {
    selectedModelKey = evt.target.value;
    localStorage.setItem("minixiangqi-model-key", selectedModelKey);
    const label = evt.target.options[evt.target.selectedIndex]?.textContent || "";
    updateModelHint(label);
    refreshSearchControl();
  });
  document.getElementById("simRange")?.addEventListener("input", (evt) => {
    const simText = document.getElementById("simText");
    if (simText) {
      simText.innerText = evt.target.value;
    }
  });
  document.getElementById("timeRange")?.addEventListener("input", (evt) => {
    const timeText = document.getElementById("timeText");
    if (timeText) {
      timeText.innerText = `${evt.target.value} 秒`;
    }
  });
  loadModels()
    .then(() => loadState())
    .catch((err) => alert(err.message));
});
