//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

func prepareFLVRepairProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// 独立进程组使取消时能够安全终止修复器的所有子进程。
	cmd.SysProcAttr.Setpgid = true
}
