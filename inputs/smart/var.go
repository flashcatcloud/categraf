package smart

import (
	"fmt"
	"regexp"
	"strings"

	"flashcat.cloud/categraf/types"
)

const (
	intelVID = "0x8086"
)

var (
	// Device Model:     APPLE SSD SM256E
	// Product:              HUH721212AL5204
	// Model Number: TS128GMTE850
	modelInfo = regexp.MustCompile(`^(Device Model|Product|Model Number):\s+(.*)$`)
	// Serial Number:    S0X5NZBC422720
	serialInfo = regexp.MustCompile(`(?i)^Serial Number:\s+(.*)$`)
	// LU WWN Device Id: 5 002538 655584d30
	wwnInfo = regexp.MustCompile(`^LU WWN Device Id:\s+(.*)$`)
	// User Capacity:    251,000,193,024 bytes [251 GB]
	userCapacityInfo = regexp.MustCompile(`^User Capacity:\s+([0-9,]+)\s+bytes.*$`)
	// SMART support is: Enabled
	smartEnabledInfo = regexp.MustCompile(`^SMART support is:\s+(\w+)$`)
	// Power mode is:    ACTIVE or IDLE or Power mode was:   STANDBY
	powermodeInfo = regexp.MustCompile(`^Power mode \w+:\s+(\w+)`)
	// Device is in STANDBY mode
	standbyInfo = regexp.MustCompile(`^Device is in\s+(\w+)`)
	// SMART overall-health self-assessment test result: PASSED
	// SMART Health Status: OK
	// PASSED, FAILED, UNKNOWN
	smartOverallHealth = regexp.MustCompile(`^(SMART overall-health self-assessment test result|SMART Health Status):\s+(\w+).*$`)

	// sasNVMeAttr is a SAS or NVMe SMART attribute
	sasNVMeAttr = regexp.MustCompile(`^([^:]+):\s+(.+)$`)

	// ID# ATTRIBUTE_NAME          FLAGS    VALUE WORST THRESH FAIL RAW_VALUE
	//   1 Raw_Read_Error_Rate     -O-RC-   200   200   000    -    0
	//   5 Reallocated_Sector_Ct   PO--CK   100   100   000    -    0
	// 192 Power-Off_Retract_Count -O--C-   097   097   000    -    14716
	attribute = regexp.MustCompile(`^\s*([0-9]+)\s(\S+)\s+([-P][-O][-S][-R][-C][-K])\s+([0-9]+)\s+([0-9]+)\s+([0-9-]+)\s+([-\w]+)\s+([\w\+\.]+).*$`)

	//  Additional Instance Log for NVME device:nvme0 namespace-id:ffffffff
	// nvme version 1.14+ metrics:
	// ID             KEY                                 Normalized     Raw
	// 0xab    program_fail_count                             100         0

	// nvme deprecated metric format:
	//	key                               normalized raw
	//	program_fail_count              : 100%       0

	// REGEX patter supports deprecated metrics (nvme-cli version below 1.14) and metrics from nvme-cli 1.14 (and above).
	intelExpressionPattern = regexp.MustCompile(`^([A-Za-z0-9_\s]+)[:|\s]+(\d+)[%|\s]+(.+)`)

	//	vid     : 0x8086
	//	sn      : CFGT53260XSP8011P
	nvmeIDCtrlExpressionPattern = regexp.MustCompile(`^([\w\s]+):([\s\w]+)`)

	// Format from nvme-cli 1.14 (and above) gives ID and KEY, this regex is for separating id from key.
	//  ID			  KEY
	// 0xab    program_fail_count
	nvmeIDSeparatePattern = regexp.MustCompile(`^([A-Za-z0-9_]+)(.+)`)

	deviceFieldIds = map[string]string{
		"1":   "read_error_rate",
		"5":   "reallocated_sectors_count",
		"7":   "seek_error_rate",
		"10":  "spin_retry_count",
		"184": "end_to_end_error",
		"187": "uncorrectable_errors",
		"188": "command_timeout",
		"190": "temp_c",
		"194": "temp_c",
		"196": "realloc_event_count",
		"197": "pending_sector_count",
		"198": "uncorrectable_sector_count",
		"199": "udma_crc_errors",
		"201": "soft_read_error_rate",
	}

	// There are some fields we're interested in which use the vendor specific device ids
	// so we need to be able to match on name instead
	deviceFieldNames = map[string]string{
		"Percent_Lifetime_Remain": "percent_lifetime_remain",
		"Wear_Leveling_Count":     "wear_leveling_count",
		"Media_Wearout_Indicator": "media_wearout_indicator",
	}

	// to obtain metrics from smartctl
	sasNVMeAttributes = map[string]struct {
		ID    string
		Name  string
		Parse func(fields, deviceFields map[string]interface{}, metric, str string) error
	}{
		"Accumulated start-stop cycles": {
			ID:   "4",
			Name: "Start_Stop_Count",
		},
		"Accumulated load-unload cycles": {
			ID:   "193",
			Name: "Load_Cycle_Count",
		},
		"Current Drive Temperature": {
			ID:    "194",
			Name:  "Temperature_Celsius",
			Parse: parseTemperature,
		},
		"Temperature": {
			ID:    "194",
			Name:  "Temperature_Celsius",
			Parse: parseTemperature,
		},
		"Power Cycles": {
			ID:   "12",
			Name: "Power_Cycle_Count",
		},
		"Power On Hours": {
			ID:   "9",
			Name: "Power_On_Hours",
		},
		"Media and Data Integrity Errors": {
			Name: "Media_and_Data_Integrity_Errors",
		},
		"Error Information Log Entries": {
			Name: "Error_Information_Log_Entries",
		},
		"Critical Warning": {
			Name: "Critical_Warning",
			Parse: func(fields, _ map[string]interface{}, metric, str string) error {
				var value int64
				if _, err := fmt.Sscanf(str, "0x%x", &value); err != nil {
					return err
				}

				fields[metric] = value

				return nil
			},
		},
		"Available Spare": {
			Name:  "Available_Spare",
			Parse: parsePercentageInt,
		},
		"Available Spare Threshold": {
			Name:  "Available_Spare_Threshold",
			Parse: parsePercentageInt,
		},
		"Percentage Used": {
			Name:  "Percentage_Used",
			Parse: parsePercentageInt,
		},
		"Percentage used endurance indicator": {
			Name:  "Percentage_Used",
			Parse: parsePercentageInt,
		},
		"Data Units Read": {
			Name:  "Data_Units_Read",
			Parse: parseDataUnits,
		},
		"Data Units Written": {
			Name:  "Data_Units_Written",
			Parse: parseDataUnits,
		},
		"Host Read Commands": {
			Name:  "Host_Read_Commands",
			Parse: parseCommaSeparatedInt,
		},
		"Host Write Commands": {
			Name:  "Host_Write_Commands",
			Parse: parseCommaSeparatedInt,
		},
		"Controller Busy Time": {
			Name:  "Controller_Busy_Time",
			Parse: parseCommaSeparatedInt,
		},
		"Unsafe Shutdowns": {
			Name:  "Unsafe_Shutdowns",
			Parse: parseCommaSeparatedInt,
		},
		"Warning  Comp. Temperature Time": {
			Name:  "Warning_Temperature_Time",
			Parse: parseCommaSeparatedInt,
		},
		"Critical Comp. Temperature Time": {
			Name:  "Critical_Temperature_Time",
			Parse: parseCommaSeparatedInt,
		},
		"Thermal Temp. 1 Transition Count": {
			Name:  "Thermal_Management_T1_Trans_Count",
			Parse: parseCommaSeparatedInt,
		},
		"Thermal Temp. 2 Transition Count": {
			Name:  "Thermal_Management_T2_Trans_Count",
			Parse: parseCommaSeparatedInt,
		},
		"Thermal Temp. 1 Total Time": {
			Name:  "Thermal_Management_T1_Total_Time",
			Parse: parseCommaSeparatedInt,
		},
		"Thermal Temp. 2 Total Time": {
			Name:  "Thermal_Management_T2_Total_Time",
			Parse: parseCommaSeparatedInt,
		},
		"Temperature Sensor 1": {
			Name:  "Temperature_Sensor_1",
			Parse: parseTemperatureSensor,
		},
		"Temperature Sensor 2": {
			Name:  "Temperature_Sensor_2",
			Parse: parseTemperatureSensor,
		},
		"Temperature Sensor 3": {
			Name:  "Temperature_Sensor_3",
			Parse: parseTemperatureSensor,
		},
		"Temperature Sensor 4": {
			Name:  "Temperature_Sensor_4",
			Parse: parseTemperatureSensor,
		},
		"Temperature Sensor 5": {
			Name:  "Temperature_Sensor_5",
			Parse: parseTemperatureSensor,
		},
		"Temperature Sensor 6": {
			Name:  "Temperature_Sensor_6",
			Parse: parseTemperatureSensor,
		},
		"Temperature Sensor 7": {
			Name:  "Temperature_Sensor_7",
			Parse: parseTemperatureSensor,
		},
		"Temperature Sensor 8": {
			Name:  "Temperature_Sensor_8",
			Parse: parseTemperatureSensor,
		},
	}
	// To obtain Intel specific metrics from nvme-cli version 1.14 and above.
	intelAttributes = map[string]struct {
		ID    string
		Name  string
		Parse func(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error
	}{
		"program_fail_count": {
			Name: "Program_Fail_Count",
		},
		"erase_fail_count": {
			Name: "Erase_Fail_Count",
		},
		"wear_leveling_count": { // previously: "wear_leveling"
			Name: "Wear_Leveling_Count",
		},
		"e2e_error_detect_count": { // previously: "end_to_end_error_detection_count"
			Name: "End_To_End_Error_Detection_Count",
		},
		"crc_error_count": {
			Name: "Crc_Error_Count",
		},
		"media_wear_percentage": { // previously: "timed_workload_media_wear"
			Name: "Media_Wear_Percentage",
		},
		"host_reads": {
			Name: "Host_Reads",
		},
		"timed_work_load": { // previously: "timed_workload_timer"
			Name: "Timed_Workload_Timer",
		},
		"thermal_throttle_status": {
			Name: "Thermal_Throttle_Status",
		},
		"retry_buff_overflow_count": { // previously: "retry_buffer_overflow_count"
			Name: "Retry_Buffer_Overflow_Count",
		},
		"pll_lock_loss_counter": { // previously: "pll_lock_loss_count"
			Name: "Pll_Lock_Loss_Count",
		},
	}
	// to obtain Intel specific metrics from nvme-cli
	intelAttributesDeprecatedFormat = map[string]struct {
		ID    string
		Name  string
		Parse func(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error
	}{
		"program_fail_count": {
			Name: "Program_Fail_Count",
		},
		"erase_fail_count": {
			Name: "Erase_Fail_Count",
		},
		"end_to_end_error_detection_count": {
			Name: "End_To_End_Error_Detection_Count",
		},
		"crc_error_count": {
			Name: "Crc_Error_Count",
		},
		"retry_buffer_overflow_count": {
			Name: "Retry_Buffer_Overflow_Count",
		},
		"wear_leveling": {
			Name:  "Wear_Leveling",
			Parse: parseWearLeveling,
		},
		"timed_workload_media_wear": {
			Name:  "Timed_Workload_Media_Wear",
			Parse: parseTimedWorkload,
		},
		"timed_workload_host_reads": {
			Name:  "Timed_Workload_Host_Reads",
			Parse: parseTimedWorkload,
		},
		"timed_workload_timer": {
			Name: "Timed_Workload_Timer",
			Parse: func(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error {
				return parseCommaSeparatedIntWithAccumulator(slist, fields, tags, metric, strings.TrimSuffix(str, " min"))
			},
		},
		"thermal_throttle_status": {
			Name:  "Thermal_Throttle_Status",
			Parse: parseThermalThrottle,
		},
		"pll_lock_loss_count": {
			Name: "Pll_Lock_Loss_Count",
		},
		"nand_bytes_written": {
			Name:  "Nand_Bytes_Written",
			Parse: parseBytesWritten,
		},
		"host_bytes_written": {
			Name:  "Host_Bytes_Written",
			Parse: parseBytesWritten,
		},
	}

	knownReadMethods = []string{"concurrent", "sequential"}
)
