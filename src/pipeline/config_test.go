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
