package config

import (
	"bytes"
	"net/url"
	"text/template"
)

type UrlLabel struct {
	LabelKey      string `toml:"url_label_key"`
	LabelValue    string `toml:"url_label_value"`
	LabelValueTpl *template.Template
}

func (ul *UrlLabel) PrepareUrlTemplate() error {
	if ul.LabelKey == "" {
		ul.LabelKey = "instance"
	}

	if ul.LabelValue != "" {
		var err error
		ul.LabelValueTpl, err = template.New("v").Parse(ul.LabelValue)
		if err != nil {
			return err
		}
	}

	return nil
}

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
