package pipeline

import (
	"testing"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertLegacyConfigIncludesWebDAVUpload(t *testing.T) {
	config := ConvertLegacyConfig(&configs.OnRecordFinished{
		ConvertToMp4: true,
		WebDAVUpload: configs.WebDAVUpload{
			Enable:            true,
			URL:               "https://dav.example.com/root",
			Username:          "user",
			Password:          "secret",
			RemotePathTmpl:    "/archive/{{ .FileName }}",
			DeleteAfterUpload: true,
		},
	})

	require.Len(t, config.Stages, 2)
	assert.Equal(t, StageNameConvertMp4, config.Stages[0].Name)

	webdavStage := config.Stages[1]
	assert.Equal(t, StageNameWebDAVUpload, webdavStage.Name)
	assert.Equal(t, "https://dav.example.com/root", webdavStage.Options[OptionURL])
	assert.Equal(t, "user", webdavStage.Options[OptionUsername])
	assert.Equal(t, "secret", webdavStage.Options[OptionPassword])
	assert.Equal(t, "/archive/{{ .FileName }}", webdavStage.Options[OptionPathTemplate])
	assert.Equal(t, true, webdavStage.Options[OptionDeleteAfter])
}

func TestConvertLegacyConfigConvertsAACBeforeWebDAV(t *testing.T) {
	config := ConvertLegacyConfig(&configs.OnRecordFinished{
		ConvertAACToM4A:       true,
		DeleteAACAfterConvert: true,
		WebDAVUpload: configs.WebDAVUpload{
			Enable: true,
			URL:    "https://dav.example.com/root",
		},
	})

	require.Len(t, config.Stages, 2)
	assert.Equal(t, StageNameConvertM4A, config.Stages[0].Name)
	assert.Equal(t, true, config.Stages[0].Options[OptionDeleteSource])
	assert.Equal(t, StageNameWebDAVUpload, config.Stages[1].Name)
}

func TestConvertLegacyConfigPreservesUploadTimingAndAudio(t *testing.T) {
	for _, timing := range []configs.UploadTiming{configs.UploadTimingImmediate, configs.UploadTimingAfterProcess} {
		t.Run(string(timing), func(t *testing.T) {
			cfg := ConvertLegacyConfig(&configs.OnRecordFinished{
				FixFlvAtFirst:   true,
				ConvertAACToM4A: true,
				UploadTiming:    timing,
				CloudUpload:     configs.CloudUpload{Enable: true, StorageName: "archive", DeleteAllAfterUpload: true, UploadSubtitles: true},
				WebDAVUpload:    configs.WebDAVUpload{Enable: true, URL: "https://dav.example.com/root"},
			})
			names := make([]string, 0, len(cfg.Stages))
			for _, stage := range cfg.Stages {
				names = append(names, stage.Name)
				if stage.Name == StageNameCloudUpload {
					assert.Contains(t, stage.GetStringSliceOption(OptionFileTypes), string(FileTypeAudio))
					assert.True(t, stage.GetBoolOption(OptionDeleteAllAfter, false))
					assert.True(t, stage.GetBoolOption(OptionUploadSubtitles, false))
				}
			}
			if timing == configs.UploadTimingImmediate {
				assert.Equal(t, []string{StageNameCloudUpload, StageNameFixFlv, StageNameConvertM4A, StageNameWebDAVUpload}, names)
			} else {
				assert.Equal(t, []string{StageNameFixFlv, StageNameConvertM4A, StageNameCloudUpload, StageNameWebDAVUpload}, names)
			}
		})
	}
}
