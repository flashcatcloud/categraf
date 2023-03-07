package manager

import (
	"unicode"

	cms20190101 "github.com/alibabacloud-go/cms-20190101/v8/client"
	cms2021101 "github.com/alibabacloud-go/cms-export-20211101/v2/client"
)

type (
	Manager struct {
		cms   *cmsClient
		cmsv2 *cmsV2Client
	}

	cmsClient struct {
		region    string
		endpoint  string
		apikey    string
		apiSecret string

		*cms20190101.Client
	}
	cmsV2Client struct {
		region    string
		endpoint  string
		apikey    string
		apiSecret string

		*cms2021101.Client
	}
)

type Option func(manager *Manager) error

func New(opts ...Option) (*Manager, error) {
	var (
		err error
	)

	m := &Manager{}
	for _, opt := range opts {
		err = opt(m)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

func SnakeCase(in string) string {
	runes := []rune(in)
	length := len(runes)

	var out []rune
	for i := 0; i < length; i++ {
		if runes[i] == '.' {
			continue
		}
		if i > 0 && unicode.IsUpper(runes[i]) && ((i+1 < length && unicode.IsLower(runes[i+1])) || unicode.IsLower(runes[i-1])) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(runes[i]))
	}

	return string(out)
}
