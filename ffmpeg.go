package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	goruntime "runtime"
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

// resolveToolSet 按「用户设置 → 程序 bin 目录 → PATH」的优先级查找 ffmpeg / ffprobe，
// 并返回完整路径与每一步的查找记录。
func resolveToolSet(settings AppSettings) (string, string, []DetectSource) {
	sources := make([]DetectSource, 0, 3)

	// 1. 用户设置
	userSrc := DetectSource{Kind: sourceUser, Label: "用户设置"}
	userDir := strings.TrimSpace(settings.FFmpegDir)
	ffmpegPath, ffprobePath := "", ""
	if userDir == "" {
		userSrc.Detail = "-"
		userSrc.Note = "未设置（跳过）"
	} else {
		userSrc.Detail = userDir
		ffmpegPath = findInDir(userDir, toolFFmpeg)
		ffprobePath = findInDir(userDir, toolFFprobe)
		switch {
		case ffmpegPath != "" && ffprobePath != "":
			userSrc.Found = true
			userSrc.Note = "已找到 ffmpeg 与 ffprobe"
		case ffmpegPath == "" && ffprobePath == "":
			userSrc.Note = "未找到 ffmpeg 与 ffprobe"
		default:
			userSrc.Note = "该目录缺少 " + missingToolName(ffmpegPath, ffprobePath)
		}
	}
	sources = append(sources, userSrc)
	if ffmpegPath != "" && ffprobePath != "" {
		return ffmpegPath, ffprobePath, sources
	}

	// 2. 程序目录下的 bin 目录
	binSrc := DetectSource{Kind: sourceBin, Label: "程序 bin 目录"}
	binDirs := bundledBinDirs()
	binSrc.Detail = strings.Join(binDirs, " · ")
	if len(binDirs) == 0 {
		binSrc.Note = "未定位到程序目录"
	} else {
		foundFFmpeg, foundFFprobe := "", ""
		for _, dir := range binDirs {
			if foundFFmpeg == "" {
				foundFFmpeg = findInDir(dir, toolFFmpeg)
			}
			if foundFFprobe == "" {
				foundFFprobe = findInDir(dir, toolFFprobe)
			}
		}
		if ffmpegPath == "" {
			ffmpegPath = foundFFmpeg
		}
		if ffprobePath == "" {
			ffprobePath = foundFFprobe
		}
		switch {
		case foundFFmpeg != "" && foundFFprobe != "":
			binSrc.Found = true
			binSrc.Note = "已找到 ffmpeg 与 ffprobe"
		case foundFFmpeg != "" || foundFFprobe != "":
			binSrc.Note = "仅找到 " + presentToolName(foundFFmpeg, foundFFprobe)
		default:
			binSrc.Note = "未找到 ffmpeg 与 ffprobe"
		}
	}
	sources = append(sources, binSrc)
	if ffmpegPath != "" && ffprobePath != "" {
		return ffmpegPath, ffprobePath, sources
	}

	// 3. 系统 PATH
	pathSrc := DetectSource{Kind: sourcePath, Label: "PATH"}
	pathSrc.Detail = "-"
	scanned := 0
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		scanned++
		if ffmpegPath == "" {
			ffmpegPath = findInDir(dir, toolFFmpeg)
		}
		if ffprobePath == "" {
			ffprobePath = findInDir(dir, toolFFprobe)
		}
	}
	switch {
	case ffmpegPath != "" && ffprobePath != "":
		pathSrc.Found = true
		pathSrc.Note = fmt.Sprintf("已扫描 %d 个目录，找到 ffmpeg 与 ffprobe", scanned)
	case ffmpegPath != "" || ffprobePath != "":
		pathSrc.Found = true
		pathSrc.Note = fmt.Sprintf("已扫描 %d 个目录，仅找到 %s", scanned, presentToolName(ffmpegPath, ffprobePath))
	default:
		pathSrc.Note = fmt.Sprintf("已扫描 %d 个目录，未找到匹配的 ffmpeg/ffprobe", scanned)
	}
	sources = append(sources, pathSrc)

	return ffmpegPath, ffprobePath, sources
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

	ctx := context.Background()
	if ffmpegPath != "" {
		det.FFmpegVersion = toolVersion(ctx, ffmpegPath)
	}

	switch {
	case ffmpegPath == "" && ffprobePath == "":
		det.Message = "未找到 ffmpeg / ffprobe。需要在同一个目录中同时找到 ffmpeg 与 ffprobe。"
	case ffmpegPath == "":
		det.Message = "只找到 ffprobe，缺少 ffmpeg；两者需放在同一个目录。"
	case ffprobePath == "":
		det.Message = "只找到 ffmpeg，缺少 ffprobe；两者需放在同一个目录。"
	default:
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

// missingToolName 返回缺失的工具名，用于提示。
func missingToolName(ffmpegPath, _ string) string {
	if ffmpegPath == "" {
		return "ffmpeg"
	}
	return "ffprobe"
}

// presentToolName 返回已找到的工具名，用于提示。
func presentToolName(ffmpegPath, _ string) string {
	if ffmpegPath != "" {
		return "ffmpeg"
	}
	return "ffprobe"
}
