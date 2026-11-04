//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow 让 ffmpeg / ffprobe 静默运行，不弹出控制台窗口。
func hideWindow() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// openInFileManager 在资源管理器中打开目录或定位文件。
func openInFileManager(path string) error {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return exec.Command("explorer", path).Start()
	}
	// /select, 只用于定位文件；对目录使用会打开父目录并选中它，而非进入目录。
	return exec.Command("explorer", "/select,"+path).Start()
}
