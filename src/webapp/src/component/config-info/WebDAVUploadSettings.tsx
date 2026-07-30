import React from 'react';
import { Alert, Card, Form, Input, Switch, Tag } from 'antd';
import { CloudUploadOutlined } from '@ant-design/icons';

const { TextArea, Password } = Input;

const validateWebDAVURL = (_: unknown, value?: string) => {
  if (!value) {
    return Promise.resolve();
  }

  try {
    const parsedURL = new URL(value);
    if ((parsedURL.protocol === 'http:' || parsedURL.protocol === 'https:') && parsedURL.hostname) {
      return Promise.resolve();
    }
  } catch {
    // 交由下方统一返回表单错误。
  }

  return Promise.reject(new Error('请输入有效的 HTTP(S) 地址'));
};

interface ConfigFieldProps {
  label: string;
  description?: string;
  children: React.ReactElement;
}

const ConfigField: React.FC<ConfigFieldProps> = ({ label, description, children }) => (
  <div className="config-item" style={{ marginBottom: 16 }}>
    <div className="config-item-label" style={{ marginBottom: 4, fontWeight: 500 }}>{label}</div>
    <div className="config-item-content">
      <div className="config-item-input">{children}</div>
      {description && (
        <div className="config-item-description" style={{ marginTop: 4, color: '#888', fontSize: 12 }}>
          {description}
        </div>
      )}
    </div>
  </div>
);

interface WebDAVUploadSettingsProps {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  config: any;
}

const WebDAVUploadSettings: React.FC<WebDAVUploadSettingsProps> = ({ config }) => {
  const watchedEnabled = Form.useWatch(['on_record_finished', 'webdav_upload', 'enable']);
  const isEnabled = watchedEnabled ?? config.on_record_finished?.webdav_upload?.enable ?? false;

  return (
    <Card
      title={<><CloudUploadOutlined /> WebDAV 自动上传</>}
      size="small"
      style={{ marginBottom: 16 }}
      extra={<Tag color={isEnabled ? 'green' : 'default'}>{isEnabled ? '已启用' : '未启用'}</Tag>}
    >
      <Alert
        message="录制后自动归档"
        description="后处理（修复、转换格式、烧录字幕）完成后上传最终文件。只有 WebDAV 服务端确认上传成功，才会按配置删除本地文件。"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />
      <ConfigField label="启用 WebDAV 上传" description="开启后，录制成功产生的文件会自动上传">
        <Form.Item name={['on_record_finished', 'webdav_upload', 'enable']} valuePropName="checked" noStyle>
          <Switch />
        </Form.Item>
      </ConfigField>
      <ConfigField
        label="WebDAV 地址"
        description="填写 WebDAV 根地址，例如 https://dav.example.com/remote.php/dav/files/username"
      >
        <Form.Item
          name={['on_record_finished', 'webdav_upload', 'url']}
          rules={[{ validator: validateWebDAVURL }]}
          noStyle
        >
          <Input placeholder="https://dav.example.com/dav/" style={{ width: 520 }} />
        </Form.Item>
      </ConfigField>
      <ConfigField label="用户名" description="WebDAV Basic Auth 用户名；服务端不需要认证时可留空">
        <Form.Item name={['on_record_finished', 'webdav_upload', 'username']} noStyle>
          <Input autoComplete="username" style={{ width: 300 }} />
        </Form.Item>
      </ConfigField>
      <ConfigField label="密码" description="密码会保存在 config.yml 中，请限制配置文件访问权限">
        <Form.Item name={['on_record_finished', 'webdav_upload', 'password']} noStyle>
          <Password autoComplete="current-password" style={{ width: 300 }} />
        </Form.Item>
      </ConfigField>
      <ConfigField
        label="远程路径模板"
        description={'支持 {{ .Platform }}、{{ .HostName }}、{{ .RoomName }}、{{ .FileName }}、{{ .Ext }} 和 {{ now | date "2006-01-02" }}'}
      >
        <Form.Item name={['on_record_finished', 'webdav_upload', 'remote_path_tmpl']} noStyle>
          <TextArea
            rows={2}
            placeholder="/录播归档/{{ .Platform }}/{{ .HostName }}/{{ .FileName }}"
            style={{ width: 620 }}
          />
        </Form.Item>
      </ConfigField>
      <ConfigField
        label="上传后删除本地文件"
        description="开启后仅保留 WebDAV 上的文件；上传失败时本地文件不会删除"
      >
        <Form.Item
          name={['on_record_finished', 'webdav_upload', 'delete_after_upload']}
          valuePropName="checked"
          noStyle
        >
          <Switch />
        </Form.Item>
      </ConfigField>
    </Card>
  );
};

export default WebDAVUploadSettings;
