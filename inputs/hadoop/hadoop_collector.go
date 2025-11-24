package hadoop

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/emirpasic/gods/lists/singlylinkedlist"
	"github.com/jcmturner/gokrb5/v8/client"
	kconfig "github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

type ComponentOption struct {
	ProcessName           string   `toml:"processName"`
	Port                  int      `toml:"port"`
	Name                  string   `toml:"name"`
	AllowRecursiveParse   bool     `default:"true" toml:"allowRecursiveParse"`
	AllowMetricsWhiteList bool     `default:"true" toml:"allowMetricsWhiteList"`
	JmxSuffix             string   `default:"/jmx" toml:"jmxUrlSuffix"`
	WhiteList             []string `toml:"white_list"`
}

type Component struct {
	CommonConfig
	ComponentOption
	metricsWhiteList *singlylinkedlist.List
}

type MetricsData struct {
	Name      string
	Value     float64
	LabelPair map[string]string
}

var hostName = getHostName()

func (c *Component) ComposeMetricUrl() string {
	return fmt.Sprintf("http://%s:%d%s", hostName, c.Port, c.JmxSuffix)
}

func (c *Component) IsProcessExisted() bool {
	cmdStr := fmt.Sprintf("ps -ef |grep %s |grep -v grep", c.ProcessName)
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	res, _ := cmd.Output()
	return len(string(res)) > 0
}

func (c *Component) GetData(requestURL string) (map[string]interface{}, error) {

	if c.UseSASL {
		saslMechanism := strings.ToLower(c.SaslMechanism)
		switch saslMechanism {
		case "gssapi":
			if c.KerberosAuthType == "keytabAuth" {
				return c.getDataWithSpnego(requestURL)
			}
		case "plain":
		default:
			return nil, fmt.Errorf(
				`invalid sasl mechanism "%s": can only be "scram-sha256", "scram-sha512", "gssapi" or "plain"`,
				saslMechanism,
			)
		}
	}

	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("get data from %s failed: %v", requestURL, err)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read data from %s failed: %v", requestURL, err)
	}

	var f = make(map[string]interface{})
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse json failed: %v", err)
	}
	return f, nil
}

func (c *Component) FetchData(data map[string]interface{}) ([]MetricsData, error) {
	dataList := make([]MetricsData, 0)
	var recursiveFetch func(data interface{}, hierarchy []string) interface{}

	recursiveFetch = func(data interface{}, hierarchy []string) interface{} {
		switch dataType := reflect.ValueOf(data).Kind(); dataType {
		case reflect.Map:
			labels := c.generateLabelPairs(data.(map[string]interface{}))
			for metricsKey, metricsValue := range data.(map[string]interface{}) {
				if hierarchy == nil {
					hierarchy = make([]string, 0)
				}
				clearedMetricsKey := metricsKeyClear(metricsKey)
				valueKind := reflect.ValueOf(metricsValue).Kind()

				if valueKind == reflect.Map || valueKind == reflect.Slice {
					if c.AllowRecursiveParse {
						hierarchy = append(hierarchy, clearedMetricsKey)
						recursiveFetch(metricsValue, hierarchy)
					}
				} else {
					var finalKey string
					keyArr := append(hierarchy, clearedMetricsKey)
					finalKey = strings.Join(keyArr, "_")
					hierarchy = nil

					metricsData, filterErr := c.filterMetricsValue(finalKey, metricsValue)
					if filterErr == nil {
						numberPrefixRegex := regexp.MustCompile(`^\d`)
						if numberPrefixRegex.Match([]byte(finalKey)) {
							finalKey = "num_" + finalKey
						}
						metricsData.Name = finalKey
						metricsData.LabelPair = labels
						dataList = append(dataList, metricsData)
					}
				}
			}
		case reflect.Slice:
			for _, item := range data.([]interface{}) {
				itemKind := reflect.ValueOf(item).Kind()
				if itemKind == reflect.Map || itemKind == reflect.Slice {
					recursiveFetch(item, hierarchy)
				}
			}
		}
		return nil
	}

	if value, ok := data["beans"]; ok {
		var nameList = value.([]interface{})
		recursiveFetch(nameList, nil)
	} else {
		recursiveFetch(data, nil)
	}
	return dataList, nil
}

func (c *Component) generateLabelPairs(nameDataMap map[string]interface{}) map[string]string {
	labels := make(map[string]string)
	if dictName, ok := nameDataMap["name"]; ok {
		dictNameStr := dictName.(string)
		if len(dictNameStr) > 0 {
			labels["name"] = dictNameStr
		}
	}
	return labels
}

func (c *Component) filterMetricsValue(clearedMetricsKey string, metricsValue interface{}) (MetricsData, error) {
	whiteList := c.metricsWhiteList
	metricsData := MetricsData{}
	strValue := fmt.Sprint(metricsValue)

	isInWhiteList := whiteList.Any(func(index int, value interface{}) bool {
		return strings.Compare(clearedMetricsKey, value.(string)) == 0
	})

	if c.AllowMetricsWhiteList && !isInWhiteList {
		return MetricsData{}, errors.New("not in WhiteList")
	}

	floatValue, err := strconv.ParseFloat(strValue, 64)
	if err != nil {
		return MetricsData{}, errors.New("value is not in numeric format")
	}
	metricsData.Value = floatValue
	return metricsData, nil
}

func (c *Component) Initialize(commonConfig CommonConfig) error {
	c.metricsWhiteList = singlylinkedlist.New()
	c.CommonConfig = commonConfig
	// 使用配置的白名单
	if len(c.ComponentOption.WhiteList) > 0 {
		for _, metricsKey := range c.ComponentOption.WhiteList {
			clearedMetricsKey := metricsKeyClear(metricsKey)
			c.metricsWhiteList.Add(clearedMetricsKey)
		}
		return nil
	}

	return nil
}

func metricsKeyClear(metricsKey string) string {
	if strings.IndexByte(metricsKey, '.') != -1 {
		metricsKey = strings.ReplaceAll(metricsKey, ".", "_")
	}
	if strings.IndexByte(metricsKey, '-') != -1 {
		metricsKey = strings.ReplaceAll(metricsKey, "-", "_")
	}
	return metricsKey
}

func getHostName() string {
	hostName, _ := os.Hostname()
	return hostName
}

func (e *Component) getDataWithSpnego(requestURL string) (map[string]interface{}, error) {
	SaslUsername := e.SaslUsername
	if SaslUsername == "HTTP/_HOST" {
		SaslUsername = fmt.Sprintf("HTTP/%s", hostName)
	}

	kt, err := keytab.Load(e.KeyTabPath)
	if err != nil {
		errInfo := fmt.Sprintf("could not load client keytab %s", err)
		return nil, errors.New(errInfo)
	}
	// Load the client krb5 config
	krb5ConfData, err := os.Open(e.KerberosConfigPath)
	krb5Conf, err := kconfig.NewFromReader(krb5ConfData)

	if err != nil {
		errInfo := fmt.Sprintf("could not load krb5.conf %s", err)
		return nil, errors.New(errInfo)
	}
	cl := client.NewWithKeytab(SaslUsername, krb5Conf.Realms[0].Realm, kt, krb5Conf, client.DisablePAFXFAST(e.SaslDisablePAFXFast))
	// Log in the client
	err = cl.Login()
	if err != nil {
		errInfo := fmt.Sprintf("could not login client %s", err)
		return nil, errors.New(errInfo)
	}
	// Form the request
	r, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		errInfo := fmt.Sprintf("could create request %s", err)
		return nil, errors.New(errInfo)
	}

	spnegoCl := spnego.NewClient(cl, nil, SaslUsername)
	resp, err := spnegoCl.Do(r)
	if err != nil {
		errInfo := fmt.Sprintf("error making spnego request %s ,err is %s", requestURL, err)
		return nil, errors.New(errInfo)
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errInfo := fmt.Sprintf("error read data from response body %s", err)
		return nil, errors.New(errInfo)
	}
	var f = make(map[string]interface{})
	err = json.Unmarshal(data, &f)
	if err != nil {
		return nil, errors.New("parse json failed")
	}
	return f, nil

}
