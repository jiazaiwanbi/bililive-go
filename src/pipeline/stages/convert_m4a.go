package stages

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/utils"
	"github.com/bililive-go/bililive-go/src/tools"
	"github.com/sirupsen/logrus"
)

// ConvertM4AStage 将 AAC 音频流无损封装到 M4A 容器。
type ConvertM4AStage struct {
	deleteSource bool
	commands     []string
	logs         string
}

// NewConvertM4AStage 创建 AAC 转 M4A 阶段。
func NewConvertM4AStage(config pipeline.StageConfig) (pipeline.Stage, error) {
	return &ConvertM4AStage{
		deleteSource: config.GetBoolOption(pipeline.OptionDeleteSource, false),
	}, nil
}

func (s *ConvertM4AStage) Name() string {
	return pipeline.StageNameConvertM4A
}

func (s *ConvertM4AStage) Execute(ctx *pipeline.PipelineContext, input []pipeline.FileInfo) ([]pipeline.FileInfo, error) {
	if len(input) == 0 {
		s.logs = "没有输入文件"
		return input, nil
	}

	// 没有 AAC 时直接透传，避免视频任务无意义地等待 FFmpeg 初始化。
	hasAAC := false
	for _, file := range input {
		if strings.EqualFold(filepath.Ext(file.Path), ".aac") {
			hasAAC = true
			break
		}
	}
	if !hasAAC {
		return input, nil
	}

	ffmpegPath := ctx.FFmpegPath
	if ffmpegPath == "" {
		if waitErr := tools.WaitFFmpegAsyncInitDone(ctx.Ctx, nil); waitErr != nil {
			s.logs = fmt.Sprintf("等待 FFmpeg 就绪被中断: %s", waitErr.Error())
			return nil, waitErr
		}
		var err error
		ffmpegPath, err = utils.GetFFmpegPath(ctx.Ctx)
		if err != nil {
			s.logs = fmt.Sprintf("ffmpeg 不可用: %s", err.Error())
			return nil, fmt.Errorf("ffmpeg not available: %w", err)
		}
	}

	output := make([]pipeline.FileInfo, 0, len(input)+1)
	for _, file := range input {
		if !strings.EqualFold(filepath.Ext(file.Path), ".aac") {
			output = append(output, file)
			continue
		}

		if _, err := os.Stat(file.Path); err != nil {
			if os.IsNotExist(err) {
				s.logs += fmt.Sprintf("文件不存在: %s\n", file.Path)
				continue
			}
			return nil, fmt.Errorf("failed to stat AAC file %s: %w", file.Path, err)
		}

		outputPath := strings.TrimSuffix(file.Path, filepath.Ext(file.Path)) + ".m4a"
		tempFile := filepath.Join(filepath.Dir(outputPath), ".converting_"+filepath.Base(outputPath))
		args := []string{
			"-hide_banner",
			"-y",
			"-i", file.Path,
			"-map", "0:a:0",
			"-vn",
			"-c:a", "copy",
			"-movflags", "+faststart",
			tempFile,
		}

		s.commands = append(s.commands, fmt.Sprintf("%s %s", ffmpegPath, strings.Join(args, " ")))
		ctx.Logger.Infof("无损封装 M4A: %s -> %s", file.Path, outputPath)

		cmdOutput, err := exec.CommandContext(ctx.Ctx, ffmpegPath, args...).CombinedOutput()
		if err != nil {
			_ = os.Remove(tempFile)
			s.logs += fmt.Sprintf("ffmpeg 封装失败: %s - %s\n", file.Path, strings.TrimSpace(string(cmdOutput)))
			return nil, fmt.Errorf("ffmpeg M4A conversion failed for %s: %w", file.Path, err)
		}

		if stat, statErr := os.Stat(tempFile); statErr != nil || stat.Size() == 0 {
			_ = os.Remove(tempFile)
			return nil, fmt.Errorf("M4A temp file was not created or is empty: %s", tempFile)
		}
		if err := os.Rename(tempFile, outputPath); err != nil {
			_ = os.Remove(tempFile)
			return nil, fmt.Errorf("failed to rename M4A temp file: %w", err)
		}

		output = append(output, pipeline.FileInfo{
			Path:       outputPath,
			Type:       pipeline.FileTypeAudio,
			SourcePath: file.Path,
		})

		if s.deleteSource {
			if err := os.Remove(file.Path); err != nil {
				logrus.WithError(err).WithField("file", file.Path).Warn("failed to delete original AAC file")
				s.logs += fmt.Sprintf("删除原始 AAC 失败: %s\n", file.Path)
			} else {
				s.logs += fmt.Sprintf("已删除原始 AAC: %s\n", file.Path)
			}
		}

		s.logs += fmt.Sprintf("封装完成: %s -> %s\n", filepath.Base(file.Path), filepath.Base(outputPath))
		ctx.Logger.Infof("M4A 封装完成: %s", outputPath)
	}

	return output, nil
}

func (s *ConvertM4AStage) GetCommands() []string {
	return s.commands
}

func (s *ConvertM4AStage) GetLogs() string {
	return s.logs
}
