package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 这些测试需要 PATH 上有 ffmpeg / ffprobe（或项目 bin/ 目录）；缺失时自动跳过。

func requireFFmpeg(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("short 模式跳过依赖 ffmpeg 的测试")
	}
	path := locateTool("ffmpeg")
	if path == "" {
		t.Skip("未找到 ffmpeg，跳过测试")
	}
	if locateTool("ffprobe") == "" {
		t.Skip("未找到 ffprobe，跳过测试")
	}
	return path
}

// makeSampleVideo 生成一段 3 秒的 H.264 + AAC 测试视频。
func makeSampleVideo(t *testing.T, dir, name string) string {
	t.Helper()
	return makeVideo(t, dir, name, "320x240", "25", "3")
}

// makeVideo 按指定分辨率/帧率/时长生成测试视频。
func makeVideo(t *testing.T, dir, name, size, rate, duration string) string {
	t.Helper()
	ffmpeg := requireFFmpeg(t)
	out := filepath.Join(dir, name)

	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%s:rate=%s:duration=%s", size, rate, duration),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:duration=%s", duration),
		"-c:v", "libx264", "-profile:v", "main", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		"-shortest", out,
	)
	cmd.SysProcAttr = hideWindow()
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("生成测试视频失败：%v\n%s", err, combined)
	}
	return out
}

func TestProbeMediaReportsEncodingInfo(t *testing.T) {
	dir := t.TempDir()
	sample := makeSampleVideo(t, dir, "sample.mp4")

	info, err := probeMedia(context.Background(), sample)
	if err != nil {
		t.Fatalf("probeMedia 失败：%v", err)
	}

	if info.Video == nil {
		t.Fatal("未解析出视频流")
	}
	if info.Video.Codec != "H264" {
		t.Errorf("视频编码 = %q，期望 H264", info.Video.Codec)
	}
	if info.Video.Width != 320 || info.Video.Height != 240 {
		t.Errorf("分辨率 = %dx%d，期望 320x240", info.Video.Width, info.Video.Height)
	}
	if info.Video.PixelFormat != "yuv420p" {
		t.Errorf("像素格式 = %q，期望 yuv420p", info.Video.PixelFormat)
	}
	if !strings.Contains(info.Video.FrameRate, "25") {
		t.Errorf("帧率 = %q，期望包含 25", info.Video.FrameRate)
	}
	if info.Video.RecommendedLevel != "3.1" {
		t.Errorf("320x240@25 的建议等级 = %q，期望 3.1", info.Video.RecommendedLevel)
	}
	if info.Audio == nil {
		t.Fatal("未解析出音频流")
	}
	if info.Audio.Codec != "AAC" {
		t.Errorf("音频编码 = %q，期望 AAC", info.Audio.Codec)
	}
	if !info.AudioCompatible {
		t.Error("AAC 应被判为与 MP4 兼容")
	}
	if info.DurationSec < 2.5 || info.DurationSec > 3.5 {
		t.Errorf("时长 = %.2fs，期望约 3s", info.DurationSec)
	}
	if info.SizeText == "" || info.DurationText == "" {
		t.Error("大小/时长的可读文本不应为空")
	}
}

func TestBuildPlanMatchesReferenceCommand(t *testing.T) {
	dir := t.TempDir()
	sample := makeSampleVideo(t, dir, "第六集 一张照片的秘密（照片隐私篇）.mp4")

	plan, err := buildPlan(TranscodeOptions{
		SourcePath: sample,
		CRF:        18,
		AudioMode:  "copy",
		Profile:    defaultProfile,
		Level:      defaultLevel,
	})
	if err != nil {
		t.Fatalf("buildPlan 失败：%v", err)
	}

	for _, want := range []string{
		"-c:v", "libx264", "-profile:v", "main", "-level", "4.0",
		"-pix_fmt", "yuv420p", "-crf", "18", "-c:a", "copy",
		"-movflags", "+faststart",
	} {
		if !containsArg(plan.Args, want) {
			t.Errorf("命令缺少参数 %q：%s", want, plan.CommandLine)
		}
	}

	if plan.OutputDir != filepath.Join(filepath.Dir(sample), "output") {
		t.Errorf("默认输出目录 = %q，期望源目录下的 output", plan.OutputDir)
	}
	if plan.OutputName != "第六集 一张照片的秘密（照片隐私篇）.mp4" {
		t.Errorf("输出文件名 = %q", plan.OutputName)
	}
	if !strings.Contains(plan.CommandLine, `"`+sample+`"`) {
		t.Errorf("含空格/中文的路径应被引号包裹：%s", plan.CommandLine)
	}
	if !plan.Faststart {
		t.Error("mp4 输出应启用 faststart")
	}
	if plan.Profile != defaultProfile || plan.Level != defaultLevel {
		t.Errorf("默认档位 = %s / %s，期望 %s / %s", plan.Profile, plan.Level, defaultProfile, defaultLevel)
	}
}

// TestBuildPlanLevelAutoOmitsLevelOption 验证「自动」时不传 -level，交给 x264 自行决定。
func TestBuildPlanLevelAutoOmitsLevelOption(t *testing.T) {
	dir := t.TempDir()
	sample := makeSampleVideo(t, dir, "sample.mp4")

	plan, err := buildPlan(TranscodeOptions{
		SourcePath: sample,
		CRF:        defaultCRF,
		AudioMode:  "copy",
		Profile:    "main",
		Level:      autoLevel,
	})
	if err != nil {
		t.Fatalf("buildPlan 失败：%v", err)
	}
	if containsArg(plan.Args, "-level") {
		t.Errorf("「自动」不应写出 -level：%s", plan.CommandLine)
	}
	if plan.Level != autoLevel {
		t.Errorf("plan.Level = %q，期望 %q", plan.Level, autoLevel)
	}
}

// TestBuildPlanSanitizesProfileAndLevel 验证非法取值会被收敛到安全档位。
func TestBuildPlanSanitizesProfileAndLevel(t *testing.T) {
	dir := t.TempDir()
	sample := makeSampleVideo(t, dir, "sample.mp4")

	for _, opts := range []TranscodeOptions{
		{SourcePath: sample, CRF: 23, Profile: "high10", Level: "9.9"},
		{SourcePath: sample, CRF: 23, Profile: "", Level: ""},
	} {
		plan, err := buildPlan(opts)
		if err != nil {
			t.Fatalf("buildPlan 失败：%v", err)
		}
		if plan.Profile != defaultProfile {
			t.Errorf("profile %q 应回落到 %q，实际 %q", opts.Profile, defaultProfile, plan.Profile)
		}
		if plan.Level != defaultLevel {
			t.Errorf("level %q 应回落到 %q，实际 %q", opts.Level, defaultLevel, plan.Level)
		}
	}

	// high profile 与 4.1 是合法选择，应被保留。
	plan, err := buildPlan(TranscodeOptions{SourcePath: sample, CRF: 23, Profile: "high", Level: "4.1"})
	if err != nil {
		t.Fatalf("buildPlan 失败：%v", err)
	}
	if plan.Profile != "high" || plan.Level != "4.1" {
		t.Errorf("应保留 high / 4.1，实际 %s / %s", plan.Profile, plan.Level)
	}
	if !containsArg(plan.Args, "high") || !containsArg(plan.Args, "4.1") {
		t.Errorf("命令未包含所选档位：%s", plan.CommandLine)
	}
}

// TestRecommendLevel 覆盖常见分辨率/帧率组合的等级推算。
func TestRecommendLevel(t *testing.T) {
	cases := []struct {
		width, height int
		fps           float64
		want          string
	}{
		{640, 480, 30, "3.1"},
		{1280, 720, 30, "3.1"},
		{1920, 1080, 23.976, "4.0"},
		{1920, 1080, 30, "4.0"},
		{1920, 1080, 60, "4.2"},
		{3840, 2160, 30, "5.1"},
		{3840, 2160, 60, "5.2"},
		{7680, 4320, 60, ""},
	}
	for _, c := range cases {
		got, hint := recommendLevel(c.width, c.height, c.fps)
		if got != c.want {
			t.Errorf("recommendLevel(%d, %d, %.3f) = %q，期望 %q", c.width, c.height, c.fps, got, c.want)
			continue
		}
		if got != "" && hint == "" {
			t.Errorf("recommendLevel(%d, %d, %.3f) 缺少说明文本", c.width, c.height, c.fps)
		}
	}
}

// TestBuildPlanNotesLevelTooLow 验证所选 level 不足以覆盖源分辨率/帧率时会给出提示。
func TestBuildPlanNotesLevelTooLow(t *testing.T) {
	dir := t.TempDir()
	sample := makeVideo(t, dir, "hd60.mp4", "1920x1080", "60", "1")

	plan, err := buildPlan(TranscodeOptions{
		SourcePath: sample,
		CRF:        23,
		AudioMode:  "copy",
		Profile:    "main",
		Level:      "4.0",
	})
	if err != nil {
		t.Fatalf("buildPlan 失败：%v", err)
	}
	if !containsNote(plan.Notes, "低于该视频所需") {
		t.Errorf("应提示 level 过低，实际说明：%v", plan.Notes)
	}
	if !containsNote(plan.Notes, "4.2") {
		t.Errorf("说明中应给出建议等级 4.2，实际：%v", plan.Notes)
	}

	// 换成足够高的等级后不应再提示。
	plan, err = buildPlan(TranscodeOptions{
		SourcePath: sample, CRF: 23, AudioMode: "copy", Profile: "main", Level: "4.2",
	})
	if err != nil {
		t.Fatalf("buildPlan 失败：%v", err)
	}
	if containsNote(plan.Notes, "低于该视频所需") {
		t.Errorf("level 4.2 足够，不应提示过低：%v", plan.Notes)
	}
}

func TestBuildPlanDefaultsToCRF23(t *testing.T) {
	dir := t.TempDir()
	sample := makeSampleVideo(t, dir, "sample.mp4")

	plan, err := buildPlan(TranscodeOptions{SourcePath: sample, CRF: defaultCRF, AudioMode: "copy"})
	if err != nil {
		t.Fatalf("buildPlan 失败：%v", err)
	}
	if plan.CRF != 23 {
		t.Errorf("默认 CRF = %d，期望 23", plan.CRF)
	}
}

func TestBuildPlanRejectsSourceOverwrite(t *testing.T) {
	dir := t.TempDir()
	sample := makeSampleVideo(t, dir, "sample.mp4")

	if _, err := buildPlan(TranscodeOptions{SourcePath: sample, OutputDir: dir, CRF: 23, AudioMode: "copy"}); err == nil {
		t.Fatal("输出目录等于源目录时应报错，避免覆盖原始文件")
	}
}

func TestTranscodeEndToEnd(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	// 直接使用含中文、全角括号与空格的真实文件名，验证 ffmpeg 的路径处理。
	name := "第六集 一张照片的秘密（照片隐私篇）.mp4"
	sample := makeSampleVideo(t, dir, name)
	outDir := filepath.Join(dir, "output")

	app := NewApp()
	snapshot, err := app.StartTranscode(TranscodeOptions{
		SourcePath: sample,
		OutputDir:  outDir,
		CRF:        23,
		AudioMode:  "copy",
		Profile:    defaultProfile,
		Level:      defaultLevel,
	})
	if err != nil {
		t.Fatalf("StartTranscode 失败：%v", err)
	}
	if !snapshot.Running {
		t.Fatal("任务应处于运行状态")
	}

	final := waitForJob(t, app, 3*time.Minute)
	if final.Status != "completed" {
		t.Fatalf("任务状态 = %q，消息 = %q", final.Status, final.Message)
	}
	if final.Percent != 100 {
		t.Errorf("完成时百分比 = %v，期望 100", final.Percent)
	}

	outPath := filepath.Join(outDir, name)
	if final.Plan == nil || final.Plan.OutputPath != outPath {
		t.Fatalf("输出路径 = %+v，期望 %s", final.Plan, outPath)
	}
	st, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("输出文件不存在：%v", err)
	}
	if st.Size() == 0 {
		t.Fatal("输出文件为空")
	}
	if final.OutputSizeText == "" {
		t.Error("快照应包含输出大小")
	}

	info, err := probeMedia(context.Background(), outPath)
	if err != nil {
		t.Fatalf("探测输出文件失败：%v", err)
	}
	if info.Video == nil || info.Video.Codec != "H264" || info.Video.PixelFormat != "yuv420p" {
		t.Errorf("输出视频参数不符：%+v", info.Video)
	}
	if info.Audio == nil || info.Audio.Codec != "AAC" {
		t.Errorf("音频应原样复制（AAC）：%+v", info.Audio)
	}

	// +faststart 会把 moov 原子写到 mdat 之前。
	if !moovBeforeMdat(t, outPath) {
		t.Error("未检测到 faststart：moov 应在 mdat 之前")
	}
}

func TestTranscodeCancelRemovesPartialFile(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	dir := t.TempDir()

	// 用一段较长的高清视频保证有足够时间观察进度并在中途取消。
	src := filepath.Join(dir, "long.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=30:duration=40",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", src,
	)
	cmd.SysProcAttr = hideWindow()
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("生成测试视频失败：%v\n%s", err, combined)
	}

	outDir := filepath.Join(dir, "output")
	app := NewApp()
	if _, err := app.StartTranscode(TranscodeOptions{
		SourcePath: src,
		OutputDir:  outDir,
		CRF:        18,
		AudioMode:  "copy",
		Profile:    defaultProfile,
		Level:      defaultLevel,
	}); err != nil {
		t.Fatalf("StartTranscode 失败：%v", err)
	}

	// 轮询期间应能看到解析自 -progress 的中间进度与速度，看到后立即取消。
	sawProgress := false
	sawSpeed := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		snapshot := app.GetTranscodeStatus()
		if !snapshot.Running {
			break
		}
		if snapshot.Percent > 0 && snapshot.Percent < 100 {
			sawProgress = true
		}
		if snapshot.Speed != "" {
			sawSpeed = true
		}
		if sawProgress {
			break
		}
	}
	if !sawProgress {
		t.Error("未观察到 0~100 之间的进度百分比，-progress 解析可能失效")
	}
	if !sawSpeed {
		t.Error("未解析到 ffmpeg 报告的速度")
	}
	if !app.GetTranscodeStatus().Running {
		t.Fatal("任务在取消之前就已结束，无法验证取消行为")
	}

	if err := app.CancelTranscode(); err != nil {
		t.Fatalf("CancelTranscode 失败：%v", err)
	}

	final := waitForJob(t, app, time.Minute)
	if final.Status != "canceled" {
		t.Fatalf("任务状态 = %q，期望 canceled（消息 %q）", final.Status, final.Message)
	}
	if _, err := os.Stat(filepath.Join(outDir, "long.mp4")); !os.IsNotExist(err) {
		t.Error("取消后应删除未完成的输出文件")
	}
}

// TestLocateToolPrefersBundledBin 验证随项目分发的 bin/ 目录优先于系统 PATH。
func TestLocateToolPrefersBundledBin(t *testing.T) {
	bundled := filepath.Join("bin", "ffmpeg.exe")
	if _, err := os.Stat(bundled); err != nil {
		t.Skip("项目内没有 bin/ffmpeg.exe，跳过")
	}
	want, err := filepath.Abs(bundled)
	if err != nil {
		t.Fatalf("解析路径失败：%v", err)
	}
	if got := locateTool("ffmpeg"); !samePath(got, want) {
		t.Errorf("locateTool(ffmpeg) = %q，期望优先使用项目内的 %q", got, want)
	}
}

// ---- 测试辅助 ----

func waitForJob(t *testing.T, app *App, timeout time.Duration) TranscodeSnapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := app.GetTranscodeStatus()
		if !snapshot.Running {
			return snapshot
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("等待任务结束超时，最后状态：%+v", app.GetTranscodeStatus())
	return TranscodeSnapshot{}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsNote(notes []string, want string) bool {
	for _, note := range notes {
		if strings.Contains(note, want) {
			return true
		}
	}
	return false
}

// moovBeforeMdat 直接读取 MP4 顶层原子顺序，确认启用了 faststart。
func moovBeforeMdat(t *testing.T, path string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取输出文件失败：%v", err)
	}
	moov := strings.Index(string(data), "moov")
	mdat := strings.Index(string(data), "mdat")
	if moov < 0 || mdat < 0 {
		t.Fatalf("未找到 moov/mdat 原子（moov=%d mdat=%d）", moov, mdat)
	}
	return moov < mdat
}
