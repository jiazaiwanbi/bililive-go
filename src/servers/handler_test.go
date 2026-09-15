package servers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
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

func TestGetPlatformStatsReportsDisabledRateLimit(t *testing.T) {
	cfg := configs.NewConfig()
	cfg.Interval = 1
	cfg.PlatformConfigs[configs.PlatformKeyDouyin] = configs.PlatformConfig{
		Name:                 "抖音",
		MinAccessIntervalSec: 60,
	}
	cfg.LiveRooms = []configs.LiveRoom{{
		Url:         "https://live.douyin.com/123456",
		IsListening: true,
	}}
	cfg.RefreshLiveRoomIndexCache()
	configs.SetCurrentConfig(cfg)
	t.Cleanup(func() { configs.SetCurrentConfig(configs.NewConfig()) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/config/platforms", nil)
	request = request.WithContext(context.WithValue(request.Context(), instance.Key, &instance.Instance{}))
	getPlatformStats(recorder, request)

	assert.Equal(t, 200, recorder.Code)
	var response struct {
		PlatformRateLimitEnabled bool `json:"platform_rate_limit_enabled"`
		Platforms                []struct {
			PlatformKey    string `json:"platform_key"`
			WarningMessage string `json:"warning_message"`
		} `json:"platforms"`
	}
	assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.PlatformRateLimitEnabled)
	if assert.Len(t, response.Platforms, 1) {
		assert.Equal(t, configs.PlatformKeyDouyin, response.Platforms[0].PlatformKey)
		assert.Empty(t, response.Platforms[0].WarningMessage)
	}
}
