export namespace main {
	
	export class AppSettings {
	    ffmpegDir: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpegDir = source["ffmpegDir"];
	    }
	}
	export class AudioInfo {
	    index: number;
	    codec: string;
	    codecLong: string;
	    profile: string;
	    sampleRate: string;
	    channels: number;
	    channelLayout: string;
	    bitRateText: string;
	    sampleFormat: string;
	
	    static createFrom(source: any = {}) {
	        return new AudioInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.codec = source["codec"];
	        this.codecLong = source["codecLong"];
	        this.profile = source["profile"];
	        this.sampleRate = source["sampleRate"];
	        this.channels = source["channels"];
	        this.channelLayout = source["channelLayout"];
	        this.bitRateText = source["bitRateText"];
	        this.sampleFormat = source["sampleFormat"];
	    }
	}
	export class CommandPlan {
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
	
	    static createFrom(source: any = {}) {
	        return new CommandPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.sourceName = source["sourceName"];
	        this.outputDir = source["outputDir"];
	        this.outputPath = source["outputPath"];
	        this.outputName = source["outputName"];
	        this.crf = source["crf"];
	        this.profile = source["profile"];
	        this.level = source["level"];
	        this.audioMode = source["audioMode"];
	        this.args = source["args"];
	        this.commandLine = source["commandLine"];
	        this.notes = source["notes"];
	        this.faststart = source["faststart"];
	    }
	}
	export class DetectSource {
	    kind: string;
	    label: string;
	    detail: string;
	    found: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new DetectSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.label = source["label"];
	        this.detail = source["detail"];
	        this.found = source["found"];
	        this.note = source["note"];
	    }
	}
	export class VideoInfo {
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
	    recommendedLevel: string;
	    recommendedLevelHint: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.codec = source["codec"];
	        this.codecLong = source["codecLong"];
	        this.profile = source["profile"];
	        this.level = source["level"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.resolution = source["resolution"];
	        this.aspectRatio = source["aspectRatio"];
	        this.pixelFormat = source["pixelFormat"];
	        this.bitDepth = source["bitDepth"];
	        this.frameRate = source["frameRate"];
	        this.bitRateText = source["bitRateText"];
	        this.colorInfo = source["colorInfo"];
	        this.scanType = source["scanType"];
	        this.durationText = source["durationText"];
	        this.recommendedLevel = source["recommendedLevel"];
	        this.recommendedLevelHint = source["recommendedLevelHint"];
	    }
	}
	export class MediaInfo {
	    path: string;
	    fileName: string;
	    sizeBytes: number;
	    sizeText: string;
	    durationSec: number;
	    durationText: string;
	    container: string;
	    containerLong: string;
	    bitRateText: string;
	    video?: VideoInfo;
	    audio?: AudioInfo;
	    audioTracks: number;
	    subtitleTracks: number;
	    warnings: string[];
	    audioCodecs: string[];
	    audioCompatible: boolean;
	    bitmapSubtitles: string[];
	
	    static createFrom(source: any = {}) {
	        return new MediaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.fileName = source["fileName"];
	        this.sizeBytes = source["sizeBytes"];
	        this.sizeText = source["sizeText"];
	        this.durationSec = source["durationSec"];
	        this.durationText = source["durationText"];
	        this.container = source["container"];
	        this.containerLong = source["containerLong"];
	        this.bitRateText = source["bitRateText"];
	        this.video = this.convertValues(source["video"], VideoInfo);
	        this.audio = this.convertValues(source["audio"], AudioInfo);
	        this.audioTracks = source["audioTracks"];
	        this.subtitleTracks = source["subtitleTracks"];
	        this.warnings = source["warnings"];
	        this.audioCodecs = source["audioCodecs"];
	        this.audioCompatible = source["audioCompatible"];
	        this.bitmapSubtitles = source["bitmapSubtitles"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OptionItem {
	    value: string;
	    label: string;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new OptionItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	        this.hint = source["hint"];
	    }
	}
	export class ToolDetection {
	    available: boolean;
	    ffmpegPath: string;
	    ffprobePath: string;
	    ffmpegVersion: string;
	    message: string;
	    sources: DetectSource[];
	    binDir: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolDetection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.ffprobePath = source["ffprobePath"];
	        this.ffmpegVersion = source["ffmpegVersion"];
	        this.message = source["message"];
	        this.sources = this.convertValues(source["sources"], DetectSource);
	        this.binDir = source["binDir"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TranscodeOptions {
	    sourcePath: string;
	    outputDir: string;
	    crf: number;
	    audioMode: string;
	    profile: string;
	    level: string;
	
	    static createFrom(source: any = {}) {
	        return new TranscodeOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.outputDir = source["outputDir"];
	        this.crf = source["crf"];
	        this.audioMode = source["audioMode"];
	        this.profile = source["profile"];
	        this.level = source["level"];
	    }
	}
	export class TranscodePresets {
	    crfs: OptionItem[];
	    profiles: OptionItem[];
	    levels: OptionItem[];
	
	    static createFrom(source: any = {}) {
	        return new TranscodePresets(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.crfs = this.convertValues(source["crfs"], OptionItem);
	        this.profiles = this.convertValues(source["profiles"], OptionItem);
	        this.levels = this.convertValues(source["levels"], OptionItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TranscodeSnapshot {
	    running: boolean;
	    status: string;
	    plan?: CommandPlan;
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
	
	    static createFrom(source: any = {}) {
	        return new TranscodeSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.status = source["status"];
	        this.plan = this.convertValues(source["plan"], CommandPlan);
	        this.percent = source["percent"];
	        this.processedSec = source["processedSec"];
	        this.processedText = source["processedText"];
	        this.speed = source["speed"];
	        this.fps = source["fps"];
	        this.frame = source["frame"];
	        this.elapsedText = source["elapsedText"];
	        this.etaText = source["etaText"];
	        this.message = source["message"];
	        this.outputSizeText = source["outputSizeText"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.logTail = source["logTail"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

