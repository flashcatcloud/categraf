package nvidia_smi

// This is a fork of https://github.com/utkuozdemir/nvidia_gpu_exporter

import (
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "nvidia_smi"

type GPUStats struct {
	config.Interval

	NvidiaSmiCommand string `toml:"nvidia_smi_command"`
	QueryFieldNames  string `toml:"query_field_names"`

	qFields               []qField
	qFieldToMetricInfoMap map[qField]MetricInfo
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &GPUStats{}
	})
}

func (s *GPUStats) Prefix() string                  { return inputName }
func (s *GPUStats) Drop()                           {}
func (s *GPUStats) GetInstances() []inputs.Instance { return nil }

func (s *GPUStats) Init() error {
	if s.NvidiaSmiCommand == "" {
		return types.ErrInstancesEmpty
	}

	qFieldsOrdered, qFieldToRFieldMap, err := buildQFieldToRFieldMap(s.QueryFieldNames, s.NvidiaSmiCommand)
	if err != nil {
		return err
	}

	s.qFields = qFieldsOrdered
	s.qFieldToMetricInfoMap = buildQFieldToMetricInfoMap(qFieldToRFieldMap)

	return nil
}

func (s *GPUStats) Gather(slist *list.SafeList) {
	if s.NvidiaSmiCommand == "" {
		return
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample("scrape_use_seconds", use))
	}(begun)

	currentTable, err := scrape(s.qFields, s.NvidiaSmiCommand)
	if err != nil {
		slist.PushFront(types.NewSample("scraper_up", 0))
		return
	}

	slist.PushFront(types.NewSample("scraper_up", 1))

	for _, currentRow := range currentTable.rows {
		uuid := strings.TrimPrefix(strings.ToLower(currentRow.qFieldToCells[uuidQField].rawValue), "gpu-")
		name := currentRow.qFieldToCells[nameQField].rawValue
		driverModelCurrent := currentRow.qFieldToCells[driverModelCurrentQField].rawValue
		driverModelPending := currentRow.qFieldToCells[driverModelPendingQField].rawValue
		vBiosVersion := currentRow.qFieldToCells[vBiosVersionQField].rawValue
		driverVersion := currentRow.qFieldToCells[driverVersionQField].rawValue

		slist.PushFront(types.NewSample("gpu_info", 1, map[string]string{
			"uuid":                 uuid,
			"name":                 name,
			"driver_model_current": driverModelCurrent,
			"driver_model_pending": driverModelPending,
			"vbios_version":        vBiosVersion,
			"driver_version":       driverVersion,
		}))

		for _, currentCell := range currentRow.cells {
			metricInfo := s.qFieldToMetricInfoMap[currentCell.qField]

			num, err := transformRawValue(currentCell.rawValue, metricInfo.valueMultiplier)
			if err != nil {
				if config.Config.DebugMode {
					log.Println("D! failed to transform gpu field:", currentCell.qField, "raw value:", currentCell.rawValue, "error:", err)
				}
				continue
			}

			slist.PushFront(types.NewSample(metricInfo.metricName, num, map[string]string{"uuid": uuid}))
		}
	}
}
