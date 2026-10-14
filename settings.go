package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AppSettings 是持久化到用户配置目录的应用设置。
type AppSettings struct {
	// FFmpegDir 是用户手动指定的 ffmpeg / ffprobe 所在目录；为空表示未设置。
	FFmpegDir string `json:"ffmpegDir"`
}

// appConfigDir 是配置在用户配置目录下的子目录名。
const appConfigDir = "fftranscode"

// settingsFileName 是配置文件在用户配置目录下的文件名。
const settingsFileName = "settings.json"

var (
	settingsMu   sync.RWMutex
	settingsPath string
)

// loadSettings 读取用户设置；文件不存在或解析失败时返回空设置而不报错。
func loadSettings() AppSettings {
	path := settingsFilePath()
	if path == "" {
		return AppSettings{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return AppSettings{}
	}
	var s AppSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		return AppSettings{}
	}
	s.FFmpegDir = strings.TrimSpace(s.FFmpegDir)
	return s
}

// saveSettings 把用户设置写入配置目录（目录不存在时自动创建）。
func saveSettings(s AppSettings) error {
	path := settingsFilePath()
	if path == "" {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// settingsFilePath 返回配置文件的绝对路径，结果会被缓存。
func settingsFilePath() string {
	settingsMu.RLock()
	cached := settingsPath
	settingsMu.RUnlock()
	if cached != "" {
		return cached
	}

	dir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			dir = wd
		} else {
			return ""
		}
	}
	path := filepath.Join(dir, appConfigDir, settingsFileName)

	settingsMu.Lock()
	settingsPath = path
	settingsMu.Unlock()
	return path
}

// currentSettings 返回当前生效的用户设置。
func currentSettings() AppSettings {
	return loadSettings()
}

// setUserFFmpegDir 保存（或清空）用户手动指定的 ffmpeg 目录。
func setUserFFmpegDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	return saveSettings(AppSettings{FFmpegDir: dir})
}
