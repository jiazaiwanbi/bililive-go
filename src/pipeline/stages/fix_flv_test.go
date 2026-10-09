package stages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/stretchr/testify/require"
)

func TestFixFlvStageSkipsEnhancedVideo(t *testing.T) {
	file := filepath.Join(t.TempDir(), "record.flv")
	data := []byte{'F', 'L', 'V', 1, 5, 0, 0, 0, 9, 0, 0, 0, 0,
		9, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0x90, 'h', 'v', 'c', '1', 0, 0, 0, 16}
	require.NoError(t, os.WriteFile(file, data, 0600))
	stage, err := NewFixFlvStage(pipeline.StageConfig{})
	require.NoError(t, err)
	input := []pipeline.FileInfo{pipeline.NewVideoFileInfo(file)}
	out, err := stage.Execute(testPipelineContext(), input)
	require.NoError(t, err)
	require.Equal(t, input, out)
	require.Empty(t, stage.(*FixFlvStage).GetCommands())
	require.Contains(t, stage.(*FixFlvStage).GetLogs(), "跳过修复")
	require.NotContains(t, stage.(*FixFlvStage).GetLogs(), "修复完成")
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, data, after)
}
