package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	// defaultCRF 是界面默认选中的 CRF；18 / 23 / 28 为可选档位。
	defaultCRF = 23
	// defaultProfile / defaultLevel 与参考命令保持一致。
	defaultProfile = "main"
	defaultLevel   = "4.0"
	// autoLevel 表示不传 -level，由编码器按分辨率与帧率自行决定。
	autoLevel = "auto"
	// logTailLimit 是内存与界面上保留的 ffmpeg 日志行数。
	logTailLimit = 300
)

// 事件名，前端通过 window.runtime.EventsOn 订阅。
const (
	eventProgress = "transcode:progress"
	eventLog      = "transcode:log"
	eventDone     = "transcode:done"
)

var (
	// mp4Containers 是支持 -movflags +faststart 的输出容器。
	mp4Containers = map[string]bool{".mp4": true, ".m4v": true, ".mov": true}
	// mp4AudioOK 是可以直接 copy 进 MP4 的音频编码。
	mp4AudioOK = map[string]bool{"aac": true, "mp3": true, "ac3": true, "eac3": true, "alac": true}
	// allowedProfiles 是与 yuv420p 8bit 兼容、可以安全放给用户选择的 H.264 profile。
	allowedProfiles = []string{"baseline", "main", "high"}
)

// TranscodeOptions 是界面提交的一次转码选项。
type TranscodeOptions struct {
	SourcePath string `json:"sourcePath"`
	OutputDir  string `json:"outputDir"`
	CRF        int    `json:"crf"`
	AudioMode  string `json:"audioMode"` // copy | aac
	Profile    string `json:"profile"`   // baseline | main | high
	Level      string `json:"level"`     // auto | 3.1 | 4.0 | ...
}

// OptionItem 是界面下拉/分段控件的一个选项。
type OptionItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// TranscodePresets 是界面渲染选项列表的数据源（单一事实来源在 Go 侧）。
type TranscodePresets struct {
	CRFs     []OptionItem `json:"crfs"`
	Profiles []OptionItem `json:"profiles"`
	Levels   []OptionItem `json:"levels"`
}

// levelSpec 描述一个 H.264 等级的分辨率/宏块率上限，MaxMBPS 与 MaxFS 取自规范。
type levelSpec struct {
	Name    string
	MaxMBPS int
	MaxFS   int
	Hint    string
}

// levelSpecs 按从低到高排列，便于挑选满足限制的最低等级。
var levelSpecs = []levelSpec{
	{"3.1", 108000, 3600, "720p30"},
	{"4.0", 245760, 8192, "1080p30"},
	{"4.1", 245760, 8192, "1080p30 高码率"},
	{"4.2", 522240, 8704, "1080p60"},
	{"5.0", 589824, 22080, "1440p30"},
	{"5.1", 983040, 36864, "4K30"},
	{"5.2", 2073600, 36864, "4K60"},
}

// GetPresets 返回界面可选的 CRF / Profile / Level 列表。
func (a *App) GetPresets() TranscodePresets {
	presets := TranscodePresets{
		CRFs: []OptionItem{
			{Value: "18", Label: "18", Hint: "高画质 · 体积最大"},
			{Value: "23", Label: "23", Hint: "默认 · 推荐"},
			{Value: "28", Label: "28", Hint: "小体积 · 画质下降"},
		},
		Profiles: []OptionItem{
			{Value: "baseline", Label: "baseline", Hint: "兼容最老的设备，同画质体积明显更大"},
			{Value: "main", Label: "main（默认）", Hint: "兼容性与体积的平衡，参考命令使用的档位"},
			{Value: "high", Label: "high", Hint: "开启 CABAC / 8x8 变换，同画质体积更小"},
		},
		Levels: []OptionItem{
			{Value: autoLevel, Label: "自动（推荐）", Hint: "不指定 -level，由 x264 按分辨率与帧率决定"},
		},
	}
	for _, spec := range levelSpecs {
		label := spec.Name
		if spec.Name == defaultLevel {
			label += "（默认）"
		}
		presets.Levels = append(presets.Levels, OptionItem{
			Value: spec.Name,
			Label: label,
			Hint:  spec.Hint,
		})
	}
	return presets
}

// normalizeProfile 把界面传来的 profile 收敛到安全取值。
func normalizeProfile(profile string) string {
	value := strings.ToLower(strings.TrimSpace(profile))
	for _, allowed := range allowedProfiles {
		if value == allowed {
			return value
		}
	}
	return defaultProfile
}

// normalizeLevel 校验 level：留空表示使用默认档，"auto" 表示不传 -level，非法值回落到默认档。
func normalizeLevel(level string) string {
	value := strings.ToLower(strings.TrimSpace(level))
	if value == "" {
		return defaultLevel
	}
	if value == autoLevel {
		return autoLevel
	}
	for _, spec := range levelSpecs {
		if value == spec.Name {
			return spec.Name
		}
	}
	return defaultLevel
}

// levelValue 把 "4.2" 之类的等级名转成可比较的数值，用于判断高低。
func levelValue(level string) float64 {
	if level == "" || level == autoLevel {
		return 0
	}
	return parseFloat(level)
}

// recommendLevel 依据分辨率与帧率，返回满足 H.264 限制的最低等级。
func recommendLevel(width, height int, fps float64) (string, string) {
	if width <= 0 || height <= 0 || fps <= 0 {
		return "", ""
	}
	macroBlocks := ((width + 15) / 16) * ((height + 15) / 16)
	macroBlocksPerSecond := float64(macroBlocks) * fps
	for _, spec := range levelSpecs {
		if macroBlocks <= spec.MaxFS && macroBlocksPerSecond <= float64(spec.MaxMBPS) {
			return spec.Name, spec.Hint
		}
	}
	return "", ""
}

// levelHint 返回等级对应的典型分辨率说明。
func levelHint(level string) string {
	for _, spec := range levelSpecs {
		if spec.Name == level {
			return spec.Hint
		}
	}
	return ""
}

// CommandPlan 是一次转码任务的完整描述，包含最终执行的命令行。
type CommandPlan struct {
	SourcePath  string   `json:"sourcePath"`
	SourceName  string   `json:"sourceName"`
	OutputDir   string   `json:"outputDir"`
	OutputPath  string   `json:"outputPath"`
	OutputName  string   `json:"outputName"`
	CRF         int      `json:"crf"`
	Profile     string   `json:"profile"`
	Level       string   `json:"level"`
	AudioMode   string   `json:"audioMode"`
	Args        []string `json:"args"`
	CommandLine string   `json:"commandLine"`
	Notes       []string `json:"notes"`
	Faststart   bool     `json:"faststart"`
}

// TranscodeProgress 是转码过程中推送给前端的进度载荷。
type TranscodeProgress struct {
	Percent       float64 `json:"percent"`
	ProcessedSec  float64 `json:"processedSec"`
	ProcessedText string  `json:"processedText"`
	Speed         string  `json:"speed"`
	Fps           string  `json:"fps"`
	Frame         int64   `json:"frame"`
	EtaText       string  `json:"etaText"`
	SizeText      string  `json:"sizeText"`
}

// TranscodeSnapshot 是任务状态快照，既是 StartTranscode 的返回值，也用于界面刷新。
type TranscodeSnapshot struct {
	Running        bool         `json:"running"`
	Status         string       `json:"status"`
	Plan           *CommandPlan `json:"plan"`
	Percent        float64      `json:"percent"`
	ProcessedSec   float64      `json:"processedSec"`
	ProcessedText  string       `json:"processedText"`
	Speed          string       `json:"speed"`
	Fps            string       `json:"fps"`
	Frame          int64        `json:"frame"`
	ElapsedText    string       `json:"elapsedText"`
	EtaText        string       `json:"etaText"`
	Message        string       `json:"message"`
	OutputSizeText string       `json:"outputSizeText"`
	StartedAt      string       `json:"startedAt"`
	FinishedAt     string       `json:"finishedAt"`
	LogTail        []string     `json:"logTail"`
}

type transcodeJob struct {
	mu sync.Mutex

	plan      CommandPlan
	status    string
	percent   float64
	processed float64
	duration  float64
	speed     string
	fps       string
	frame     int64
	sizeBytes int64
	message   string
	logs      []string
	startedAt time.Time
	endedAt   time.Time

	// wailsCtx 用于向前端推送日志事件。
	wailsCtx context.Context
	cancel   context.CancelFunc
}

// ---- 命令行构建 ----

// PlanTranscode 依据选项生成最终要执行的 ffmpeg 命令，界面用它做命令预览。
func (a *App) PlanTranscode(opts TranscodeOptions) (CommandPlan, error) {
	plan, err := buildPlan(opts)
	if err != nil {
		return CommandPlan{}, err
	}
	return *plan, nil
}

// buildPlan 组装与参考命令一致的 ffmpeg 参数，只在必要时做可见的调整。
func buildPlan(opts TranscodeOptions) (*CommandPlan, error) {
	src := strings.TrimSpace(opts.SourcePath)
	if src == "" {
		return nil, errors.New("请先选择要转码的视频文件")
	}
	absSrc, err := filepath.Abs(src)
	if err != nil {
		absSrc = src
	}
	st, err := os.Stat(absSrc)
	if err != nil {
		return nil, fmt.Errorf("无法读取源文件：%s", absSrc)
	}
	if st.IsDir() {
		return nil, errors.New("选择的路径是文件夹，请选择视频文件")
	}

	crf := opts.CRF
	if crf < 0 || crf > 51 {
		crf = defaultCRF
	}
	audioMode := strings.ToLower(strings.TrimSpace(opts.AudioMode))
	if audioMode != "aac" {
		audioMode = "copy"
	}
	profile := normalizeProfile(opts.Profile)
	level := normalizeLevel(opts.Level)

	if strings.TrimSpace(opts.OutputDir) == "" {
		opts.OutputDir = filepath.Join(filepath.Dir(absSrc), "output")
	}
	absOutDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		absOutDir = opts.OutputDir
	}

	srcExt := strings.ToLower(filepath.Ext(absSrc))
	base := strings.TrimSuffix(filepath.Base(absSrc), filepath.Ext(absSrc))

	notes := make([]string, 0, 4)
	outExt := srcExt
	if !mp4Containers[srcExt] {
		outExt = ".mp4"
		notes = append(notes, fmt.Sprintf(
			"源容器为 %s，输出改用 .mp4，才能使用 -movflags +faststart。",
			strings.TrimPrefix(srcExt, ".")))
	}
	outName := base + outExt
	outPath := filepath.Join(absOutDir, outName)

	if samePath(absSrc, outPath) {
		return nil, errors.New("输出路径与源文件相同，请改用其他输出目录，以免覆盖原始视频")
	}

	faststart := mp4Containers[outExt]

	// 探测源文件，用于按需修正参数，并校验 profile / level 选择是否合理。
	dropSubtitles := false
	if info, err := probeMedia(context.Background(), absSrc); err == nil {
		if audioMode == "copy" && info.AudioTracks > 0 && !info.AudioCompatible {
			audioMode = "aac"
			notes = append(notes, fmt.Sprintf(
				"音频编码 %s 无法 copy 进 MP4，已自动改为 AAC 128k。",
				strings.ToUpper(strings.Join(info.AudioCodecs, ", "))))
		}
		if len(info.BitmapSubtitles) > 0 {
			dropSubtitles = true
			notes = append(notes, fmt.Sprintf(
				"源文件含图形字幕（%s），MP4 无法承载，已加 -sn 丢弃。",
				strings.ToUpper(strings.Join(info.BitmapSubtitles, ", "))))
		}
		if info.Video != nil && info.Video.RecommendedLevel != "" {
			recommended := levelValue(info.Video.RecommendedLevel)
			if level == autoLevel {
				notes = append(notes, fmt.Sprintf(
					"Level 自动：按 %s / %s 预计使用 level %s。",
					info.Video.Resolution, info.Video.FrameRate, info.Video.RecommendedLevel))
			} else if levelValue(level) < recommended {
				notes = append(notes, fmt.Sprintf(
					"所选 Level %s 低于该视频所需（按 %s / %s 需要 level %s），ffmpeg 可能报错，建议改为 level %s 或「自动」。",
					level, info.Video.Resolution, info.Video.FrameRate,
					info.Video.RecommendedLevel, info.Video.RecommendedLevel))
			}
		}
	}

	if profile != defaultProfile {
		notes = append(notes, fmt.Sprintf(
			"Profile 由 %s 改为 %s：%s。", defaultProfile, profile, profileHint(profile)))
	}
	if level != defaultLevel && level != autoLevel {
		notes = append(notes, fmt.Sprintf(
			"Level 由 %s 改为 %s（%s）。", defaultLevel, level, levelHint(level)))
	}

	args := []string{
		"-hide_banner",
		"-y",
		"-i", absSrc,
		"-c:v", "libx264",
		"-profile:v", profile,
	}
	if level != autoLevel {
		args = append(args, "-level", level)
	}
	args = append(args,
		"-pix_fmt", "yuv420p",
		"-crf", strconv.Itoa(crf),
	)
	if audioMode == "aac" {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	} else {
		args = append(args, "-c:a", "copy")
	}
	if dropSubtitles {
		args = append(args, "-sn")
	}
	if faststart {
		args = append(args, "-movflags", "+faststart")
	}
	// -progress pipe:1 提供机器可读的进度，-nostats 抑制 stderr 的刷屏统计。
	args = append(args, "-progress", "pipe:1", "-nostats", outPath)

	return &CommandPlan{
		SourcePath:  absSrc,
		SourceName:  filepath.Base(absSrc),
		OutputDir:   absOutDir,
		OutputPath:  outPath,
		OutputName:  outName,
		CRF:         crf,
		Profile:     profile,
		Level:       level,
		AudioMode:   audioMode,
		Args:        args,
		CommandLine: "ffmpeg " + quoteArgs(args),
		Notes:       notes,
		Faststart:   faststart,
	}, nil
}

// profileHint 返回 profile 的说明，用于界面提示。
func profileHint(profile string) string {
	switch profile {
	case "baseline":
		return "兼容性最好（无 CABAC / B 帧），同画质体积明显更大"
	case "high":
		return "开启 CABAC 与 8x8 变换，同画质体积更小，但部分老设备不支持"
	case "main":
		return "兼容性与体积的平衡"
	}
	return ""
}

// ---- 任务控制 ----

// StartTranscode 启动一次后台转码任务，进度通过 transcode:progress 事件推送。
func (a *App) StartTranscode(opts TranscodeOptions) (TranscodeSnapshot, error) {
	a.jobMu.Lock()
	if a.job != nil && a.job.isRunning() {
		a.jobMu.Unlock()
		return TranscodeSnapshot{}, errors.New("已有转码任务正在进行，请先取消或等待其完成")
	}

	plan, err := buildPlan(opts)
	if err != nil {
		a.jobMu.Unlock()
		return TranscodeSnapshot{}, err
	}

	ffmpegPath := locateTool("ffmpeg")
	if ffmpegPath == "" {
		a.jobMu.Unlock()
		return TranscodeSnapshot{}, errors.New("未找到 ffmpeg，请安装 FFmpeg 并将 bin 目录加入 PATH，或设置 FFMPEG_PATH 环境变量")
	}

	if err := os.MkdirAll(plan.OutputDir, 0o755); err != nil {
		a.jobMu.Unlock()
		return TranscodeSnapshot{}, fmt.Errorf("创建输出目录失败：%w", err)
	}

	// 总时长用于计算百分比与剩余时间；读取失败时进度以不确定模式显示。
	duration := 0.0
	if info, err := probeMedia(context.Background(), plan.SourcePath); err == nil {
		duration = info.DurationSec
	}

	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)

	job := &transcodeJob{
		plan:      *plan,
		status:    "running",
		duration:  duration,
		startedAt: time.Now(),
		wailsCtx:  a.ctx,
		cancel:    cancel,
		logs:      make([]string, 0, logTailLimit),
	}
	a.job = job
	a.jobMu.Unlock()

	go a.runJob(ctx, cancel, job, ffmpegPath)

	return job.snapshot(), nil
}

// CancelTranscode 终止正在进行的转码任务。
func (a *App) CancelTranscode() error {
	a.jobMu.Lock()
	job := a.job
	a.jobMu.Unlock()
	if job == nil || !job.isRunning() {
		return nil
	}
	job.appendLog("== 收到取消请求，正在终止 ffmpeg ==")
	job.cancel()
	return nil
}

// GetTranscodeStatus 返回当前任务状态，用于界面初始化时恢复显示。
func (a *App) GetTranscodeStatus() TranscodeSnapshot {
	a.jobMu.Lock()
	job := a.job
	a.jobMu.Unlock()
	if job == nil {
		return TranscodeSnapshot{Status: "idle", Percent: 0, LogTail: []string{}}
	}
	return job.snapshot()
}

func (a *App) runJob(ctx context.Context, cancel context.CancelFunc, job *transcodeJob, ffmpegPath string) {
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, job.plan.Args...)
	cmd.Dir = job.plan.OutputDir
	cmd.SysProcAttr = hideWindow()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.finish(job, fmt.Errorf("无法读取 ffmpeg 输出：%w", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.finish(job, fmt.Errorf("无法读取 ffmpeg 日志：%w", err))
		return
	}

	job.appendLog("$ " + job.plan.CommandLine)

	if err := cmd.Start(); err != nil {
		a.finish(job, fmt.Errorf("启动 ffmpeg 失败：%w", err))
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		job.readProgress(stdout, a)
	}()
	go func() {
		defer wg.Done()
		job.readLog(stderr, a)
	}()

	// 必须先读完两个管道再 Wait，否则 Wait 会提前关闭管道、丢失日志。
	wg.Wait()
	waitErr := cmd.Wait()

	switch {
	case ctx.Err() != nil:
		job.setStatus("canceled")
		job.removePartialOutput()
		a.finish(job, errors.New("转码已取消，未完成的输出文件已删除"))
	case waitErr != nil:
		job.removePartialOutput()
		a.finish(job, fmt.Errorf("ffmpeg 退出异常：%s%s", waitErr.Error(), job.lastErrorLine()))
	default:
		st, statErr := os.Stat(job.plan.OutputPath)
		if statErr != nil {
			a.finish(job, fmt.Errorf("ffmpeg 已结束但未生成输出文件：%s", job.plan.OutputPath))
			return
		}
		job.setSize(st.Size())
		job.setPercent(100)
		job.setStatus("completed")
		job.appendLog(fmt.Sprintf("== 转码完成：%s（%s）", job.plan.OutputPath, formatBytes(st.Size())))
		a.finish(job, nil)
	}
}

// finish 统一收尾：更新状态并广播完成事件。runErr 为 nil 表示成功。
func (a *App) finish(job *transcodeJob, runErr error) {
	if runErr != nil {
		if job.getStatus() != "canceled" {
			job.setStatus("failed")
		}
		job.setMessage(runErr.Error())
		job.appendLog("== 失败：" + runErr.Error())
	}
	snapshot := job.snapshot()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, eventDone, snapshot)
	}
}

// ---- job 状态读写 ----

func (j *transcodeJob) isRunning() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status == "running"
}

func (j *transcodeJob) getStatus() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

func (j *transcodeJob) setStatus(s string) {
	j.mu.Lock()
	j.status = s
	if s != "running" {
		j.endedAt = time.Now()
	}
	j.mu.Unlock()
}

func (j *transcodeJob) setMessage(msg string) {
	j.mu.Lock()
	j.message = msg
	j.mu.Unlock()
}

func (j *transcodeJob) setPercent(p float64) {
	j.mu.Lock()
	j.percent = p
	j.mu.Unlock()
}

func (j *transcodeJob) setSize(n int64) {
	j.mu.Lock()
	j.sizeBytes = n
	j.mu.Unlock()
}

func (j *transcodeJob) appendLog(line string) {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return
	}
	j.mu.Lock()
	if len(j.logs) >= logTailLimit {
		j.logs = append(j.logs[:0], j.logs[1:]...)
	}
	j.logs = append(j.logs, line)
	ctx := j.wailsCtx
	j.mu.Unlock()

	if ctx != nil {
		runtime.EventsEmit(ctx, eventLog, line)
	}
}

// lastErrorLine 从日志尾部挑一条最像错误的信息，附在失败消息后面。
func (j *transcodeJob) lastErrorLine() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := len(j.logs) - 1; i >= 0; i-- {
		lower := strings.ToLower(j.logs[i])
		if strings.Contains(lower, "error") || strings.Contains(lower, "invalid") ||
			strings.Contains(lower, "failed") || strings.Contains(lower, "unable") {
			return "。最后一条错误：" + j.logs[i]
		}
	}
	if len(j.logs) > 0 {
		return "。最后一条日志：" + j.logs[len(j.logs)-1]
	}
	return ""
}

func (j *transcodeJob) removePartialOutput() {
	if j.plan.OutputPath == "" {
		return
	}
	if err := os.Remove(j.plan.OutputPath); err == nil {
		j.appendLog("== 已删除未完成的输出文件：" + j.plan.OutputPath)
	}
}

func (j *transcodeJob) snapshot() TranscodeSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()

	snap := TranscodeSnapshot{
		Running:        j.status == "running",
		Status:         j.status,
		Plan:           &j.plan,
		Percent:        j.percent,
		ProcessedSec:   j.processed,
		ProcessedText:  formatDuration(j.processed),
		Speed:          j.speed,
		Fps:            j.fps,
		Frame:          j.frame,
		Message:        j.message,
		OutputSizeText: formatBytes(j.sizeBytes),
		LogTail:        append([]string(nil), j.logs...),
	}
	if !j.startedAt.IsZero() {
		snap.StartedAt = j.startedAt.Format("15:04:05")
	}
	if !j.endedAt.IsZero() {
		snap.FinishedAt = j.endedAt.Format("15:04:05")
		snap.ElapsedText = formatDuration(j.endedAt.Sub(j.startedAt).Seconds())
	} else if !j.startedAt.IsZero() {
		snap.ElapsedText = formatDuration(time.Since(j.startedAt).Seconds())
	}
	if j.duration > 0 && j.processed > 0 {
		snap.EtaText = estimateEta(j.duration, j.processed, j.speed)
	}
	return snap
}

// ---- ffmpeg 输出解析 ----

// readProgress 解析 -progress pipe:1 输出的 key=value 流。
func (j *transcodeJob) readProgress(r io.Reader, a *App) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok {
			continue
		}
		switch key {
		case "frame":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				j.mu.Lock()
				j.frame = v
				j.mu.Unlock()
			}
		case "fps":
			j.mu.Lock()
			j.fps = strings.TrimSpace(value)
			j.mu.Unlock()
		case "speed":
			j.mu.Lock()
			j.speed = strings.TrimSpace(value)
			j.mu.Unlock()
		case "out_time_us", "out_time_ms":
			// 注意：ffmpeg 的 out_time_us 与 out_time_ms 都是微秒。
			if v, err := strconv.ParseFloat(value, 64); err == nil && v >= 0 {
				j.mu.Lock()
				j.processed = v / 1_000_000
				j.mu.Unlock()
			}
		case "out_time":
			if d, ok := parseClock(value); ok {
				j.mu.Lock()
				j.processed = d
				j.mu.Unlock()
			}
		case "total_size":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
				j.setSize(v)
			}
		case "progress":
			j.emitProgress(a)
		}
	}
}

// readLog 收集 ffmpeg 的 stderr（-nostats 之后主要是警告与错误）。
func (j *transcodeJob) readLog(r io.Reader, _ *App) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		j.appendLog(scanner.Text())
	}
}

func (j *transcodeJob) emitProgress(a *App) {
	j.mu.Lock()
	percent := -1.0
	if j.duration > 0 {
		percent = j.processed / j.duration * 100
		if percent > 99.9 {
			percent = 99.9 // 100% 留给任务真正结束
		}
		if percent < 0 {
			percent = 0
		}
	}
	j.percent = percent
	duration := j.duration
	payload := TranscodeProgress{
		Percent:       percent,
		ProcessedSec:  j.processed,
		ProcessedText: formatDuration(j.processed),
		Speed:         j.speed,
		Fps:           j.fps,
		Frame:         j.frame,
		SizeText:      formatBytes(j.sizeBytes),
		EtaText:       estimateEta(duration, j.processed, j.speed),
	}
	j.mu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, eventProgress, payload)
	}
}

// ---- 工具函数 ----

// estimateEta 依据 ffmpeg 报告的 speed 估算剩余时间。
func estimateEta(duration, processed float64, speed string) string {
	if duration <= 0 || processed <= 0 {
		return ""
	}
	multiplier := parseFloat(strings.TrimSuffix(strings.TrimSpace(speed), "x"))
	if multiplier <= 0.01 {
		return ""
	}
	remaining := (duration - processed) / multiplier
	if remaining <= 0 {
		return ""
	}
	if remaining < 60 {
		return fmt.Sprintf("剩余约 %.0f 秒", remaining)
	}
	return fmt.Sprintf("剩余约 %.1f 分钟", remaining/60)
}

func parseClock(s string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 3 {
		return 0, false
	}
	return parseFloat(parts[0])*3600 + parseFloat(parts[1])*60 + parseFloat(parts[2]), true
}

func quoteArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, quoteArg(arg))
	}
	return strings.Join(quoted, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
}

func samePath(a, b string) bool {
	ca, err1 := filepath.Abs(a)
	cb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(filepath.Clean(ca), filepath.Clean(cb))
}

// locateTool 查找 ffmpeg / ffprobe。查找顺序：
//  1. 环境变量 FFMPEG_PATH / FFPROBE_PATH
//  2. 随应用分发的 bin 目录（可执行文件同级或上级、当前工作目录）
//  3. 系统 PATH
//
// 同目录下的兄弟可执行文件会作为最后的兜底，避免只装了一个工具时完全不可用。
func locateTool(name string) string {
	exeNames := []string{name}
	if goruntime.GOOS == "windows" {
		exeNames = append(exeNames, name+".exe")
	}

	if custom := strings.TrimSpace(os.Getenv(strings.ToUpper(name) + "_PATH")); custom != "" {
		if st, err := os.Stat(custom); err == nil && !st.IsDir() {
			return custom
		}
	}

	for _, dir := range localToolDirs() {
		for _, exe := range exeNames {
			candidate := filepath.Join(dir, exe)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate
			}
		}
	}

	if found, err := exec.LookPath(name); err == nil {
		return found
	}

	other := "ffmpeg"
	if name == "ffmpeg" {
		other = "ffprobe"
	}
	if sibling, err := exec.LookPath(other); err == nil {
		candidate := filepath.Join(filepath.Dir(sibling), name+".exe")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

// localToolDirs 返回优先查找的本地目录：可执行文件所在目录及其 bin 子目录、当前工作目录及其 bin 子目录。
func localToolDirs() []string {
	dirs := make([]string, 0, 4)
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(base, "bin"), base)
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, "bin"), wd)
	}
	return dirs
}
