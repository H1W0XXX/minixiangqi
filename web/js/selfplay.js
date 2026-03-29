let selfplayRunning = false;
let selfplayPaused = false;
let selfplayMoveCount = 0;
let selfplayModels = [];

const logEl = document.getElementById("log");

function selfplayModelSelect(prefix) {
  return document.getElementById(`${prefix}ModelSelect`);
}

function currentSelfplayModelKey(prefix) {
  return selfplayModelSelect(prefix)?.value || "";
}

function currentSelfplayBackend(prefix) {
  const key = currentSelfplayModelKey(prefix);
  const model = selfplayModels.find((item) => item.key === key);
  if (model?.backend) {
    return model.backend;
  }
  if (key.startsWith("nnue|")) {
    return "nnue";
  }
  if (key.startsWith("onnx|")) {
    return "onnx";
  }
  return "none";
}

function addSelfplayLog(message) {
  if (!logEl) {
    return;
  }
  if (logEl.innerText === "等待开始...") {
    logEl.innerText = "";
  }
  const line = document.createElement("div");
  line.style.borderBottom = "1px solid rgba(255,255,255,0.08)";
  line.style.padding = "4px 0";
  line.innerText = `[${new Date().toLocaleTimeString()}] ${message}`;
  logEl.prepend(line);
}

async function loadSelfplayModels() {
  const data = await postJSON("/api/models", {});
  const list = data.models || [];
  selfplayModels = list.filter((model) => model.key !== "none");
  ["red", "black"].forEach((prefix) => {
    const select = selfplayModelSelect(prefix);
    if (!select) {
      return;
    }
    const saved = localStorage.getItem(`minixiangqi-selfplay-${prefix}-model-key`);
    select.innerHTML = "";
    selfplayModels.forEach((model) => {
      const option = document.createElement("option");
      option.value = model.key;
      option.textContent = model.label;
      option.dataset.backend = model.backend || "";
      select.appendChild(option);
    });
    const fallback = selfplayModels[0]?.key || data.current_key || "none";
    const chosen = selfplayModels.some((model) => model.key === saved) ? saved : fallback;
    if (chosen) {
      select.value = chosen;
    }
  });
  refreshSelfplayConfigUI();
}

function syncSlider(prefix) {
  const select = document.getElementById(`${prefix}Algo`);
  const range = document.getElementById(`${prefix}Val`);
  const label = document.getElementById(`${prefix}ValLabel`);
  const title = document.getElementById(`${prefix}RangeTitle`);
  if (!select || !range || !label || !title) {
    return;
  }
  const update = () => {
    const backend = currentSelfplayBackend(prefix);
    if (backend === "nnue") {
      select.value = "fast";
      select.disabled = true;
      range.min = "1";
      range.max = "6";
      range.step = "1";
      title.innerText = "Alpha-Beta 搜索深度";
      if (Number(range.value) < 1 || Number(range.value) > 6) {
        range.value = "3";
      }
    } else {
      select.disabled = selfplayRunning;
      title.innerText = "模拟次数 / 搜索强度";
      range.min = "40";
      range.max = "2000";
      range.step = "40";
      if (Number(range.value) < 40) {
        range.value = "320";
      }
    }
    label.innerText = range.value;
  };
  if (!range.dataset.bound) {
    range.addEventListener("input", update);
    range.dataset.bound = "1";
  }
  if (!select.dataset.bound) {
    select.addEventListener("change", update);
    select.dataset.bound = "1";
  }
  update();
}

function refreshSelfplayConfigUI() {
  syncSlider("red");
  syncSlider("black");
}

function isSelfplayGameOver() {
  return currentStatus !== "ongoing";
}

function refreshSelfplayMeta() {
  const moveCountEl = document.getElementById("moveCount");
  if (moveCountEl) {
    moveCountEl.innerText = String(selfplayMoveCount);
  }
}

function updateSelfplayControls() {
  const boardSvg = document.getElementById("boardSvg");
  document.getElementById("btnStart").disabled = selfplayRunning;
  document.getElementById("btnPause").disabled = !selfplayRunning;
  document.getElementById("btnStop").disabled = !selfplayRunning;
  document.getElementById("btnPause").innerText = selfplayPaused ? "继续" : "暂停";
  ["redModelSelect", "redAlgo", "redVal", "blackModelSelect", "blackAlgo", "blackVal", "stepDelay"].forEach((id) => {
    const el = document.getElementById(id);
    if (el) {
      el.disabled = selfplayRunning;
    }
  });
  if (boardSvg) {
    boardSvg.style.pointerEvents = selfplayRunning ? "none" : "auto";
  }
  refreshSelfplayConfigUI();
}

async function selfplayMoveRequest(modelKey, backend, useMCTS, value) {
  return postJSON("/api/ai_move", {
    game_id: gameId,
    model_key: modelKey,
    position: sideToMove === 0 ? `${encodeBoard()} w` : `${encodeBoard()} b`,
    to_move: sideToMove,
    time_ms: backend === "nnue" ? 5000 : (useMCTS ? 5000 : 800),
    use_mcts: backend === "nnue" ? false : useMCTS,
    mcts_simulations: value,
  });
}

async function startSelfplay() {
  if (selfplayRunning) {
    return;
  }
  selectedModelKey = currentSelfplayModelKey("red");
  addSelfplayLog("初始化新对局");
  await newGame();
  selfplayMoveCount = 0;
  selfplayRunning = true;
  selfplayPaused = false;
  refreshSelfplayMeta();
  updateSelfplayControls();
  addSelfplayLog("自对弈开始");
  runSelfplayLoop();
}

async function runSelfplayLoop() {
  while (selfplayRunning) {
    if (selfplayPaused) {
      await new Promise((resolve) => setTimeout(resolve, 250));
      continue;
    }

    if (isSelfplayGameOver()) {
      addSelfplayLog(`对局结束: ${currentStatus}`);
      selfplayRunning = false;
      updateSelfplayControls();
      break;
    }

    const prefix = sideToMove === 0 ? "red" : "black";
    const sideName = sideToMove === 0 ? "红方" : "黑方";
    const algo = document.getElementById(`${prefix}Algo`).value;
    const value = Number(document.getElementById(`${prefix}Val`).value);
    const delay = Math.max(0, Number(document.getElementById("stepDelay").value) || 0);
    const modelKey = currentSelfplayModelKey(prefix);
    const modelLabel = selfplayModelSelect(prefix)?.selectedOptions?.[0]?.textContent || modelKey;
    const backend = currentSelfplayBackend(prefix);
    const useMCTS = backend === "nnue" ? false : algo === "mcts";

    addSelfplayLog(`${sideName} 思考中: ${modelLabel} / ${backend === "nnue" ? `Alpha-Beta 深度 ${value}` : (useMCTS ? `MCTS ${value}` : `快速搜索 ${value}`)}`);

    try {
      const data = await selfplayMoveRequest(modelKey, backend, useMCTS, value);
      if (!selfplayRunning) {
        break;
      }
      document.getElementById("searchText").innerText = `${data.nodes} / ${data.time_ms}ms`;
      document.getElementById("winProbText").innerText = `${(data.win_prob * 100).toFixed(1)}%`;
      if (data.best_move && data.best_move.from >= 0 && data.best_move.to >= 0) {
        addSelfplayLog(`${sideName} 落子: ${data.best_move.from} -> ${data.best_move.to}`);
        await playMove(data.best_move);
        selfplayMoveCount += 1;
        refreshSelfplayMeta();
      } else {
        addSelfplayLog(`${sideName} 无合法着法`);
        selfplayRunning = false;
        updateSelfplayControls();
        break;
      }
      if (delay > 0) {
        await new Promise((resolve) => setTimeout(resolve, delay));
      }
    } catch (err) {
      addSelfplayLog(`异常中止: ${err.message}`);
      selfplayRunning = false;
      selfplayPaused = false;
      updateSelfplayControls();
      break;
    }
  }
}

function pauseSelfplay() {
  if (!selfplayRunning) {
    return;
  }
  selfplayPaused = !selfplayPaused;
  addSelfplayLog(selfplayPaused ? "已暂停" : "已继续");
  updateSelfplayControls();
}

function stopSelfplay() {
  if (!selfplayRunning) {
    return;
  }
  selfplayRunning = false;
  selfplayPaused = false;
  addSelfplayLog("已强制终止");
  updateSelfplayControls();
}

document.addEventListener("DOMContentLoaded", () => {
  loadSelfplayModels().catch((err) => addSelfplayLog(`模型加载失败: ${err.message}`));
  document.getElementById("btnStart").addEventListener("click", () => {
    startSelfplay().catch((err) => addSelfplayLog(`启动失败: ${err.message}`));
  });
  document.getElementById("btnPause").addEventListener("click", pauseSelfplay);
  document.getElementById("btnStop").addEventListener("click", stopSelfplay);
  ["redModelSelect", "blackModelSelect"].forEach((id) => {
    document.getElementById(id)?.addEventListener("change", (evt) => {
      const prefix = id.startsWith("red") ? "red" : "black";
      localStorage.setItem(`minixiangqi-selfplay-${prefix}-model-key`, evt.target.value);
      refreshSelfplayConfigUI();
    });
  });
  refreshSelfplayMeta();
  updateSelfplayControls();
});
