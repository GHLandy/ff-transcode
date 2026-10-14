import type {
    AppSettings,
    CommandPlan,
    MediaInfo,
    ToolDetection,
    TranscodeOptions,
    TranscodePresets,
    TranscodeSnapshot,
} from './types';

interface GoApp {
    CheckTools(): Promise<ToolDetection>;
    GetUserSettings(): Promise<AppSettings>;
    ChooseFFmpeg(): Promise<ToolDetection>;
    ClearFFmpegSetting(): Promise<ToolDetection>;
    OpenBinDir(): Promise<void>;
    GetPresets(): Promise<TranscodePresets>;
    SelectVideoFile(): Promise<string>;
    SelectOutputDir(current: string): Promise<string>;
    ProbeMedia(path: string): Promise<MediaInfo>;
    PlanTranscode(opts: TranscodeOptions): Promise<CommandPlan>;
    StartTranscode(opts: TranscodeOptions): Promise<TranscodeSnapshot>;
    CancelTranscode(): Promise<void>;
    GetTranscodeStatus(): Promise<TranscodeSnapshot>;
    RevealInExplorer(path: string): Promise<void>;
}

interface RuntimeAPI {
    EventsOn(eventName: string, callback: (data: unknown) => void): () => void;
    EventsOff(eventName: string, ...additionalEventNames: string[]): void;
}

declare global {
    interface Window {
        go?: { main: { App: GoApp } };
        runtime?: RuntimeAPI;
    }
}

const RUNTIME_MISSING = 'Wails 运行时不可用，请通过 wails dev 或 wails build 启动本应用。';

function app(): GoApp {
    const bridge = window.go?.main?.App;
    if (!bridge) {
        throw new Error(RUNTIME_MISSING);
    }
    return bridge;
}

export function checkTools(): Promise<ToolDetection> {
    return app().CheckTools();
}

/** 读取当前用户设置。 */
export function getUserSettings(): Promise<AppSettings> {
    return app().GetUserSettings();
}

/** 打开文件对话框手动指定 ffmpeg，并返回重新检测后的结果。 */
export function chooseFfmpeg(): Promise<ToolDetection> {
    return app().ChooseFFmpeg();
}

/** 清除用户手动设置，回退到自动查找，并返回重新检测后的结果。 */
export function clearFfmpegSetting(): Promise<ToolDetection> {
    return app().ClearFFmpegSetting();
}

/** 在文件管理器中打开程序 bin 目录。 */
export function openBinDir(): Promise<void> {
    return app().OpenBinDir();
}

/** 获取 Go 侧提供的 CRF / Profile / Level 可选项。 */
export function getPresets(): Promise<TranscodePresets> {
    return app().GetPresets();
}

export function selectVideoFile(): Promise<string> {
    return app().SelectVideoFile();
}

export function selectOutputDir(current: string): Promise<string> {
    return app().SelectOutputDir(current);
}

export function probeMedia(path: string): Promise<MediaInfo> {
    return app().ProbeMedia(path);
}

export function planTranscode(opts: TranscodeOptions): Promise<CommandPlan> {
    return app().PlanTranscode(opts);
}

export function startTranscode(opts: TranscodeOptions): Promise<TranscodeSnapshot> {
    return app().StartTranscode(opts);
}

export function cancelTranscode(): Promise<void> {
    return app().CancelTranscode();
}

export function getTranscodeStatus(): Promise<TranscodeSnapshot> {
    return app().GetTranscodeStatus();
}

export function revealInExplorer(path: string): Promise<void> {
    return app().RevealInExplorer(path);
}

/** 订阅后端事件；运行时不完整时返回一个空的退订函数。 */
export function on(event: string, callback: (data: any) => void): () => void {
    const runtime = window.runtime;
    if (!runtime || typeof runtime.EventsOn !== 'function') {
        return () => undefined;
    }
    return runtime.EventsOn(event, callback);
}
