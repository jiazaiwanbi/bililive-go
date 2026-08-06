package recorders

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluele/gcache"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/live"
	livemock "github.com/bililive-go/bililive-go/src/live/mock"
	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
)

func TestTryRecordStopsWithoutPanicWhenFilenameRenderFails(t *testing.T) {
	previousConfig := configs.GetCurrentConfig()
	cfg := configs.NewConfig()
	cfg.OutputTmpl = `{{ fail "render failed" }}`
	configs.SetCurrentConfig(cfg)
	t.Cleanup(func() {
		configs.SetCurrentConfig(previousConfig)
	})

	ctrl := gomock.NewController(t)
	l := livemock.NewMockLive(ctrl)
	logger := livelogger.New(0, logrus.Fields{"test": t.Name()})
	streamURL := &url.URL{Scheme: "https", Host: "example.com", Path: "/stream.flv"}

	l.EXPECT().GetRawUrl().Return("https://example.com/room").AnyTimes()
	l.EXPECT().GetStreamInfos().Return([]*live.StreamUrlInfo{{Url: streamURL}}, nil)
	l.EXPECT().GetLogger().Return(logger)

	cache := gcache.New(1).LRU().Build()
	if err := cache.Set(l, &live.Info{Live: l}); err != nil {
		t.Fatalf("写入直播信息缓存失败: %v", err)
	}
	r := &recorder{Live: l, cache: cache}

	r.tryRecord(context.Background())

	logs := logger.GetLogs()
	if !strings.Contains(logs, "failed to render filename, recording aborted") {
		t.Fatalf("未记录文件名渲染失败日志: %s", logs)
	}
	if !strings.Contains(logs, "render failed") {
		t.Fatalf("日志未保留原始模板错误: %s", logs)
	}
}

func TestCanPostProcessStoppedRecording(t *testing.T) {
	tempDir := t.TempDir()
	recordingPath := filepath.Join(tempDir, "record.aac")
	require.NoError(t, os.WriteFile(recordingPath, []byte("audio-data"), 0o600))

	activeRecorder := &recorder{stop: make(chan struct{})}
	assert.False(t, activeRecorder.canPostProcessStoppedRecording(recordingPath))

	stoppedRecorder := &recorder{stop: make(chan struct{})}
	close(stoppedRecorder.stop)
	assert.True(t, stoppedRecorder.canPostProcessStoppedRecording(recordingPath))
	assert.False(t, stoppedRecorder.canPostProcessStoppedRecording(filepath.Join(tempDir, "missing.aac")))

	emptyPath := filepath.Join(tempDir, "empty.aac")
	require.NoError(t, os.WriteFile(emptyPath, nil, 0o600))
	assert.False(t, stoppedRecorder.canPostProcessStoppedRecording(emptyPath))
}
