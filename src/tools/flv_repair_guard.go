package tools

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	blog "github.com/bililive-go/bililive-go/src/log"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	flvRepairRSSLimit    = uint64(2 << 30)
	flvRepairOutputLimit = 8 << 20
	flvRepairTimeout     = 2 * time.Hour
)

var flvRepairSlot = make(chan struct{}, 1)

// inspectFLVForRepair 只读取标签头和视频首字节，内存用量与文件大小无关。
// 必须检查所有标签：同一文件可能在中途切换到 Enhanced FLV。
// v2.17.3 无法识别 Enhanced 关键帧，会把整场直播留在同一个内存分组。
func inspectFLVForRepair(ctx context.Context, fileName string) (string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var header [9]byte
	if _, err = io.ReadFull(f, header[:]); err != nil {
		return "", fmt.Errorf("读取 FLV 文件头失败: %w", err)
	}
	if string(header[:3]) != "FLV" || header[3] != 1 {
		return "", errors.New("无效的 FLV 文件头")
	}
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	offset := int64(binary.BigEndian.Uint32(header[5:9]))
	if offset < 9 || offset > stat.Size() {
		return "", errors.New("无效的 FLV 数据偏移")
	}
	offset += 4 // PreviousTagSize0
	var tag [11]byte
	var first [1]byte
	for offset+11 <= stat.Size() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := f.ReadAt(tag[:], offset); err != nil {
			return "", err
		}
		size := int64(tag[1])<<16 | int64(tag[2])<<8 | int64(tag[3])
		// 无法继续遍历损坏的结构时，让受资源保护的修复器尝试修复。
		if tag[0] != 8 && tag[0] != 9 && tag[0] != 18 {
			break
		}
		if tag[0] == 9 && size > 0 && offset+11 < stat.Size() {
			if _, err := f.ReadAt(first[:], offset+11); err != nil {
				return "", err
			}
			if first[0]&0x80 != 0 {
				return "Enhanced FLV 视频（如 H.265/HEVC）不兼容当前 BililiveRecorder，继续使用原始文件", nil
			}
		}
		offset += 11 + size + 4
	}
	return "", ctx.Err()
}

// limitedRepairOutput 持续排空管道，但最多保留 8 MiB，避免日志占满主进程内存。
type limitedRepairOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (b *limitedRepairOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := flvRepairOutputLimit - b.buffer.Len()
	if n > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func flvRepairRSS(pid int) (uint64, error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, err
	}
	info, err := p.MemoryInfo()
	if err != nil {
		return 0, err
	}
	return info.RSS, nil
}

// runFLVRepair 限制托管堆并监测 RSS；取消或超限时终止整个进程树。
func runFLVRepair(ctx context.Context, cmd *exec.Cmd, onPID func(int), rss func(int) (uint64, error)) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, flvRepairTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// .NET 环境变量使用十六进制：托管堆 1 GiB，RSS 2 GiB 包含原生内存。
	env := cmd.Environ()
	cmd.Env = make([]string, 0, len(env)+1)
	for _, value := range env {
		name, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(name, "DOTNET_GCHeapHardLimit") && !strings.EqualFold(name, "COMPlus_GCHeapHardLimit") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "DOTNET_GCHeapHardLimit=40000000")
	prepareFLVRepairProcess(cmd)
	var output limitedRepairOutput
	cmd.Stdout = &output
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("flv-repair-%d", cmd.Process.Pid)
	RegisterProcess(name, cmd.Process.Pid, ProcessCategoryRecorder)
	defer UnregisterProcess(name)
	if onPID != nil {
		onPID(cmd.Process.Pid)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stop := func(reason error) ([]byte, error) {
		if err := killProcessTree(cmd.Process.Pid); err != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil, reason
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	monitorErrors := 0
	for {
		select {
		case err := <-done:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil {
				return nil, fmt.Errorf("FLV 修复进程失败: %w", err)
			}
			if output.overflow {
				return nil, errors.New("FLV 修复器输出超过 8 MiB 上限")
			}
			return output.buffer.Bytes(), nil
		case <-ctx.Done():
			return stop(ctx.Err())
		case <-ticker.C:
			used, err := rss(cmd.Process.Pid)
			if err != nil {
				monitorErrors++
				if monitorErrors >= 3 {
					return stop(fmt.Errorf("无法监测 FLV 修复进程内存: %w", err))
				}
				continue
			}
			monitorErrors = 0
			if used > flvRepairRSSLimit {
				return stop(fmt.Errorf("FLV 修复进程内存 %d bytes 超过 2 GiB 上限，已终止并保留原文件", used))
			}
		}
	}
}

func flvRepairParts(fileName string) (map[string]bool, error) {
	dir := filepath.Dir(fileName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)) + ".fix_p"
	parts := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), filepath.Ext(fileName)) {
			parts[filepath.Join(dir, entry.Name())] = true
		}
	}
	return parts, nil
}

func cleanupNewFLVRepairParts(fileName string, previous map[string]bool) {
	parts, err := flvRepairParts(fileName)
	if err != nil {
		blog.GetLogger().WithError(err).Warn("无法枚举 FLV 修复临时文件")
		return
	}
	for part := range parts {
		if !previous[part] {
			if err := os.Remove(part); err != nil {
				blog.GetLogger().WithError(err).Warnf("无法清理 FLV 修复临时文件: %s", part)
			}
		}
	}
}
