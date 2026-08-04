package servers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bililive-go/bililive-go/src/configs"
)

func TestGetSoopLiveAuthConfigDoesNotExposeSavedPassword(t *testing.T) {
	cfg := configs.NewConfig()
	cfg.SoopLiveAuth.Username = "tester"
	cfg.SoopLiveAuth.Password = "secret"
	configs.SetCurrentConfig(cfg)

	recorder := httptest.NewRecorder()
	getSoopLiveAuthConfig(recorder, nil)

	assert.Equal(t, 200, recorder.Code)

	var resp commonResp
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	assert.NoError(t, err)

	data, ok := resp.Data.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "tester", data["username"])
	assert.Equal(t, true, data["has_saved_credentials"])
	_, exists := data["password"]
	assert.False(t, exists)
}

func TestApplyConfigUpdatesWebDAVUpload(t *testing.T) {
	cfg := configs.NewConfig()
	err := applyConfigUpdates(cfg, map[string]interface{}{
		"on_record_finished": map[string]interface{}{
			"webdav_upload": map[string]interface{}{
				"enable":              true,
				"url":                 " https://dav.example.com/root/ ",
				"username":            "alice",
				"password":            "secret",
				"remote_path_tmpl":    "/archive/{{ .FileName }}",
				"delete_after_upload": true,
			},
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, configs.WebDAVUpload{
		Enable:            true,
		URL:               "https://dav.example.com/root/",
		Username:          "alice",
		Password:          "secret",
		RemotePathTmpl:    "/archive/{{ .FileName }}",
		DeleteAfterUpload: true,
	}, cfg.OnRecordFinished.WebDAVUpload)
}

func TestApplyConfigUpdatesAACToM4A(t *testing.T) {
	cfg := configs.NewConfig()
	err := applyConfigUpdates(cfg, map[string]interface{}{
		"on_record_finished": map[string]interface{}{
			"convert_aac_to_m4a":       true,
			"delete_aac_after_convert": true,
		},
	})

	assert.NoError(t, err)
	assert.True(t, cfg.OnRecordFinished.ConvertAACToM4A)
	assert.True(t, cfg.OnRecordFinished.DeleteAACAfterConvert)
}
