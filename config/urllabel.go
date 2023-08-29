package config

import (
	"bytes"
	"log"
	"net/url"
	"strings"
	"text/template"
)

type UrlLabel struct {
	LabelKey      string `toml:"url_label_key"`
	LabelValue    string `toml:"url_label_value"`
	LabelValueTpl *template.Template

	LabelPair    map[string]string  `toml:"url_label_pair"`
	LabelPairTpl *template.Template `toml:"-"`
}

func (ul *UrlLabel) PrepareUrlTemplate() error {
	if ul.LabelKey == "-" && len(ul.LabelPair) == 0 {
		return nil
	}
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
	if len(ul.LabelPair) > 0 {
		var err error
		value := ""
		for k, v := range ul.LabelPair {
			if v == "" || v == " " {
				delete(ul.LabelPair, k)
				continue
			}
			if len(value) != 0 {
				value = value + "||" + k + "=" + v
			} else {
				value = k + "=" + v
			}
		}
		if Config.DebugMode {
			log.Printf("D! label pair tpl:%s", value)
		}
		ul.LabelPairTpl, err = template.New("pair").Parse(value)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ul *UrlLabel) GenerateLabel(u *url.URL) (map[string]string, error) {
	ret := make(map[string]string)
	if ul.LabelKey == "-" && len(ul.LabelPair) == 0 {
		return ret, nil
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
	if ul.LabelKey != "-" {
		if ul.LabelValue != "" {
			err := ul.LabelValueTpl.Execute(&buffer, dict)
			if err != nil {
				return ret, err
			}
			ret[ul.LabelKey] = buffer.String()
		} else {
			ret[ul.LabelKey] = u.String()
		}
		buffer.Reset()
	}
	if len(ul.LabelPair) > 0 {
		var buffer bytes.Buffer
		err := ul.LabelPairTpl.Execute(&buffer, dict)
		if err != nil {
			return ret, err
		}
		pairs := strings.Split(buffer.String(), "||")
		for idx := range pairs {
			kvs := strings.SplitN(pairs[idx], "=", 2)
			if len(kvs) != 2 {
				continue
			}
			if Config.DebugMode {
				log.Printf("D! label pairs after rendering: %s=%s", kvs[0], kvs[1])
			}
			ret[kvs[0]] = kvs[1]
		}
		buffer.Reset()
	}

	return ret, nil
}
