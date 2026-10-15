import { Fragment, type ReactElement, useCallback, useEffect, useRef, useState } from 'react';
import * as api from './api';
import DetectModal from './DetectModal';
import './App.css';
import type {
  CommandPlan,
  MediaInfo,
  OptionItem,
  TranscodeOptions,
  TranscodePresets,
  TranscodeProgress,
  TranscodeSnapshot,
  ToolDetection,
} from './types';

/** 环境检测状态：detecting=检测中，ok=已就绪，missing=未通过。 */
type ToolPhase = 'detecting' | 'ok' | 'missing';

const DEFAULT_CRF = 23;
/** 与 Go 侧默认档位保持一致，取不到预设时作为兜底。 */
const DEFAULT_PROFILE = 'high';
const DEFAULT_LEVEL = '4.1';
const AUTO_LEVEL = 'auto';
const LOG_LIMIT = 300;

const FALLBACK_CRFS: OptionItem[] = [
  { value: '18', label: '18', hint: '高画质 · 体积最大' },
  { value: '23', label: '23', hint: '默认 · 推荐' },
  { value: '28', label: '28', hint: '小体积 · 画质下降' },
];

interface AppState {
  toolPhase: ToolPhase;
  tools: ToolDetection | null;
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
  status: { text: string; kind: 'idle' | 'busy' | 'ok' | 'warn' | 'error' };
  logs: string[];
  progress: {
    percent: number;
    stats: string;
    indeterminate: boolean;
  };
}

const INITIAL: AppState = {
  toolPhase: 'detecting',
  tools: null,
  info: null,
  presets: null,
  sourcePath: '',
  crf: DEFAULT_CRF,
  audioAac: false,
  profile: DEFAULT_PROFILE,
  level: DEFAULT_LEVEL,
  outputDir: '',
  plan: null,
  running: false,
  status: { text: '就绪', kind: 'idle' },
  logs: [],
  progress: { percent: 0, stats: '', indeterminate: false },
};

function levelValue(level: string): number {
  if (!level || level === AUTO_LEVEL) {
    return 0;
  }
  const value = Number.parseFloat(level);
  return Number.isFinite(value) ? value : 0;
}

function errorText(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

function InfoSection({
  title,
  rows,
}: {
  title: string;
  rows: Array<[string, string]>;
}): ReactElement | null {
  const body = rows.filter(
    ([, value]) => value !== undefined && value !== null && String(value).trim() !== '',
  );
  if (body.length === 0) {
    return null;
  }
  return (
    <div className="info-section">
      <div className="info-section-title">{title}</div>
      <div className="info-rows">
        {body.map(([key, value]) => (
          <Fragment key={key}>
            <div className="k">{key}</div>
            <div className="v">{value}</div>
          </Fragment>
        ))}
      </div>
    </div>
  );
}

export default function App() {
  const [state, setState] = useState<AppState>(INITIAL);
  const [theme, setTheme] = useState<'light' | 'dark'>(() =>
    typeof localStorage !== 'undefined' && localStorage.getItem('theme') === 'dark'
      ? 'dark'
      : 'light',
  );
  const logBoxRef = useRef<HTMLPreElement | null>(null);
  const planToken = useRef(0);
  const stateRef = useRef(state);
  stateRef.current = state;

  // 主题：默认亮色，应用到 <html data-theme> 并持久化到 localStorage。
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    try {
      localStorage.setItem('theme', theme);
    } catch {
      // 忽略隐私模式下的写入失败。
    }
  }, [theme]);

  const toggleTheme = useCallback(() => {
    setTheme((prev) => (prev === 'light' ? 'dark' : 'light'));
  }, []);

  const patch = useCallback((p: Partial<AppState>) => {
    setState((prev) => ({ ...prev, ...p }));
  }, []);

  // ---------- 环境检测 ----------
  const detectingRef = useRef(false);
  const [showDetect, setShowDetect] = useState(false);

  const runDetect = useCallback(async () => {
    if (detectingRef.current) {
      return;
    }
    detectingRef.current = true;
    patch({ toolPhase: 'detecting', tools: null });
    try {
      const det = await api.checkTools();
      setState((prev) => ({
        ...prev,
        tools: det,
        toolPhase: det.available ? 'ok' : 'missing',
      }));
    } catch (err) {
      setState((prev) => ({
        ...prev,
        tools: {
          available: false,
          ffmpegPath: '',
          ffprobePath: '',
          ffmpegVersion: '',
          message: `检测失败：${errorText(err)}`,
          sources: [],
          binDir: '',
        },
        toolPhase: 'missing',
      }));
    } finally {
      detectingRef.current = false;
    }
  }, [patch]);

  const setStatus = useCallback(
    (text: string, kind: AppState['status']['kind'] = 'idle') => {
      patch({ status: { text, kind } });
    },
    [patch],
  );

  const chooseFfmpeg = useCallback(async () => {
    try {
      const det = await api.chooseFfmpeg();
      setState((prev) => ({
        ...prev,
        tools: det,
        toolPhase: det.available ? 'ok' : 'missing',
      }));
      if (det.available) {
        setShowDetect(false);
      }
    } catch (err) {
      setStatus(`设置失败：${errorText(err)}`, 'error');
    }
  }, [setStatus]);

  const clearFfmpeg = useCallback(async () => {
    try {
      const det = await api.clearFfmpegSetting();
      setState((prev) => ({
        ...prev,
        tools: det,
        toolPhase: det.available ? 'ok' : 'missing',
      }));
    } catch (err) {
      setStatus(`重置失败：${errorText(err)}`, 'error');
    }
  }, [setStatus]);

  const openBinDir = useCallback(async () => {
    try {
      await api.openBinDir();
    } catch (err) {
      setStatus(`打开目录失败：${errorText(err)}`, 'error');
    }
  }, [setStatus]);

  const setRunning = useCallback(
    (running: boolean) => {
      patch({ running });
    },
    [patch],
  );

  const setProgress = useCallback(
    (progress: TranscodeProgress | null) => {
      if (!progress) {
        patch({
          progress: { percent: 0, stats: '', indeterminate: false },
        });
        return;
      }
      if (progress.percent < 0) {
        patch({ progress: { percent: 100, stats: '', indeterminate: true } });
        return;
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
      patch({
        progress: {
          percent: Math.min(100, Math.max(0, progress.percent)),
          stats: parts.join(' · '),
          indeterminate: false,
        },
      });
    },
    [patch],
  );

  // ---------- 命令预览 ----------
  const refreshPlan = useCallback(async () => {
    const cur = stateRef.current;
    if (!cur.sourcePath) {
      patch({
        plan: null,
        status: { text: '就绪', kind: 'idle' },
      });
      return;
    }
    const token = ++planToken.current;
    const opts: TranscodeOptions = {
      sourcePath: cur.sourcePath,
      outputDir: cur.outputDir,
      crf: cur.crf,
      audioMode: cur.audioAac ? 'aac' : 'copy',
      profile: cur.profile,
      level: cur.level,
    };
    try {
      const plan = await api.planTranscode(opts);
      if (token !== planToken.current) {
        return;
      }
      patch({
        plan,
        outputDir: plan.outputDir,
        status: { text: '已选择文件，可以开始转码', kind: 'idle' },
      });
    } catch (err) {
      if (token !== planToken.current) {
        return;
      }
      patch({
        plan: null,
        status: { text: `无法生成命令：${errorText(err)}`, kind: 'error' },
      });
    }
  }, [patch]);

  const selectLevel = useCallback(
    (level: string) => {
      patch({ level });
      void refreshPlan();
    },
    [patch, refreshPlan],
  );

  // ---------- 交互 ----------
  const pickFile = useCallback(async () => {
    try {
      const selected = await api.selectVideoFile();
      if (!selected) {
        return;
      }
      setState((prev) => ({
        ...prev,
        sourcePath: selected,
        info: null,
        plan: null,
        outputDir: '',
        status: { text: '正在读取视频编码信息…', kind: 'idle' },
      }));
      const info = await api.probeMedia(selected);
      setState((prev) => ({
        ...prev,
        info,
        // 音频与 MP4 不兼容时后端会自动改用 AAC，界面同步勾选状态。
        audioAac: info.audioTracks > 0 && !info.audioCompatible ? true : prev.audioAac,
        status: { text: '已选择文件，可以开始转码', kind: 'idle' },
      }));
      await refreshPlan();
    } catch (err) {
      setStatus(`读取失败：${errorText(err)}`, 'error');
    }
  }, [patch, refreshPlan, setStatus]);

  const pickOutputDir = useCallback(async () => {
    const current = stateRef.current.outputDir;
    try {
      const selected = await api.selectOutputDir(current);
      if (!selected) {
        return;
      }
      patch({ outputDir: selected });
      await refreshPlan();
    } catch (err) {
      setStatus(`选择输出目录失败：${errorText(err)}`, 'error');
    }
  }, [patch, refreshPlan, setStatus]);

  const startTranscode = useCallback(async () => {
    const cur = stateRef.current;
    if (!cur.sourcePath || cur.running) {
      return;
    }
    patch({ logs: [], progress: { percent: 0, stats: '', indeterminate: false } });
    setStatus('正在启动 ffmpeg…', 'busy');
    try {
      const snapshot = await api.startTranscode({
        sourcePath: cur.sourcePath,
        outputDir: cur.outputDir,
        crf: cur.crf,
        audioMode: cur.audioAac ? 'aac' : 'copy',
        profile: cur.profile,
        level: cur.level,
      });
      setRunning(true);
      if (snapshot.logTail && snapshot.logTail.length > 0) {
        patch({ logs: [...snapshot.logTail] });
      }
      setStatus('转码中…', 'busy');
    } catch (err) {
      setRunning(false);
      setStatus(`启动失败：${errorText(err)}`, 'error');
    }
  }, [patch, setRunning, setStatus]);

  const cancelTranscode = useCallback(async () => {
    try {
      setStatus('正在取消…', 'warn');
      await api.cancelTranscode();
    } catch (err) {
      setStatus(`取消失败：${errorText(err)}`, 'error');
    }
  }, [setStatus]);

  const revealOutput = useCallback(async () => {
    const target = stateRef.current.plan?.outputPath;
    if (!target) {
      return;
    }
    try {
      await api.revealInExplorer(target);
    } catch (err) {
      setStatus(`打开目录失败：${errorText(err)}`, 'error');
    }
  }, [setStatus]);

  // ---------- 事件 ----------
  const onProgress = useCallback(
    (progress: TranscodeProgress) => {
      if (!stateRef.current.running) {
        setRunning(true);
      }
      setProgress(progress);
      setStatus('转码中…', 'busy');
    },
    [setProgress, setRunning, setStatus],
  );

  const onLog = useCallback((line: string) => {
    setState((prev) => {
      const logs = [...prev.logs, line];
      if (logs.length > LOG_LIMIT) {
        logs.splice(0, logs.length - LOG_LIMIT);
      }
      return { ...prev, logs };
    });
  }, []);

  const onDone = useCallback(
    (snapshot: TranscodeSnapshot) => {
      setRunning(false);
      const next: Partial<AppState> = {};
      if (snapshot.plan) {
        next.plan = snapshot.plan;
        next.outputDir = snapshot.plan.outputDir;
        if (snapshot.plan.profile) {
          next.profile = snapshot.plan.profile;
        }
        if (snapshot.plan.level) {
          next.level = snapshot.plan.level;
        }
      }
      if (snapshot.logTail && snapshot.logTail.length > 0) {
        next.logs = [...snapshot.logTail];
      }
      if (snapshot.status === 'completed') {
        const size = snapshot.outputSizeText ? ` · 输出 ${snapshot.outputSizeText}` : '';
        const used = snapshot.elapsedText ? ` · 用时 ${snapshot.elapsedText}` : '';
        next.status = { text: `转码完成${size}${used}`, kind: 'ok' };
      } else if (snapshot.status === 'canceled') {
        next.status = { text: `已取消：${snapshot.message || ''}`, kind: 'warn' };
      } else if (snapshot.status === 'failed') {
        next.status = {
          text: `失败：${snapshot.message || '未知错误'}`,
          kind: 'error',
        };
      }
      patch(next);
    },
    [patch, setRunning],
  );

  const restoreStatus = useCallback(async () => {
    try {
      const snapshot = await api.getTranscodeStatus();
      if (snapshot.logTail && snapshot.logTail.length > 0) {
        patch({ logs: [...snapshot.logTail] });
      }
      if (snapshot.running) {
        setRunning(true);
        setStatus('转码中…', 'busy');
        setProgress({
          percent: snapshot.percent,
          processedSec: snapshot.processedSec,
          processedText: snapshot.processedText,
          speed: snapshot.speed,
          fps: snapshot.fps,
          frame: snapshot.frame,
          etaText: snapshot.etaText,
          sizeText: '',
        });
      } else if (snapshot.status !== 'idle') {
        onDone(snapshot);
      }
    } catch {
      // 尚无任务时忽略。
    }
  }, [onDone, patch, setProgress, setRunning, setStatus]);

  // ---------- 启动 ----------
  useEffect(() => {
    const offProgress = api.on('transcode:progress', (payload) =>
      onProgress(payload as TranscodeProgress),
    );
    const offLog = api.on('transcode:log', (payload) => onLog(String(payload)));
    const offDone = api.on('transcode:done', (payload) => onDone(payload as TranscodeSnapshot));

    (async () => {
      await runDetect();
      try {
        const presets = await api.getPresets();
        setState((prev) => {
          const profile =
            presets.profiles.find((p) => p.value === prev.profile)?.value ??
            presets.profiles[0]?.value ??
            prev.profile;
          const level =
            presets.levels.find((l) => l.value === prev.level)?.value ??
            presets.levels[0]?.value ??
            prev.level;
          return { ...prev, presets, profile, level };
        });
      } catch (err) {
        setStatus(`无法读取可选档位：${errorText(err)}`, 'error');
      }
      await restoreStatus();
    })();

    return () => {
      offProgress();
      offLog();
      offDone();
    };
  }, [onDone, onLog, onProgress, restoreStatus, runDetect, setStatus]);

  // 日志滚动条贴近底部。
  useEffect(() => {
    const box = logBoxRef.current;
    if (box) {
      box.scrollTop = box.scrollHeight;
    }
  }, [state.logs]);

  // ---------- 派生渲染数据 ----------
  const crfs = state.presets?.crfs ?? FALLBACK_CRFS;
  const info = state.info;
  const video = info?.video ?? null;
  const audio = info?.audio ?? null;
  const recommended = video?.recommendedLevel ?? '';
  const recommendedHint = video?.recommendedLevelHint ?? '';

  const profileItem = state.presets?.profiles.find((p) => p.value === state.profile);
  const profileHint = profileItem
    ? `${profileItem.label.replace('（默认）', '')}：${profileItem.hint}`
    : '';

  let levelHint: ReactElement | string = '';
  if (
    recommended &&
    state.level !== AUTO_LEVEL &&
    levelValue(state.level) < levelValue(recommended)
  ) {
    const why = video ? `按 ${video.resolution} / ${video.frameRate} 计算` : '按源视频计算';
    levelHint = (
      <span className="warn-text">
        所选 Level {state.level} 低于该视频所需：{why}
        至少需要 Level {recommended}
        {recommendedHint ? `（${recommendedHint}）` : ''}，否则 ffmpeg 可能报错。
        <button className="btn tiny" onClick={() => selectLevel(recommended)} type="button">
          应用 Level {recommended}
        </button>
      </span>
    );
  } else if (state.level === AUTO_LEVEL) {
    levelHint = recommended
      ? `自动：按源视频 ${video?.resolution ?? ''} / ${
          video?.frameRate ?? ''
        } 预计使用 Level ${recommended}${recommendedHint ? `（${recommendedHint}）` : ''}。`
      : '自动：不指定 -level，由 x264 按分辨率与帧率决定。';
  } else {
    const item = state.presets?.levels.find((l) => l.value === state.level);
    levelHint = item ? `Level ${state.level}：${item.hint}` : '';
  }

  const infoSections: Array<[string, Array<[string, string]>]> = [];
  if (info) {
    infoSections.push([
      '文件',
      [
        ['文件名', info.fileName],
        ['文件大小', info.sizeText],
        ['时长', info.durationText],
        ['容器', info.containerLong || info.container],
        ['总码率', info.bitRateText],
        ['轨道数', `音频 ${info.audioTracks} 条 · 字幕 ${info.subtitleTracks} 条`],
      ],
    ]);
    infoSections.push([
      video ? '视频流（原始编码）' : '视频流',
      video
        ? [
            ['编码', video.codecLong ? `${video.codec}（${video.codecLong}）` : video.codec],
            ['Profile / Level', [video.profile, video.level].filter(Boolean).join(' / ')],
            ['分辨率', video.resolution],
            ['宽高比', video.aspectRatio],
            ['帧率', video.frameRate],
            ['像素格式', video.pixelFormat],
            ['位深', video.bitDepth],
            ['码率', video.bitRateText],
            ['色彩', video.colorInfo],
            ['扫描方式', video.scanType],
            [
              '建议 Level',
              video.recommendedLevel
                ? `${video.recommendedLevel}${
                    video.recommendedLevelHint ? `（${video.recommendedLevelHint}）` : ''
                  }`
                : '',
            ],
          ]
        : [['提示', '未识别到视频流']],
    ]);
    if (audio) {
      infoSections.push([
        '音频流（原始编码）',
        [
          ['编码', audio.codecLong ? `${audio.codec}（${audio.codecLong}）` : audio.codec],
          ['Profile', audio.profile],
          ['采样率', audio.sampleRate],
          [
            '声道',
            audio.channels ? `${audio.channelLayout}（${audio.channels}）` : audio.channelLayout,
          ],
          ['码率', audio.bitRateText],
          ['采样格式', audio.sampleFormat],
        ],
      ]);
    }
  }

  const startDisabled = state.running || !state.sourcePath || !state.plan;
  const revealDisabled = state.running || !state.plan;

  return (
    <div className="shell">
      <button
        className="theme-toggle"
        onClick={() => toggleTheme()}
        type="button"
        title={theme === 'light' ? '切换到暗色' : '切换到亮色'}
        aria-label="切换主题"
      >
        <span className="ic">{theme === 'light' ? '🌙' : '☀️'}</span>
        <span>{theme === 'light' ? '暗色' : '亮色'}</span>
      </button>

      <header className="hero">
        <h1>
          视频转码<span className="tag">H.264 / MP4</span>
        </h1>
        <p className="hero-sub">libx264 · yuv420p · +faststart · 默认 high profile / level 4.1</p>
        <p className="hero-sub">
          基于 FFmpeg 将视频转码为浏览器兼容的 H.264 / MP4，以便在主流浏览器中正常播放。
        </p>
      </header>

      <div className={`banner banner-${state.toolPhase}`}>
        <div className="banner-main">
          {state.toolPhase === 'detecting' && (
            <>
              <b>
                <span className="spin" /> FFmpeg 环境检测中…
              </b>
              <div>正在按「用户设置 → 程序 bin 目录 → PATH」的顺序查找 ffmpeg / ffprobe。</div>
            </>
          )}
          {state.toolPhase === 'ok' && (
            <>
              <b>FFmpeg 已就绪</b>
              <div>{state.tools?.ffmpegVersion || state.tools?.ffmpegPath}</div>
            </>
          )}
          {state.toolPhase === 'missing' && (
            <>
              <b>环境检查未通过</b>
              <div>{state.tools?.message || '未找到可用的 ffmpeg / ffprobe。'}</div>
            </>
          )}
        </div>
        <div className="banner-actions">
          {state.toolPhase !== 'detecting' && (
            <button className="btn tiny" onClick={() => void runDetect()} type="button">
              重新检测
            </button>
          )}
          <button
            className="icon-btn"
            onClick={() => setShowDetect(true)}
            type="button"
            title="查看检测详情"
            aria-label="查看检测详情"
          >
            i
          </button>
        </div>
      </div>

      <section className="card">
        <h2 className="card-title">
          <span className="step">1</span>选择视频文件
        </h2>
        <div className="row">
          <button
            className="btn primary"
            onClick={() => void pickFile()}
            disabled={state.running || ['detecting', 'missing'].includes(state.toolPhase)}
            type="button"
          >
            选择视频文件
          </button>
          <span className="file-label muted">{state.sourcePath || '尚未选择文件'}</span>
        </div>
        {state.info && (
          <div className="info-wrap">
            {state.info.warnings && state.info.warnings.length > 0 && (
              <div className="warnbox">
                <b>提示</b>
                <ul>
                  {state.info.warnings.map((w) => (
                    <li key={w}>{w}</li>
                  ))}
                </ul>
              </div>
            )}
            <div className="info-grid">
              {infoSections.map(([title, rows]) => (
                <InfoSection key={title} title={title} rows={rows} />
              ))}
            </div>
          </div>
        )}
      </section>

      <section className="card">
        <h2 className="card-title">
          <span className="step">2</span>转码参数
        </h2>

        <div className="field">
          <div className="field-label">CRF（画质与体积的平衡）</div>
          <div className={`crf-group${state.running ? ' is-locked' : ''}`}>
            {crfs.map((item) => (
              <button
                key={item.value}
                className={`crf-opt${Number(item.value) === state.crf ? ' is-active' : ''}`}
                onClick={() => {
                  if (state.running) {
                    return;
                  }
                  setState((prev) => ({ ...prev, crf: Number(item.value) }));
                  void refreshPlan();
                }}
                type="button"
              >
                <b>{item.label}</b>
                <span>{item.hint}</span>
              </button>
            ))}
          </div>
          <div className="hint">
            数值越小画质越好、文件越大：18 ≈ 视觉无损，23 为默认，28 压缩更狠。
          </div>
        </div>

        <div className="field">
          <div className="field-label">视频规格（高级）</div>
          <div className="grid-2">
            <label className="select-box">
              <span>Profile</span>
              <select
                value={state.profile}
                disabled={state.running}
                onChange={(e) => {
                  setState((prev) => ({ ...prev, profile: e.target.value }));
                  void refreshPlan();
                }}
              >
                {state.presets?.profiles.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="select-box">
              <span>Level</span>
              <select
                value={state.level}
                disabled={state.running}
                onChange={(e) => selectLevel(e.target.value)}
              >
                {state.presets?.levels.map((l) => (
                  <option key={l.value} value={l.value}>
                    {l.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {profileHint && <div className="hint">{profileHint}</div>}
          <div className="hint">{levelHint}</div>
        </div>

        <div className="field">
          <div className="field-label">音频</div>
          <label className="check">
            <input
              type="checkbox"
              checked={state.audioAac}
              disabled={state.running}
              onChange={(e) => {
                setState((prev) => ({ ...prev, audioAac: e.target.checked }));
                void refreshPlan();
              }}
            />
            <span>
              音频转码为 AAC 128k（默认 <code>-c:a copy</code> 原样复制音频流）
            </span>
          </label>
        </div>

        <div className="field">
          <div className="field-label">输出目录</div>
          <div className="row">
            <input
              className="input"
              readOnly
              placeholder="默认使用源文件目录下的 output 文件夹"
              value={state.outputDir}
            />
            <button
              className="btn"
              onClick={() => void pickOutputDir()}
              disabled={state.running || ['detecting', 'missing'].includes(state.toolPhase)}
              type="button"
            >
              选择
            </button>
          </div>
          {state.plan && (
            <div className="hint">
              输出文件：<code>{state.plan.outputName}</code>（同名文件会被覆盖）
            </div>
          )}
        </div>
      </section>

      <section className="card">
        <h2 className="card-title">
          <span className="step">3</span>命令预览
        </h2>
        <pre className="cmd">
          {state.plan ? state.plan.commandLine : '选择视频文件后显示将要执行的 ffmpeg 命令'}
        </pre>
        {state.plan && state.plan.notes.length > 0 && (
          <ul className="notes">
            {state.plan.notes.map((note, i) => (
              <li key={i}>{note}</li>
            ))}
          </ul>
        )}
      </section>

      <section className="card">
        <h2 className="card-title">
          <span className="step">4</span>执行
        </h2>
        <div className="row">
          <button
            className="btn primary"
            onClick={() => void startTranscode()}
            disabled={startDisabled}
            type="button"
          >
            开始转码
          </button>
          <button
            className="btn danger"
            onClick={() => void cancelTranscode()}
            disabled={!state.running}
            type="button"
          >
            取消
          </button>
          <button
            className="btn ghost"
            onClick={() => void revealOutput()}
            disabled={revealDisabled}
            type="button"
          >
            打开输出目录
          </button>
        </div>
        <div className="progress-block">
          <div className="progress">
            <div
              className={`progress-bar${state.progress.indeterminate ? ' is-indeterminate' : ''}`}
              style={{ width: `${state.progress.percent}%` }}
            />
          </div>
          <div className="progress-meta">
            <span className="pct">
              {state.progress.indeterminate ? '进行中' : `${state.progress.percent.toFixed(1)}%`}
            </span>
            <span className={`status-text status-${state.status.kind}`}>{state.status.text}</span>
            <span className="muted">{state.progress.stats}</span>
          </div>
        </div>
        <details className="log">
          <summary>
            FFmpeg 日志
            {state.logs.length > 0 && <span className="muted">（{state.logs.length} 行）</span>}
          </summary>
          <pre ref={logBoxRef}>{state.logs.join('\n')}</pre>
        </details>
      </section>

      {showDetect && (
        <DetectModal
          detection={state.tools}
          onClose={() => setShowDetect(false)}
          onChoose={() => void chooseFfmpeg()}
          onClear={() => void clearFfmpeg()}
          onOpenBinDir={() => void openBinDir()}
          onDownloadFFmpeg={() => void api.openFFmpegDownloadPage()}
        />
      )}

      <footer className="app-foot">
        <p className="authorinfo">
          <span>作者：GHLandy</span>
          <span className="sep">·</span>
          <a href="mailto:liftland953@163.com" title="mailto:liftland953@163.com">
            liftland953@163.com
          </a>
        </p>
        <p className="tech">基于 Wails · FFmpeg / libx264 · React 19 · TypeScript · Vite 构建</p>
        <p className="disclaimer">
          免责声明：本工具按原样提供，用于本地视频转码，不随软件分发 FFmpeg
          二进制（须由用户自行配置）；不对转码结果、源文件损坏或版权问题承担责任，使用本工具请遵守当地法律法规及
          FFmpeg 许可证要求。
        </p>
      </footer>
    </div>
  );
}
