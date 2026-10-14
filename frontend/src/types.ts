// 与 Go 后端返回的 JSON 结构一一对应。

export interface VideoInfo {
    index: number;
    codec: string;
    codecLong: string;
    profile: string;
    level: string;
    width: number;
    height: number;
    resolution: string;
    aspectRatio: string;
    pixelFormat: string;
    bitDepth: string;
    frameRate: string;
    bitRateText: string;
    colorInfo: string;
    scanType: string;
    durationText: string;
    /** 编码该分辨率/帧率所需的最低 H.264 等级。 */
    recommendedLevel: string;
    recommendedLevelHint: string;
}

export interface AudioInfo {
    index: number;
    codec: string;
    codecLong: string;
    profile: string;
    sampleRate: string;
    channels: number;
    channelLayout: string;
    bitRateText: string;
    sampleFormat: string;
}

export interface MediaInfo {
    path: string;
    fileName: string;
    sizeBytes: number;
    sizeText: string;
    durationSec: number;
    durationText: string;
    container: string;
    containerLong: string;
    bitRateText: string;
    video: VideoInfo | null;
    audio: AudioInfo | null;
    audioTracks: number;
    subtitleTracks: number;
    warnings: string[];
    audioCodecs: string[];
    audioCompatible: boolean;
    bitmapSubtitles: string[];
}

/** 查找来源：user=用户设置，bin=程序 bin 目录，path=系统 PATH。 */
export type DetectSourceKind = 'user' | 'bin' | 'path';

/** 查找过程中的一步。 */
export interface DetectSource {
    kind: DetectSourceKind;
    label: string;
    detail: string;
    found: boolean;
    note: string;
}

/** 一次完整的 ffmpeg / ffprobe 检测结果。 */
export interface ToolDetection {
    available: boolean;
    ffmpegPath: string;
    ffprobePath: string;
    ffmpegVersion: string;
    message: string;
    sources: DetectSource[];
    /** 程序 bin 目录的绝对路径。 */
    binDir: string;
}

/** 持久化的用户设置。 */
export interface AppSettings {
    ffmpegDir: string;
}

export interface CommandPlan {
    sourcePath: string;
    sourceName: string;
    outputDir: string;
    outputPath: string;
    outputName: string;
    crf: number;
    profile: string;
    level: string;
    audioMode: string;
    args: string[];
    commandLine: string;
    notes: string[];
    faststart: boolean;
}

/** 一次转码的全部选项，与 Go 侧 TranscodeOptions 对应。 */
export interface TranscodeOptions {
    sourcePath: string;
    outputDir: string;
    crf: number;
    audioMode: string;
    profile: string;
    level: string;
}

/** 下拉/分段控件的一个选项。 */
export interface OptionItem {
    value: string;
    label: string;
    hint: string;
}

/** Go 侧提供的可选项列表。 */
export interface TranscodePresets {
    crfs: OptionItem[];
    profiles: OptionItem[];
    levels: OptionItem[];
}

export interface TranscodeProgress {
    percent: number;
    processedSec: number;
    processedText: string;
    speed: string;
    fps: string;
    frame: number;
    etaText: string;
    sizeText: string;
}

export interface TranscodeSnapshot {
    running: boolean;
    status: string;
    plan: CommandPlan | null;
    percent: number;
    processedSec: number;
    processedText: string;
    speed: string;
    fps: string;
    frame: number;
    elapsedText: string;
    etaText: string;
    message: string;
    outputSizeText: string;
    startedAt: string;
    finishedAt: string;
    logTail: string[];
}
