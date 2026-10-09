package tools

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func repairTestFLV(tags ...[]byte) []byte {
	data := []byte{'F', 'L', 'V', 1, 5, 0, 0, 0, 9, 0, 0, 0, 0}
	for _, body := range tags {
		header := make([]byte, 11)
		header[0] = 9
		header[1], header[2], header[3] = byte(len(body)>>16), byte(len(body)>>8), byte(len(body))
		data = append(data, header...)
		data = append(data, body...)
		data = binary.BigEndian.AppendUint32(data, uint32(11+len(body)))
	}
	return data
}

func TestFLVRepairFormatDetection(t *testing.T) {
	for _, test := range []struct {
		name string
		tags [][]byte
		skip bool
	}{
		{"H264", [][]byte{{0x17, 0, 0, 0, 0}, {0x27, 1, 0, 0, 0}}, false},
		{"HEVC序列头", [][]byte{{0x90, 'h', 'v', 'c', '1'}}, true},
		{"HEVC关键帧", [][]byte{{0x91, 'h', 'v', 'c', '1'}}, true},
		{"HEVC帧", [][]byte{{0xa1, 'h', 'v', 'c', '1'}}, true},
		{"中途切换HEVC", [][]byte{{0x17, 0}, {0x27, 1}, {0x91, 'h', 'v', 'c', '1'}}, true},
		{"AV1", [][]byte{{0x90, 'a', 'v', '0', '1'}}, true},
		{"无视频标签", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "record.flv")
			data := repairTestFLV(test.tags...)
			require.NoError(t, os.WriteFile(file, data, 0600))
			reason, err := inspectFLVForRepair(context.Background(), file)
			require.NoError(t, err)
			require.Equal(t, test.skip, reason != "")
			if test.skip {
				// 无需初始化工具 API，且不应启动修复进程或改动原文件。
				result, err := FixFlvByBililiveRecorderWithResult(context.Background(), file, func(int) { t.Fatal("不应启动旧修复器") })
				require.NoError(t, err)
				require.Equal(t, []string{file}, result.OutputFiles)
				require.NotEmpty(t, result.SkipReason)
			}
			after, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, data, after)
		})
	}
}

func TestFLVRepairInspectionCancellationAndTruncation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "record.flv")
	data := repairTestFLV([]byte{0x17, 0})
	require.NoError(t, os.WriteFile(file, data[:len(data)-5], 0600))
	_, err := inspectFLVForRepair(context.Background(), file)
	require.NoError(t, err) // 普通 FLV 截断可交给受保护的修复器。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = inspectFLVForRepair(ctx, file)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, os.WriteFile(file, []byte("not a FLV file"), 0600))
	_, err = inspectFLVForRepair(context.Background(), file)
	require.ErrorContains(t, err, "无效的 FLV")
}

// 测试子进程不分配大量内存，RSS 超限由监测函数模拟。
func TestFLVRepairChild(t *testing.T) {
	switch os.Getenv("BILILIVE_FLV_REPAIR_TEST") {
	case "wait":
		time.Sleep(30 * time.Second)
	case "output":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, flvRepairOutputLimit+1))
	case "env":
		fmt.Print(os.Getenv("DOTNET_GCHeapHardLimit"))
	default:
		return
	}
	os.Exit(0)
}

func repairChildCommand(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestFLVRepairChild$")
	cmd.Env = append(os.Environ(), "BILILIVE_FLV_REPAIR_TEST="+mode, "DOTNET_GCHeapHardLimit=ffffffff", "COMPlus_GCHeapHardLimit=ffffffff")
	return cmd
}

func TestFLVRepairProcessCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := repairChildCommand("wait")
	_, err := runFLVRepair(ctx, cmd, func(int) { cancel() }, flvRepairRSS)
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, cmd.ProcessState)
}

func TestFLVRepairProcessMemoryLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := repairChildCommand("wait")
	_, err := runFLVRepair(ctx, cmd, nil, func(int) (uint64, error) { return flvRepairRSSLimit + 1, nil })
	require.ErrorContains(t, err, "超过 2 GiB")
	require.NotNil(t, cmd.ProcessState)
}

func TestFLVRepairProcessMonitoringFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := repairChildCommand("wait")
	_, err := runFLVRepair(ctx, cmd, nil, func(int) (uint64, error) { return 0, errors.New("无法读取内存") })
	require.ErrorContains(t, err, "无法监测")
	require.NotNil(t, cmd.ProcessState)
}

func TestFLVRepairProcessHeapAndOutputLimits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := runFLVRepair(ctx, repairChildCommand("env"), nil, flvRepairRSS)
	require.NoError(t, err)
	require.Equal(t, "40000000", string(out))
	_, err = runFLVRepair(ctx, repairChildCommand("output"), nil, flvRepairRSS)
	require.ErrorContains(t, err, "输出超过 8 MiB")
}

func TestCleanupFLVRepairPartsPreservesOriginalAndExistingParts(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "[2026-10-03]录播.flv")
	existing := strings.TrimSuffix(file, ".flv") + ".fix_p001.flv"
	newPart := strings.TrimSuffix(file, ".flv") + ".fix_p002.flv"
	unrelated := filepath.Join(dir, "另一场.fix_p001.flv")
	for _, path := range []string{file, existing, unrelated} {
		require.NoError(t, os.WriteFile(path, []byte("原文件"), 0600))
	}
	previous, err := flvRepairParts(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(newPart, []byte("部分输出"), 0600))
	cleanupNewFLVRepairParts(file, previous)
	require.FileExists(t, file)
	require.FileExists(t, existing)
	require.FileExists(t, unrelated)
	require.NoFileExists(t, newPart)
}
