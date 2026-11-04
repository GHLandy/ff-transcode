package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"
)

// toolNames 是本应用依赖的两个可执行文件。
const (
	toolFFmpeg  = "ffmpeg"
	toolFFprobe = "ffprobe"
)

// SourceKind 标识查找来源，供界面展示查找过程。
const (
	sourceUser     = "user"     // 用户手动指定的 ffmpeg 位置
	sourceBin      = "bin"      // 程序目录下的 bin 目录
	sourcePath     = "path"     // 系统 PATH 环境变量
	sourceNotFound = "notfound" // 未找到
)

// DetectSource 是查找过程中的一步，记录该来源的细节与结果。
type DetectSource struct {
	Kind   string `json:"kind"`   // user | bin | path
	Label  string `json:"label"`  // 用户设置 / 程序 bin 目录 / PATH
	Detail string `json:"detail"` // 该来源检查的具体位置或说明
	Found  bool   `json:"found"`
	Note   string `json:"note"` // 结果说明：未设置（跳过）/ 未找到 ffmpeg 与 ffprobe / 已扫描 N 个目录…
}

// ToolDetection 是一次完整的 ffmpeg / ffprobe 检测结果。
type ToolDetection struct {
	Available     bool           `json:"available"`
	FFmpegPath    string         `json:"ffmpegPath"`
	FFprobePath   string         `json:"ffprobePath"`
	FFmpegVersion string         `json:"ffmpegVersion"`
	Message       string         `json:"message"`
	Sources       []DetectSource `json:"sources"`
	// BinDir 是程序 bin 目录的绝对路径，界面据此提示用户把 ffmpeg 放进去。
	BinDir string `json:"binDir"`
}

// toolPair 是同一目录下同时找到的 ffmpeg / ffprobe。
type toolPair struct {
	ffmpeg  string
	ffprobe string
}

// resolveToolSet 按「用户设置 → 程序 bin 目录 → PATH」的优先级查找 ffmpeg / ffprobe，
// 并返回完整路径与每一步的查找记录。每个候选目录都必须同时具备两个可执行文件
// 才算命中，避免把不同来源（可能是不同版本）的工具拼成一对。
func resolveToolSet(settings AppSettings) (string, string, []DetectSource) {
	sources := make([]DetectSource, 0, 3)

	userDirs := make([]string, 0, 1)
	if dir := strings.TrimSpace(settings.FFmpegDir); dir != "" {
		userDirs = append(userDirs, dir)
	}
	userDetail := "-"
	if len(userDirs) > 0 {
		userDetail = userDirs[0]
	}
	pair, src := scanSource(sourceUser, "用户设置", userDetail, userDirs, "未设置（跳过）")
	sources = append(sources, src)
	if src.Found {
		return pair.ffmpeg, pair.ffprobe, sources
	}

	binDirs := bundledBinDirs()
	binDetail := "-"
	if len(binDirs) > 0 {
		binDetail = strings.Join(binDirs, " · ")
	}
	pair, src = scanSource(sourceBin, "程序 bin 目录", binDetail, binDirs, "未定位到程序目录")
	sources = append(sources, src)
	if src.Found {
		return pair.ffmpeg, pair.ffprobe, sources
	}

	pair, src = scanSource(sourcePath, "PATH", "-", pathEnvDirs(), "PATH 为空")
	sources = append(sources, src)
	if src.Found {
		return pair.ffmpeg, pair.ffprobe, sources
	}

	return "", "", sources
}

// scanSource 逐个检查一个来源的候选目录，只有同一目录中 ffmpeg 与 ffprobe
// 都存在才返回这一对；否则记录不完整的原因，继续检查下一个目录。
func scanSource(kind, label, detail string, dirs []string, emptyNote string) (toolPair, DetectSource) {
	src := DetectSource{Kind: kind, Label: label, Detail: detail}
	if len(dirs) == 0 {
		src.Note = emptyNote
		return toolPair{}, src
	}

	partial := ""
	for _, dir := range dirs {
		ffmpeg := findInDir(dir, toolFFmpeg)
		ffprobe := findInDir(dir, toolFFprobe)
		if ffmpeg != "" && ffprobe != "" {
			src.Found = true
			src.Detail = dir
			src.Note = "已找到 ffmpeg 与 ffprobe"
			return toolPair{ffmpeg: ffmpeg, ffprobe: ffprobe}, src
		}
		if partial == "" {
			switch {
			case ffmpeg != "":
				partial = "仅找到 ffmpeg，该目录缺少 ffprobe（两者须配套）"
			case ffprobe != "":
				partial = "仅找到 ffprobe，该目录缺少 ffmpeg（两者须配套）"
			default:
				partial = "未找到 ffmpeg 与 ffprobe"
			}
		}
	}
	src.Note = partial
	return toolPair{}, src
}

// pathEnvDirs 返回 PATH 中非空的目录列表。
func pathEnvDirs() []string {
	dirs := make([]string, 0, 8)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir = strings.TrimSpace(dir); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// detectTools 执行一次完整检测，并组装界面所需的全部信息。
func detectTools(settings AppSettings) ToolDetection {
	ffmpegPath, ffprobePath, sources := resolveToolSet(settings)

	det := ToolDetection{
		FFmpegPath:  ffmpegPath,
		FFprobePath: ffprobePath,
		Sources:     sources,
		BinDir:      primaryBinDir(),
	}

	// -version 也可能因用户指定的可执行文件异常而挂起；必须限制等待时间，
	// 否则 CheckTools 永不返回，界面会一直停在「检测中…」。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if ffmpegPath != "" {
		det.FFmpegVersion = toolVersion(ctx, ffmpegPath)
	}

	if ffmpegPath == "" {
		det.Message = "未找到 ffmpeg / ffprobe。需要在同一个目录中同时找到这两个可执行文件。"
	} else {
		det.Available = true
		det.Message = "FFmpeg 已就绪"
	}
	return det
}

// toolVersion 读取 ffmpeg/ffprobe 的版本号（首行）。
func toolVersion(ctx context.Context, path string) string {
	cmd := exec.CommandContext(ctx, path, "-version")
	cmd.SysProcAttr = hideWindow()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return line
}

// findInDir 在指定目录中查找可执行文件，返回绝对路径，未找到时返回空字符串。
func findInDir(dir, name string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	for _, exe := range exeNames(name) {
		candidate := filepath.Join(dir, exe)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			if abs, absErr := filepath.Abs(candidate); absErr == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

// exeNames 返回某个工具在当前平台下的候选文件名。
func exeNames(name string) []string {
	if goruntime.GOOS == "windows" {
		return []string{name + ".exe", name}
	}
	return []string{name}
}

// bundledBinDirs 返回程序目录下的 bin 目录候选：仅运行程序（可执行文件）同级目录下的 bin。
func bundledBinDirs() []string {
	dirs := make([]string, 0, 1)
	seen := make(map[string]bool)
	add := func(dir string) {
		dir = filepath.Clean(dir)
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	if exe, err := os.Executable(); err == nil {
		add(filepath.Join(filepath.Dir(exe), "bin"))
	}
	return dirs
}

// primaryBinDir 返回界面提示用户放置 ffmpeg 的首选 bin 目录（运行程序目录下的 bin）。
func primaryBinDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "bin")
	}
	return "bin"
}
