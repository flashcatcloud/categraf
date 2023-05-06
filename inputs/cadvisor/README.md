## cadvisor

cadvisor 采集插件， 采集cadvisor 数据，如果是通过kubelet采集，可以附加pod的label和annotation

## Configuration

```toml
# # collect interval
# interval = 15

[[instances]]
# 填写kubelet的ip和port
url = "https://1.2.3.4:10250/metrics/cadvisor"
# 如果path为空, 会自动补齐为/metrics/cadvisor
# url = "https://1.2.3.4:10250"
# 如果是通过kubelet采集，可以附加pod的label和annotation
type = "kubelet"

# 直接采集cadvisor , type 设置为cadvisor
#url = "http://1.2.3.4:8080/metrics"
#type = "cadvisor"

# url_label_key 和 url_label_value 用法参加下面说明
url_label_key = "instance"
url_label_value = "{{.Host}}"
# # 认证的token 或者token file
#bearer_token_string = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjE2YU9MSjNlQUlHenhCWWlVaHpESFVUYWhfVUh6RngzMFFfUVR2TGJsUEkifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJuOWV2NiIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJuaWdodGluZ2FsZS1jYXRlZ3JhZi12Ni10b2tlbi0yNmNuZCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJuaWdodGluZ2FsZS1jYXRlZ3JhZi12NiIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6IjNjNTVjZTNmLTU2ZDEtNDhhNy1iNjUyLTQwNDk5YTU4YzlmMyIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpuOWV2NjpuaWdodGluZ2FsZS1jYXRlZ3JhZi12NiJ9.oQsXDiIxuu6VYmxed5Y32ivz_VxDRzCqZB2w9VA85y0oSSuy6Ae5dyp36TJLmPudHAP6tA7UFAWix8O-p2rZVgjIErPVvZCqCgiLxJ04rZUbx_r4a5rtZGHPdmIa68GjOajjrjMp9cI65ExmPyTPAQbwTQLkZVeRzr0hUlK9xxE76eeBO_C5HViCwTCZ7ExJgNKCkkV_e2YRzPtr3CYTPZN_rbhGemc1_jiYSt-C7nJat9QQThPn-R5Lj0fuxiH2aejUpBhmAwUiaIPSehA1IWjGMvg1LpsZHP3Q213j_XQWhGCCmrkDWyuo8PfPCa17Soby3NzVkS_tS0yKwEaLPA"
bearer_token_file = "/path/to/token/file"

# 需要忽略的label key
ignore_label_keys = ["id","name", "container_label*"]
# 只采集那些label key, 建议保持为空，采集所有的label。 优先级高于ignore_label_keys。
#choose_label_keys = ["*"]

timeout = "3s"

# # Optional TLS Config
# # 想跳过自签证书，use_tls 记得要配置为true
use_tls = true
# tls_min_version = "1.2"
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
insecure_skip_verify = true
```

## url_label_key 和 url_label_value 用法
```toml
# 从URL中提取Host部分，放到instancelabel中 
# 假设 url =https://1.2.3.4:10250/metrics/cadvisor 
# 最终附加的label为 instance=1.2.3.4:10250

url_label_key = "instance" 
url_label_value = "{{.Host}}"
```

如果 HTTP scheme 部分和 `/metrics` Path 部分都想取，可以这么写：

```toml
url_label_value = "{{.Scheme}}://{{.Host}}{{.Path}}"
```

相关变量是用这个方法生成的，供大家参考：

```go
func (ul *UrlLabel) GenerateLabel(u *url.URL) (string, string, error) {
	if ul.LabelValue == "" {
		return ul.LabelKey, u.String(), nil
	}

	dict := map[string]string{
		"Scheme":   u.Scheme,
		"Host":     u.Host,
		"Hostname": u.Hostname(),
		"Port":     u.Port(),
		"Path":     u.Path,
		"Query":    u.RawQuery,
		"Fragment": u.Fragment,
	}

	var buffer bytes.Buffer
	err := ul.LabelValueTpl.Execute(&buffer, dict)
	if err != nil {
		return "", "", err
	}

	return ul.LabelKey, buffer.String(), nil
}
```