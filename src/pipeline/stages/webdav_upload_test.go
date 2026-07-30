package stages

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebDAVUploadStageUploadsAndDeletesAfterSuccess(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	var paths []string
	var uploaded []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "alice" || password != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			uploaded = body
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	localFile := filepath.Join(t.TempDir(), "record.aac")
	require.NoError(t, os.WriteFile(localFile, []byte("audio-data"), 0o644))

	stage, err := NewWebDAVUploadStage(pipeline.StageConfig{
		Name: pipeline.StageNameWebDAVUpload,
		Options: map[string]any{
			pipeline.OptionURL:          server.URL + "/dav-root",
			pipeline.OptionUsername:     "alice",
			pipeline.OptionPassword:     "secret",
			pipeline.OptionPathTemplate: "/归档/{{ .Platform }}/{{ .HostName }}/{{ .FileName }}",
			pipeline.OptionDeleteAfter:  true,
		},
	})
	require.NoError(t, err)

	output, err := stage.Execute(testPipelineContext(), []pipeline.FileInfo{pipeline.NewVideoFileInfo(localFile)})
	require.NoError(t, err)
	assert.Empty(t, output)
	assert.Equal(t, []byte("audio-data"), uploaded)
	assert.NoFileExists(t, localFile)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"MKCOL", "MKCOL", "MKCOL", http.MethodPut}, methods)
	assert.Equal(t, "/dav-root/归档/哔哩哔哩/测试主播/record.aac", paths[len(paths)-1])
}

func TestWebDAVUploadStageKeepsLocalFileWhenUploadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "MKCOL" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.Error(w, "storage full", http.StatusInsufficientStorage)
	}))
	defer server.Close()

	localFile := filepath.Join(t.TempDir(), "record.flv")
	require.NoError(t, os.WriteFile(localFile, []byte("video-data"), 0o644))

	stage, err := NewWebDAVUploadStage(pipeline.StageConfig{
		Options: map[string]any{
			pipeline.OptionURL:         server.URL,
			pipeline.OptionDeleteAfter: true,
		},
	})
	require.NoError(t, err)

	_, err = stage.Execute(testPipelineContext(), []pipeline.FileInfo{pipeline.NewVideoFileInfo(localFile)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 507")
	assert.FileExists(t, localFile)
}

func TestNewWebDAVUploadStageRejectsInvalidURL(t *testing.T) {
	_, err := NewWebDAVUploadStage(pipeline.StageConfig{
		Options: map[string]any{pipeline.OptionURL: "ftp://example.com/dav"},
	})
	require.Error(t, err)
}

func TestWebDAVUploadStageRendersDateTemplate(t *testing.T) {
	stageValue, err := NewWebDAVUploadStage(pipeline.StageConfig{
		Options: map[string]any{
			pipeline.OptionURL:          "https://dav.example.com",
			pipeline.OptionPathTemplate: `archive/{{ now | date "2006" }}/{{ .RoomName }}.{{ .Ext }}`,
		},
	})
	require.NoError(t, err)

	stage := stageValue.(*WebDAVUploadStage)
	rendered, err := stage.renderRemotePath(testPipelineContext(), pipeline.NewVideoFileInfo("record.mp4"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(rendered, "archive/20"))
	assert.True(t, strings.HasSuffix(rendered, "/测试房间.mp4"))
}

func testPipelineContext() *pipeline.PipelineContext {
	return &pipeline.PipelineContext{
		Ctx: context.Background(),
		RecordInfo: pipeline.RecordInfo{
			Platform: "哔哩哔哩",
			HostName: "测试主播",
			RoomName: "测试房间",
		},
		Logger: livelogger.New(livelogger.DefaultBufferSize, logrus.Fields{"test": "webdav"}),
	}
}
