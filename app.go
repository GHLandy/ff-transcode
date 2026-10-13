package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是绑定到前端的应用对象，聚合媒体探测与转码能力。
type App struct {
	ctx context.Context

	jobMu sync.Mutex
	job   *transcodeJob
}

// NewApp 创建应用实例。
func NewApp() *App {
	return &App{}
}

// startup 由 Wails 在应用启动时调用，保存上下文供对话框与事件使用。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// baseContext 返回可用的上下文，避免 startup 之前调用时出现 nil。
func (a *App) baseContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// ---- 环境检测 ----

// ToolStatus 报告 ffmpeg / ffprobe 的可用性。
type ToolStatus struct {
	Available     bool   `json:"available"`
	FFmpegPath    string `json:"ffmpegPath"`
	FFprobePath   string `json:"ffprobePath"`
	FFmpegVersion string `json:"ffmpegVersion"`
	Message       string `json:"message"`
}

// CheckTools 检测 ffmpeg / ffprobe 是否可用，界面据此提示环境问题。
func (a *App) CheckTools() ToolStatus {
	ffmpegPath := locateTool("ffmpeg")
	ffprobePath := locateTool("ffprobe")

	status := ToolStatus{FFmpegPath: ffmpegPath, FFprobePath: ffprobePath}
	if ffmpegPath != "" {
		cmd := exec.Command(ffmpegPath, "-version")
		cmd.SysProcAttr = hideWindow()
		if out, err := cmd.Output(); err == nil {
			status.FFmpegVersion = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		}
	}

	switch {
	case ffmpegPath == "" && ffprobePath == "":
		status.Message = "未找到 ffmpeg 与 ffprobe。请安装 FFmpeg 并把 bin 目录加入 PATH。"
	case ffmpegPath == "":
		status.Message = "未找到 ffmpeg，无法转码；请把 ffmpeg 放入 PATH 或随应用 bin 目录。"
	case ffprobePath == "":
		status.Message = "未找到 ffprobe，无法读取视频编码信息；请把 ffprobe 放入 PATH 或随应用 bin 目录。"
	default:
		status.Available = true
		status.Message = "FFmpeg 已就绪"
	}
	return status
}

// ---- 文件选择 ----

// SelectVideoFile 打开文件对话框选择视频，返回空字符串表示用户取消。
func (a *App) SelectVideoFile() (string, error) {
	return runtime.OpenFileDialog(a.baseContext(), runtime.OpenDialogOptions{
		Title: "选择要转码的视频文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "视频文件 (*.mp4;*.mov;*.mkv;*.m4v;*.avi;*.flv;*.wmv;*.ts;*.webm;*.mpg;*.mpeg)",
				Pattern:     "*.mp4;*.mov;*.mkv;*.m4v;*.avi;*.flv;*.wmv;*.ts;*.webm;*.mpg;*.mpeg",
			},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
}

// SelectOutputDir 打开目录对话框选择输出目录。
func (a *App) SelectOutputDir(current string) (string, error) {
	return runtime.OpenDirectoryDialog(a.baseContext(), runtime.OpenDialogOptions{
		Title:            "选择输出目录",
		DefaultDirectory: strings.TrimSpace(current),
	})
}

// RevealInExplorer 在资源管理器中定位输出文件。
func (a *App) RevealInExplorer(path string) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return errors.New("路径为空")
	}
	if _, err := os.Stat(target); err != nil {
		// 文件可能已被移动，退回到所在目录。
		parent := filepath.Dir(target)
		if _, dirErr := os.Stat(parent); dirErr != nil {
			return errors.New("路径不存在：" + target)
		}
		target = parent
	}
	return openInFileManager(target)
}

// ---- 媒体探测 ----

// ProbeMedia 读取视频文件的容器、视频流与音频流信息，供界面展示原始编码参数。
func (a *App) ProbeMedia(path string) (*MediaInfo, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("路径为空")
	}
	return probeMedia(a.baseContext(), path)
}
