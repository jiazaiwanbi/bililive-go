//go:build windows

package tools

import "os/exec"

func prepareFLVRepairProcess(cmd *exec.Cmd) {
	// Windows 由 killProcessTree 使用 taskkill /T 终止进程树。
}
