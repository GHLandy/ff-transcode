//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindow 在非 Windows 平台上无需处理窗口。
func hideWindow() *syscall.SysProcAttr {
	return nil
}

// openInFileManager 在系统文件管理器中打开目录。
func openInFileManager(path string) error {
	return exec.Command("xdg-open", path).Start()
}
