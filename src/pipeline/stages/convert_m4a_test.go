package stages

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertM4AStagePassesThroughNonAACWithoutFFmpeg(t *testing.T) {
	stage, err := NewConvertM4AStage(pipeline.StageConfig{Name: pipeline.StageNameConvertM4A})
	require.NoError(t, err)

	input := []pipeline.FileInfo{pipeline.NewVideoFileInfo("record.flv")}
	output, err := stage.Execute(testPipelineContext(), input)

	require.NoError(t, err)
	assert.Equal(t, input, output)
}

func TestConvertM4AStageRemuxesAACAndDeletesSource(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("测试环境没有 FFmpeg")
	}

	tempDir := t.TempDir()
	aacPath := filepath.Join(tempDir, "record.aac")
	generate := exec.CommandContext(context.Background(), ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1",
		"-c:a", "aac", "-f", "adts", "-y", aacPath,
	)
	require.NoError(t, generate.Run())

	stage, err := NewConvertM4AStage(pipeline.StageConfig{
		Name: pipeline.StageNameConvertM4A,
		Options: map[string]any{
			pipeline.OptionDeleteSource: true,
		},
	})
	require.NoError(t, err)

	ctx := testPipelineContext()
	ctx.FFmpegPath = ffmpegPath
	output, err := stage.Execute(ctx, []pipeline.FileInfo{pipeline.NewAudioFileInfo(aacPath)})

	require.NoError(t, err)
	require.Len(t, output, 1)
	assert.Equal(t, filepath.Join(tempDir, "record.m4a"), output[0].Path)
	assert.Equal(t, pipeline.FileTypeAudio, output[0].Type)
	assert.FileExists(t, output[0].Path)
	assert.NoFileExists(t, aacPath)
	stat, err := os.Stat(output[0].Path)
	require.NoError(t, err)
	assert.Positive(t, stat.Size())
}

func TestConvertM4AStageKeepsSourceWithoutPassingItToLaterStages(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("测试环境没有 FFmpeg")
	}

	tempDir := t.TempDir()
	aacPath := filepath.Join(tempDir, "record.aac")
	generate := exec.CommandContext(context.Background(), ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1",
		"-c:a", "aac", "-f", "adts", "-y", aacPath,
	)
	require.NoError(t, generate.Run())

	stage, err := NewConvertM4AStage(pipeline.StageConfig{
		Name: pipeline.StageNameConvertM4A,
		Options: map[string]any{
			pipeline.OptionDeleteSource: false,
		},
	})
	require.NoError(t, err)

	ctx := testPipelineContext()
	ctx.FFmpegPath = ffmpegPath
	output, err := stage.Execute(ctx, []pipeline.FileInfo{pipeline.NewAudioFileInfo(aacPath)})

	require.NoError(t, err)
	require.Len(t, output, 1)
	assert.Equal(t, filepath.Join(tempDir, "record.m4a"), output[0].Path)
	assert.FileExists(t, aacPath)
	assert.FileExists(t, output[0].Path)
}
