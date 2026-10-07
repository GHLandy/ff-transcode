import "./style.css";
import "./app.css";

import * as api from "./api";
import type {
  CommandPlan,
  MediaInfo,
  OptionItem,
  TranscodeOptions,
  TranscodePresets,
  TranscodeProgress,
  TranscodeSnapshot,
  ToolStatus,
} from "./types";

const DEFAULT_CRF = 23;
/** 与 Go 侧默认档位保持一致，取不到预设时作为兜底。 */
const DEFAULT_PROFILE = "main";
const DEFAULT_LEVEL = "4.0";
const AUTO_LEVEL = "auto";
const LOG_LIMIT = 300;

/** 兜底 CRF 档位；正常情况下会由 Go 侧 GetPresets 覆盖。 */
const FALLBACK_CRFS: OptionItem[] = [
  { value: "18", label: "18", hint: "高画质 · 体积最大" },
  { value: "23", label: "23", hint: "默认 · 推荐" },
  { value: "28", label: "28", hint: "小体积 · 画质下降" },
];

interface State {
  tools: ToolStatus | null;
  info: MediaInfo | null;
  presets: TranscodePresets | null;
  sourcePath: string;
  crf: number;
  audioAac: boolean;
  profile: string;
  level: string;
  outputDir: string;
  plan: CommandPlan | null;
  running: boolean;
  logs: string[];
}

const state: State = {
  tools: null,
  info: null,
  presets: null,
  sourcePath: "",
  crf: DEFAULT_CRF,
  audioAac: false,
  profile: DEFAULT_PROFILE,
  level: DEFAULT_LEVEL,
  outputDir: "",
  plan: null,
  running: false,
  logs: [],
};

// ---------- DOM 辅助 ----------

function el<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (!node) {
    throw new Error(`界面元素 #${id} 不存在`);
  }
  return node as T;
}

function esc(text: string): string {
  return String(text)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function errorText(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

// ---------- 页面骨架 ----------

function crfButtons(items: OptionItem[], active: number): string {
  return items
    .map(
      (item) => `
        <button class="crf-opt${Number(item.value) === active ? " is-active" : ""}" data-crf="${esc(item.value)}">
            <b>${esc(item.label)}</b>
            <span>${esc(item.hint)}</span>
        </button>`,
    )
    .join("");
}

document.getElementById("app")!.innerHTML = `
<div class="shell">
    <header class="hero">
        <h1>视频转码<span class="tag">H.264 / MP4</span></h1>
        <p class="hero-sub">libx264 · yuv420p · +faststart · 默认 main profile / level 4.0</p>
    </header>

    <div id="toolBanner" class="banner" hidden></div>

    <section class="card">
        <h2 class="card-title"><span class="step">1</span>选择视频文件</h2>
        <div class="row">
            <button id="btnPick" class="btn primary">选择视频文件…</button>
            <span id="fileLabel" class="file-label muted">尚未选择文件</span>
        </div>
        <div id="infoState" class="hint"></div>
        <div id="infoWrap" hidden>
            <div id="warnBox" class="warnbox" hidden></div>
            <div id="infoGrid" class="info-grid"></div>
        </div>
    </section>

    <section class="card">
        <h2 class="card-title"><span class="step">2</span>转码参数</h2>

        <div class="field">
            <div class="field-label">CRF（画质与体积的平衡）</div>
            <div class="crf-group" id="crfGroup">${crfButtons(FALLBACK_CRFS, DEFAULT_CRF)}</div>
            <div class="hint">数值越小画质越好、文件越大：18 ≈ 视觉无损，23 为默认，28 压缩更狠。</div>
        </div>

        <div class="field">
            <div class="field-label">视频规格（高级）</div>
            <div class="grid-2">
                <label class="select-box">
                    <span>Profile</span>
                    <select id="profileSel"></select>
                </label>
                <label class="select-box">
                    <span>Level</span>
                    <select id="levelSel"></select>
                </label>
            </div>
            <div class="hint" id="profileHint"></div>
            <div class="hint" id="levelHint"></div>
        </div>

        <div class="field">
            <div class="field-label">音频</div>
            <label class="check">
                <input type="checkbox" id="audioAac">
                <span>音频转码为 AAC 128k（默认 <code>-c:a copy</code> 原样复制音频流）</span>
            </label>
            <div class="hint" id="audioHint">当前：<code>-c:a copy</code></div>
        </div>

        <div class="field">
            <div class="field-label">输出目录</div>
            <div class="row">
                <input id="outDir" class="input" readonly placeholder="默认使用源文件目录下的 output 文件夹">
                <button id="btnOutDir" class="btn">选择…</button>
            </div>
            <div class="hint" id="outFileHint"></div>
        </div>
    </section>

    <section class="card">
        <h2 class="card-title"><span class="step">3</span>命令预览</h2>
        <pre id="cmdPreview" class="cmd">选择视频文件后显示将要执行的 ffmpeg 命令</pre>
        <ul id="notes" class="notes" hidden></ul>
    </section>

    <section class="card">
        <h2 class="card-title"><span class="step">4</span>执行</h2>
        <div class="row">
            <button id="btnStart" class="btn primary" disabled>开始转码</button>
            <button id="btnCancel" class="btn danger" disabled>取消</button>
            <button id="btnReveal" class="btn ghost" disabled>打开输出目录</button>
        </div>
        <div class="progress-block">
            <div class="progress"><div id="progressBar" class="progress-bar"></div></div>
            <div class="progress-meta">
                <span id="progressPct" class="pct">0%</span>
                <span id="statusText" class="status-text">就绪</span>
                <span id="progressStats" class="muted"></span>
            </div>
        </div>
        <details class="log" id="logDetails">
            <summary>FFmpeg 日志<span id="logCount" class="muted"></span></summary>
            <pre id="logBox"></pre>
        </details>
    </section>
</div>`;

// ---------- 渲染 ----------

function setStatus(
  text: string,
  kind: "idle" | "busy" | "ok" | "warn" | "error" = "idle",
): void {
  const node = el("statusText");
  node.textContent = text;
  node.className = `status-text status-${kind}`;
}

function renderTools(tools: ToolStatus): void {
  const banner = el("toolBanner");
  banner.hidden = false;
  banner.className = tools.available ? "banner ok" : "banner error";

  if (tools.available) {
    banner.innerHTML = `<b>FFmpeg 已就绪</b><div>${esc(tools.ffmpegVersion || tools.ffmpegPath)}</div>`;
    return;
  }
  banner.innerHTML = `<b>环境检查未通过</b><div>${esc(tools.message)}</div>`;
}

function infoSection(title: string, rows: Array<[string, string]>): string {
  const body = rows
    .filter(
      ([, value]) =>
        value !== undefined && value !== null && String(value).trim() !== "",
    )
    .map(
      ([key, value]) =>
        `<div class="k">${esc(key)}</div><div class="v">${esc(String(value))}</div>`,
    )
    .join("");
  if (!body) {
    return "";
  }
  return `<div class="info-section">
        <div class="info-section-title">${esc(title)}</div>
        <div class="info-rows">${body}</div>
    </div>`;
}

function renderMedia(info: MediaInfo): void {
  const video = info.video;
  const audio = info.audio;

  const sections = [
    infoSection("文件", [
      ["文件名", info.fileName],
      ["文件大小", info.sizeText],
      ["时长", info.durationText],
      ["容器", info.containerLong || info.container],
      ["总码率", info.bitRateText],
      [
        "轨道数",
        `音频 ${info.audioTracks} 条 · 字幕 ${info.subtitleTracks} 条`,
      ],
    ]),
    video
      ? infoSection("视频流（原始编码）", [
          [
            "编码",
            video.codecLong
              ? `${video.codec}（${video.codecLong}）`
              : video.codec,
          ],
          [
            "Profile / Level",
            [video.profile, video.level].filter(Boolean).join(" / "),
          ],
          ["分辨率", video.resolution],
          ["宽高比", video.aspectRatio],
          ["帧率", video.frameRate],
          ["像素格式", video.pixelFormat],
          ["位深", video.bitDepth],
          ["码率", video.bitRateText],
          ["色彩", video.colorInfo],
          ["扫描方式", video.scanType],
          [
            "建议 Level",
            video.recommendedLevel
              ? `${video.recommendedLevel}${video.recommendedLevelHint ? `（${video.recommendedLevelHint}）` : ""}`
              : "",
          ],
        ])
      : infoSection("视频流", [["提示", "未识别到视频流"]]),
    audio
      ? infoSection("音频流（原始编码）", [
          [
            "编码",
            audio.codecLong
              ? `${audio.codec}（${audio.codecLong}）`
              : audio.codec,
          ],
          ["Profile", audio.profile],
          ["采样率", audio.sampleRate],
          [
            "声道",
            audio.channels
              ? `${audio.channelLayout}（${audio.channels}）`
              : audio.channelLayout,
          ],
          ["码率", audio.bitRateText],
          ["采样格式", audio.sampleFormat],
        ])
      : "",
  ];

  el("infoGrid").innerHTML = sections.join("");
  el("infoWrap").hidden = false;

  const warnBox = el("warnBox");
  if (info.warnings && info.warnings.length > 0) {
    warnBox.hidden = false;
    warnBox.innerHTML = `<b>提示</b><ul>${info.warnings.map((w) => `<li>${esc(w)}</li>`).join("")}</ul>`;
  } else {
    warnBox.hidden = true;
    warnBox.innerHTML = "";
  }
}

function clearMedia(): void {
  el("infoWrap").hidden = true;
  el("infoGrid").innerHTML = "";
  el("warnBox").hidden = true;
  el("infoState").textContent = "";
}

function renderPlan(plan: CommandPlan): void {
  el("cmdPreview").textContent = plan.commandLine;

  const notes = el("notes");
  if (plan.notes.length > 0) {
    notes.hidden = false;
    notes.innerHTML = plan.notes
      .map((note) => `<li>${esc(note)}</li>`)
      .join("");
  } else {
    notes.hidden = true;
    notes.innerHTML = "";
  }

  el("outFileHint").innerHTML =
    `输出文件：<code>${esc(plan.outputName)}</code>（同名文件会被覆盖）`;
  el("audioHint").innerHTML =
    plan.audioMode === "aac"
      ? "当前：<code>-c:a aac -b:a 128k</code>"
      : "当前：<code>-c:a copy</code>（原样复制音频流）";
}

function setRunning(running: boolean): void {
  state.running = running;
  el<HTMLButtonElement>("btnPick").disabled = running;
  el<HTMLButtonElement>("btnOutDir").disabled = running;
  el<HTMLButtonElement>("btnCancel").disabled = !running;
  el<HTMLButtonElement>("btnStart").disabled =
    running || !state.sourcePath || !state.plan;
  el<HTMLButtonElement>("btnReveal").disabled = running || !state.plan;
  el<HTMLSelectElement>("profileSel").disabled = running;
  el<HTMLSelectElement>("levelSel").disabled = running;
  el("crfGroup").classList.toggle("is-locked", running);
}

function renderProgress(progress: TranscodeProgress | null): void {
  const bar = el("progressBar");
  if (!progress) {
    bar.classList.remove("is-indeterminate");
    bar.style.width = "0%";
    el("progressPct").textContent = "0%";
    el("progressStats").textContent = "";
    return;
  }

  if (progress.percent < 0) {
    bar.classList.add("is-indeterminate");
    bar.style.width = "100%";
    el("progressPct").textContent = "进行中";
  } else {
    bar.classList.remove("is-indeterminate");
    bar.style.width = `${Math.min(100, Math.max(0, progress.percent)).toFixed(1)}%`;
    el("progressPct").textContent = `${progress.percent.toFixed(1)}%`;
  }

  const parts: string[] = [];
  if (progress.processedText) {
    parts.push(`已处理 ${progress.processedText}`);
  }
  if (progress.speed) {
    parts.push(`速度 ${progress.speed}`);
  }
  if (progress.fps) {
    parts.push(`${progress.fps} fps`);
  }
  if (progress.etaText) {
    parts.push(progress.etaText);
  }
  if (progress.sizeText) {
    parts.push(`已写入 ${progress.sizeText}`);
  }
  el("progressStats").textContent = parts.join(" · ");
}

function renderLogs(): void {
  const box = el("logBox");
  const nearBottom = box.scrollTop + box.clientHeight >= box.scrollHeight - 24;
  box.textContent = state.logs.join("\n");
  el("logCount").textContent =
    state.logs.length > 0 ? `（${state.logs.length} 行）` : "";
  if (nearBottom) {
    box.scrollTop = box.scrollHeight;
  }
}

function appendLog(line: string): void {
  state.logs.push(line);
  if (state.logs.length > LOG_LIMIT) {
    state.logs.splice(0, state.logs.length - LOG_LIMIT);
  }
  renderLogs();
}

// ---------- 参数预设（Profile / Level） ----------

function fillSelect(id: string, items: OptionItem[], selected: string): void {
  const node = el<HTMLSelectElement>(id);
  node.innerHTML = items
    .map(
      (item) =>
        `<option value="${esc(item.value)}">${esc(item.label)}</option>`,
    )
    .join("");
  if (items.some((item) => item.value === selected)) {
    node.value = selected;
  } else if (items.length > 0) {
    node.value = items[0].value;
  }
}

/** levelValue 把 "4.2" 转成可比较的数值，"auto" 视为 0。 */
function levelValue(level: string): number {
  if (!level || level === AUTO_LEVEL) {
    return 0;
  }
  const value = Number.parseFloat(level);
  return Number.isFinite(value) ? value : 0;
}

function renderProfileHint(): void {
  const item = state.presets?.profiles.find((p) => p.value === state.profile);
  el("profileHint").textContent = item
    ? `${item.label.replace("（默认）", "")}：${item.hint}`
    : "";
}

/** renderLevelHint 展示当前 level 的含义；源视频需要更高等级时给出「一键应用」。 */
function renderLevelHint(): void {
  const node = el("levelHint");
  const video = state.info?.video ?? null;
  const recommended = video?.recommendedLevel ?? "";
  const recommendedHint = video?.recommendedLevelHint ?? "";
  const item = state.presets?.levels.find((l) => l.value === state.level);

  if (
    recommended &&
    state.level !== AUTO_LEVEL &&
    levelValue(state.level) < levelValue(recommended)
  ) {
    const why = video
      ? `按 ${video.resolution} / ${video.frameRate} 计算`
      : "按源视频计算";
    node.innerHTML =
      `<span class="warn-text">所选 Level ${esc(state.level)} 低于该视频所需：${esc(why)}` +
      `至少需要 Level ${esc(recommended)}${recommendedHint ? `（${esc(recommendedHint)}）` : ""}，否则 ffmpeg 可能报错。</span>` +
      `<button id="btnApplyLevel" class="btn tiny">应用 Level ${esc(recommended)}</button>`;
    el("btnApplyLevel").addEventListener("click", () =>
      selectLevel(recommended),
    );
    return;
  }

  if (state.level === AUTO_LEVEL) {
    node.textContent = recommended
      ? `自动：按源视频 ${video?.resolution ?? ""} / ${video?.frameRate ?? ""} 预计使用 Level ${recommended}${recommendedHint ? `（${recommendedHint}）` : ""}。`
      : "自动：不指定 -level，由 x264 按分辨率与帧率决定。";
    return;
  }

  node.textContent = item ? `Level ${state.level}：${item.hint}` : "";
}

function selectLevel(level: string): void {
  state.level = level;
  el<HTMLSelectElement>("levelSel").value = level;
  renderLevelHint();
  void refreshPlan();
}

function applyPresets(presets: TranscodePresets): void {
  state.presets = presets;

  if (presets.crfs.length > 0) {
    el("crfGroup").innerHTML = crfButtons(presets.crfs, state.crf);
  }
  fillSelect("profileSel", presets.profiles, state.profile);
  fillSelect("levelSel", presets.levels, state.level);

  // 以 select 的最终取值为准，避免 state 与界面不一致。
  state.profile = el<HTMLSelectElement>("profileSel").value || state.profile;
  state.level = el<HTMLSelectElement>("levelSel").value || state.level;

  renderProfileHint();
  renderLevelHint();
}

// ---------- 交互 ----------

let planToken = 0;

/** currentOptions 汇总当前界面选择，提交给 Go 侧。 */
function currentOptions(): TranscodeOptions {
  return {
    sourcePath: state.sourcePath,
    outputDir: state.outputDir,
    crf: state.crf,
    audioMode: state.audioAac ? "aac" : "copy",
    profile: state.profile,
    level: state.level,
  };
}

async function refreshPlan(): Promise<void> {
  if (!state.sourcePath) {
    state.plan = null;
    el("cmdPreview").textContent = "选择视频文件后显示将要执行的 ffmpeg 命令";
    el("notes").hidden = true;
    el("outFileHint").textContent = "";
    el<HTMLButtonElement>("btnStart").disabled = true;
    return;
  }

  const token = ++planToken;
  try {
    const plan = await api.planTranscode(currentOptions());
    if (token !== planToken) {
      return;
    }
    state.plan = plan;
    state.outputDir = plan.outputDir;
    el<HTMLInputElement>("outDir").value = plan.outputDir;
    renderPlan(plan);
    el<HTMLButtonElement>("btnStart").disabled = state.running;
  } catch (err) {
    if (token !== planToken) {
      return;
    }
    state.plan = null;
    el("cmdPreview").textContent = `无法生成命令：${errorText(err)}`;
    el("notes").hidden = true;
    el<HTMLButtonElement>("btnStart").disabled = true;
  }
}

async function pickFile(): Promise<void> {
  try {
    const selected = await api.selectVideoFile();
    if (!selected) {
      return;
    }
    state.sourcePath = selected;
    state.info = null;
    state.plan = null;
    state.outputDir = "";
    el("fileLabel").textContent = selected;
    clearMedia();
    el("infoState").textContent = "正在读取视频编码信息…";
    el<HTMLButtonElement>("btnStart").disabled = true;
    setStatus("就绪");

    const info = await api.probeMedia(selected);
    state.info = info;
    el("infoState").textContent = "";
    renderMedia(info);
    renderLevelHint();

    // 音频与 MP4 不兼容时后端会自动改用 AAC，界面同步勾选状态。
    if (info.audioTracks > 0 && !info.audioCompatible) {
      state.audioAac = true;
      el<HTMLInputElement>("audioAac").checked = true;
    }
    await refreshPlan();
    setStatus("已选择文件，可以开始转码");
  } catch (err) {
    el("infoState").textContent = "";
    setStatus(`读取失败：${errorText(err)}`, "error");
  }
}

async function pickOutputDir(): Promise<void> {
  try {
    const selected = await api.selectOutputDir(state.outputDir);
    if (!selected) {
      return;
    }
    state.outputDir = selected;
    el<HTMLInputElement>("outDir").value = selected;
    await refreshPlan();
  } catch (err) {
    setStatus(`选择输出目录失败：${errorText(err)}`, "error");
  }
}

async function startTranscode(): Promise<void> {
  if (!state.sourcePath || state.running) {
    return;
  }
  state.logs = [];
  renderLogs();
  renderProgress(null);
  setStatus("正在启动 ffmpeg…", "busy");
  try {
    const snapshot = await api.startTranscode(currentOptions());
    setRunning(true);
    if (snapshot.logTail) {
      state.logs = [...snapshot.logTail];
      renderLogs();
    }
    setStatus("转码中…", "busy");
  } catch (err) {
    setRunning(false);
    setStatus(`启动失败：${errorText(err)}`, "error");
  }
}

async function cancelTranscode(): Promise<void> {
  try {
    setStatus("正在取消…", "warn");
    await api.cancelTranscode();
  } catch (err) {
    setStatus(`取消失败：${errorText(err)}`, "error");
  }
}

async function revealOutput(): Promise<void> {
  const target = state.plan?.outputPath;
  if (!target) {
    return;
  }
  try {
    await api.revealInExplorer(target);
  } catch (err) {
    setStatus(`打开目录失败：${errorText(err)}`, "error");
  }
}

function onProgress(progress: TranscodeProgress): void {
  if (!state.running) {
    setRunning(true);
  }
  renderProgress(progress);
  setStatus("转码中…", "busy");
}

function onDone(snapshot: TranscodeSnapshot): void {
  setRunning(false);
  if (snapshot.plan) {
    state.plan = snapshot.plan;
    state.outputDir = snapshot.plan.outputDir;
    el<HTMLInputElement>("outDir").value = snapshot.plan.outputDir;
    if (snapshot.plan.profile) {
      state.profile = snapshot.plan.profile;
      el<HTMLSelectElement>("profileSel").value = snapshot.plan.profile;
    }
    if (snapshot.plan.level) {
      state.level = snapshot.plan.level;
      el<HTMLSelectElement>("levelSel").value = snapshot.plan.level;
    }
    renderPlan(snapshot.plan);
    renderProfileHint();
    renderLevelHint();
  }
  if (snapshot.logTail) {
    state.logs = [...snapshot.logTail];
    renderLogs();
  }

  if (snapshot.status === "completed") {
    el("progressBar").classList.remove("is-indeterminate");
    el("progressBar").style.width = "100%";
    el("progressPct").textContent = "100%";
    const size = snapshot.outputSizeText
      ? ` · 输出 ${snapshot.outputSizeText}`
      : "";
    const used = snapshot.elapsedText ? ` · 用时 ${snapshot.elapsedText}` : "";
    setStatus(`转码完成${size}${used}`, "ok");
  } else if (snapshot.status === "canceled") {
    setStatus(`已取消：${snapshot.message || ""}`, "warn");
  } else if (snapshot.status === "failed") {
    setStatus(`失败：${snapshot.message || "未知错误"}`, "error");
  }
}

async function restoreStatus(): Promise<void> {
  try {
    const snapshot = await api.getTranscodeStatus();
    if (snapshot.logTail && snapshot.logTail.length > 0) {
      state.logs = [...snapshot.logTail];
      renderLogs();
    }
    if (snapshot.running) {
      setRunning(true);
      setStatus("转码中…", "busy");
      renderProgress({
        percent: snapshot.percent,
        processedSec: snapshot.processedSec,
        processedText: snapshot.processedText,
        speed: snapshot.speed,
        fps: snapshot.fps,
        frame: snapshot.frame,
        etaText: snapshot.etaText,
        sizeText: "",
      });
    } else if (snapshot.status !== "idle") {
      onDone(snapshot);
    }
  } catch {
    // 尚无任务时忽略。
  }
}

// ---------- 启动 ----------

async function init(): Promise<void> {
  el("btnPick").addEventListener("click", () => void pickFile());
  el("btnOutDir").addEventListener("click", () => void pickOutputDir());
  el("btnStart").addEventListener("click", () => void startTranscode());
  el("btnCancel").addEventListener("click", () => void cancelTranscode());
  el("btnReveal").addEventListener("click", () => void revealOutput());

  el("crfGroup").addEventListener("click", (event) => {
    const target = (event.target as HTMLElement).closest<HTMLElement>(
      ".crf-opt",
    );
    if (!target || state.running) {
      return;
    }
    const value = Number(target.dataset.crf);
    if (!Number.isFinite(value)) {
      return;
    }
    state.crf = value;
    el("crfGroup")
      .querySelectorAll(".crf-opt")
      .forEach((node) => node.classList.toggle("is-active", node === target));
    void refreshPlan();
  });

  el<HTMLInputElement>("audioAac").addEventListener("change", (event) => {
    state.audioAac = (event.target as HTMLInputElement).checked;
    void refreshPlan();
  });

  el<HTMLSelectElement>("profileSel").addEventListener("change", (event) => {
    state.profile = (event.target as HTMLSelectElement).value;
    renderProfileHint();
    void refreshPlan();
  });

  el<HTMLSelectElement>("levelSel").addEventListener("change", (event) => {
    selectLevel((event.target as HTMLSelectElement).value);
  });

  api.on("transcode:progress", (payload) =>
    onProgress(payload as TranscodeProgress),
  );
  api.on("transcode:log", (payload) => appendLog(String(payload)));
  api.on("transcode:done", (payload) => onDone(payload as TranscodeSnapshot));

  try {
    const tools = await api.checkTools();
    state.tools = tools;
    renderTools(tools);
  } catch (err) {
    renderTools({ available: false, message: errorText(err) } as ToolStatus);
  }

  try {
    applyPresets(await api.getPresets());
  } catch (err) {
    el("levelHint").textContent = `无法读取可选档位：${errorText(err)}`;
  }

  await restoreStatus();
}

void init();
