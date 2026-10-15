package main

import (
	"context"
	"errors"
	"os"
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

// CheckTools 按「用户设置 → 程序 bin 目录 → PATH」的优先级检测 ffmpeg / ffprobe。
func (a *App) CheckTools() ToolDetection {
	return detectTools(currentSettings())
}

// GetUserSettings 返回当前生效的用户设置。
func (a *App) GetUserSettings() AppSettings {
	return currentSettings()
}

// ChooseFFmpeg 打开文件对话框，让用户手动指定 ffmpeg 可执行文件；
// 选定后读取其所在目录并保存为「用户设置」，同目录须存在 ffprobe。
// 返回空字符串表示用户取消；返回的检测结果已按新设置重新执行。
func (a *App) ChooseFFmpeg() (ToolDetection, error) {
	selected, err := runtime.OpenFileDialog(a.baseContext(), runtime.OpenDialogOptions{
		Title: "选择 ffmpeg 可执行文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "ffmpeg (ffmpeg / ffmpeg.exe)", Pattern: "ffmpeg;ffmpeg.exe"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return ToolDetection{}, err
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		// 用户取消：返回当前检测结果，便于前端刷新。
		return detectTools(currentSettings()), nil
	}

	dir := filepath.Dir(selected)
	if err := setUserFFmpegDir(dir); err != nil {
		return ToolDetection{}, errors.New("保存设置失败：" + err.Error())
	}
	return detectTools(currentSettings()), nil
}

// ClearFFmpegSetting 清除用户手动设置的 ffmpeg 目录，回退到自动查找。
func (a *App) ClearFFmpegSetting() (ToolDetection, error) {
	if err := setUserFFmpegDir(""); err != nil {
		return ToolDetection{}, errors.New("保存设置失败：" + err.Error())
	}
	return detectTools(currentSettings()), nil
}

// OpenBinDir 在文件管理器中打开程序 bin 目录，方便用户放入 ffmpeg / ffprobe。
func (a *App) OpenBinDir() error {
	dir := primaryBinDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.New("创建目录失败：" + err.Error())
	}
	return openInFileManager(dir)
}

// OpenFFmpegDownloadPage 打开 FFmpeg 官方下载页。
func (a *App) OpenFFmpegDownloadPage() {
	runtime.BrowserOpenURL(a.baseContext(), "https://ffmpeg.org/download.html")
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
