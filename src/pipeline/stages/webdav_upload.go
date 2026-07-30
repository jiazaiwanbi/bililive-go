package stages

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/bililive-go/bililive-go/src/pipeline"
)

const defaultWebDAVPathTemplate = "/录播归档/{{ .Platform }}/{{ .HostName }}/{{ .FileName }}"

// WebDAVUploadStage 将后处理产物上传到 WebDAV。
type WebDAVUploadStage struct {
	config       pipeline.StageConfig
	baseURL      *url.URL
	username     string
	password     string
	pathTemplate string
	deleteAfter  bool
	client       *http.Client
	commands     []string
	logs         string
}

// NewWebDAVUploadStage 创建 WebDAV 上传阶段。
func NewWebDAVUploadStage(config pipeline.StageConfig) (pipeline.Stage, error) {
	rawURL := strings.TrimSpace(config.GetStringOption(pipeline.OptionURL, ""))
	baseURL, err := url.Parse(rawURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, fmt.Errorf("WebDAV 地址必须是有效的 HTTP(S) URL")
	}

	pathTemplate := strings.TrimSpace(config.GetStringOption(pipeline.OptionPathTemplate, ""))
	if pathTemplate == "" {
		pathTemplate = defaultWebDAVPathTemplate
	}

	return &WebDAVUploadStage{
		config:       config,
		baseURL:      baseURL,
		username:     config.GetStringOption(pipeline.OptionUsername, ""),
		password:     config.GetStringOption(pipeline.OptionPassword, ""),
		pathTemplate: pathTemplate,
		deleteAfter:  config.GetBoolOption(pipeline.OptionDeleteAfter, false),
		client:       http.DefaultClient,
	}, nil
}

func (s *WebDAVUploadStage) Name() string {
	return pipeline.StageNameWebDAVUpload
}

func (s *WebDAVUploadStage) Execute(ctx *pipeline.PipelineContext, input []pipeline.FileInfo) ([]pipeline.FileInfo, error) {
	if len(input) == 0 {
		s.logs = "没有输入文件"
		return input, nil
	}

	output := make([]pipeline.FileInfo, 0, len(input))
	for _, file := range input {
		fileInfo, err := os.Stat(file.Path)
		if err != nil {
			return input, fmt.Errorf("读取待上传文件 %q 失败: %w", file.Path, err)
		}
		if fileInfo.IsDir() {
			return input, fmt.Errorf("WebDAV 不支持上传目录: %s", file.Path)
		}

		remotePath, err := s.renderRemotePath(ctx, file)
		if err != nil {
			return input, err
		}
		if err := s.createRemoteDirectories(ctx, remotePath); err != nil {
			return input, err
		}
		if err := s.uploadFile(ctx, file.Path, remotePath, fileInfo.Size()); err != nil {
			return input, err
		}

		s.commands = append(s.commands, fmt.Sprintf("PUT %s -> %s", file.Path, s.remoteURL(remotePath)))
		s.logs += fmt.Sprintf("上传成功: %s -> %s\n", filepath.Base(file.Path), remotePath)
		ctx.Logger.Infof("WebDAV 上传成功: %s -> %s", file.Path, remotePath)

		if s.deleteAfter {
			if err := os.Remove(file.Path); err != nil {
				return input, fmt.Errorf("WebDAV 上传成功，但删除本地文件 %q 失败: %w", file.Path, err)
			}
			s.logs += fmt.Sprintf("已删除本地文件: %s\n", file.Path)
			continue
		}

		output = append(output, file)
	}

	return output, nil
}

func (s *WebDAVUploadStage) renderRemotePath(ctx *pipeline.PipelineContext, file pipeline.FileInfo) (string, error) {
	tmpl, err := template.New("webdav_path").Funcs(template.FuncMap{
		"now": time.Now,
		"date": func(format string, value time.Time) string {
			return value.Format(format)
		},
	}).Parse(s.pathTemplate)
	if err != nil {
		return "", fmt.Errorf("解析 WebDAV 远程路径模板失败: %w", err)
	}

	ext := strings.TrimPrefix(filepath.Ext(file.Path), ".")
	data := struct {
		Platform string
		HostName string
		RoomName string
		FileName string
		Ext      string
	}{
		Platform: ctx.RecordInfo.Platform,
		HostName: ctx.RecordInfo.HostName,
		RoomName: ctx.RecordInfo.RoomName,
		FileName: filepath.Base(file.Path),
		Ext:      ext,
	}

	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("渲染 WebDAV 远程路径模板失败: %w", err)
	}

	remotePath := strings.ReplaceAll(rendered.String(), "\\", "/")
	remotePath = strings.TrimPrefix(path.Clean("/"+remotePath), "/")
	if remotePath == "" || remotePath == "." || strings.HasSuffix(remotePath, "/") {
		return "", fmt.Errorf("WebDAV 远程路径必须包含文件名")
	}
	return remotePath, nil
}

func (s *WebDAVUploadStage) createRemoteDirectories(ctx *pipeline.PipelineContext, remotePath string) error {
	parts := strings.Split(remotePath, "/")
	for i := 1; i < len(parts); i++ {
		dirPath := strings.Join(parts[:i], "/")
		req, err := http.NewRequestWithContext(ctx.Ctx, "MKCOL", s.remoteURL(dirPath), nil)
		if err != nil {
			return fmt.Errorf("创建 WebDAV 目录请求失败: %w", err)
		}
		s.setAuth(req)

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("创建 WebDAV 目录 %q 失败: %w", dirPath, err)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		closeErr := resp.Body.Close()
		if closeErr != nil {
			return fmt.Errorf("关闭 WebDAV 响应失败: %w", closeErr)
		}
		if resp.StatusCode != http.StatusCreated &&
			resp.StatusCode != http.StatusOK &&
			resp.StatusCode != http.StatusNoContent &&
			resp.StatusCode != http.StatusMethodNotAllowed {
			return fmt.Errorf("创建 WebDAV 目录 %q 失败: HTTP %d", dirPath, resp.StatusCode)
		}
	}
	return nil
}

func (s *WebDAVUploadStage) uploadFile(ctx *pipeline.PipelineContext, localPath, remotePath string, size int64) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开待上传文件 %q 失败: %w", localPath, err)
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodPut, s.remoteURL(remotePath), file)
	if err != nil {
		return fmt.Errorf("创建 WebDAV 上传请求失败: %w", err)
	}
	req.ContentLength = size
	if contentType := mime.TypeByExtension(filepath.Ext(localPath)); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	s.setAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("上传 %q 到 WebDAV 失败: %w", localPath, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("上传 %q 到 WebDAV 失败: HTTP %d: %s", localPath, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *WebDAVUploadStage) remoteURL(remotePath string) string {
	target := *s.baseURL
	target.Path = path.Join(s.baseURL.Path, remotePath)
	target.RawPath = ""
	return target.String()
}

func (s *WebDAVUploadStage) setAuth(req *http.Request) {
	if s.username != "" || s.password != "" {
		req.SetBasicAuth(s.username, s.password)
	}
}

func (s *WebDAVUploadStage) GetCommands() []string {
	return s.commands
}

func (s *WebDAVUploadStage) GetLogs() string {
	return s.logs
}
