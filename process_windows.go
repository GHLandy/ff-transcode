//go:build windows

package main

import (
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
	// explorer 对 /select, 的解析比较特殊，参数必须是一个整体。
	return exec.Command("explorer", "/select,"+path).Start()
}
