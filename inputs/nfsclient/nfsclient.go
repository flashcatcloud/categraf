package nfsclient

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "nfsclient"

type NfsClient struct {
	config.PluginConfig

	Fullstat          bool     `toml:"fullstat"`
	IncludeMounts     []string `toml:"include_mounts"`
	ExcludeMounts     []string `toml:"exclude_mounts"`
	IncludeOperations []string `toml:"include_operations"`
	ExcludeOperations []string `toml:"exclude_operations"`

	nfs3Ops        map[string]bool
	nfs4Ops        map[string]bool
	mountstatsPath string
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NfsClient{}
	})
}

func (s *NfsClient) Clone() inputs.Input {
	return &NfsClient{}
}

func (s *NfsClient) Name() string {
	return inputName
}

func (s *NfsClient) Init() error {
	var nfs3Fields = []string{
		"NULL",
		"GETATTR",
		"SETATTR",
		"LOOKUP",
		"ACCESS",
		"READLINK",
		"READ",
		"WRITE",
		"CREATE",
		"MKDIR",
		"SYMLINK",
		"MKNOD",
		"REMOVE",
		"RMDIR",
		"RENAME",
		"LINK",
		"READDIR",
		"READDIRPLUS",
		"FSSTAT",
		"FSINFO",
		"PATHCONF",
		"COMMIT",
	}

	var nfs4Fields = []string{
		"NULL",
		"READ",
		"WRITE",
		"COMMIT",
		"OPEN",
		"OPEN_CONFIRM",
		"OPEN_NOATTR",
		"OPEN_DOWNGRADE",
		"CLOSE",
		"SETATTR",
		"FSINFO",
		"RENEW",
		"SETCLIENTID",
		"SETCLIENTID_CONFIRM",
		"LOCK",
		"LOCKT",
		"LOCKU",
		"ACCESS",
		"GETATTR",
		"LOOKUP",
		"LOOKUP_ROOT",
		"REMOVE",
		"RENAME",
		"LINK",
		"SYMLINK",
		"CREATE",
		"PATHCONF",
		"STATFS",
		"READLINK",
		"READDIR",
		"SERVER_CAPS",
		"DELEGRETURN",
		"GETACL",
		"SETACL",
		"FS_LOCATIONS",
		"RELEASE_LOCKOWNER",
		"SECINFO",
		"FSID_PRESENT",
		"EXCHANGE_ID",
		"CREATE_SESSION",
		"DESTROY_SESSION",
		"SEQUENCE",
		"GET_LEASE_TIME",
		"RECLAIM_COMPLETE",
		"LAYOUTGET",
		"GETDEVICEINFO",
		"LAYOUTCOMMIT",
		"LAYOUTRETURN",
		"SECINFO_NO_NAME",
		"TEST_STATEID",
		"FREE_STATEID",
		"GETDEVICELIST",
		"BIND_CONN_TO_SESSION",
		"DESTROY_CLIENTID",
		"SEEK",
		"ALLOCATE",
		"DEALLOCATE",
		"LAYOUTSTATS",
		"CLONE",
		"COPY",
		"OFFLOAD_CANCEL",
		"LOOKUPP",
		"LAYOUTERROR",
		"COPY_NOTIFY",
		"GETXATTR",
		"SETXATTR",
		"LISTXATTRS",
		"REMOVEXATTR",
	}

	nfs3Ops := make(map[string]bool)
	nfs4Ops := make(map[string]bool)

	s.mountstatsPath = s.getMountStatsPath()

	if len(s.IncludeOperations) == 0 {
		for _, Op := range nfs3Fields {
			nfs3Ops[Op] = true
		}
		for _, Op := range nfs4Fields {
			nfs4Ops[Op] = true
		}
	} else {
		for _, Op := range s.IncludeOperations {
			nfs3Ops[Op] = true
		}
		for _, Op := range s.IncludeOperations {
			nfs4Ops[Op] = true
		}
	}

	if len(s.ExcludeOperations) > 0 {
		for _, Op := range s.ExcludeOperations {
			if nfs3Ops[Op] {
				delete(nfs3Ops, Op)
			}
			if nfs4Ops[Op] {
				delete(nfs4Ops, Op)
			}
		}
	}

	s.nfs3Ops = nfs3Ops
	s.nfs4Ops = nfs4Ops

	if len(s.IncludeMounts) > 0 {
		if config.Config.DebugMode {
			log.Println("D! Including these mount patterns:", s.IncludeMounts)
		}
	} else {
		if config.Config.DebugMode {
			log.Println("D! Including all mounts.")
		}
	}

	if len(s.ExcludeMounts) > 0 {
		if config.Config.DebugMode {
			log.Println("D! Excluding these mount patterns:", s.ExcludeMounts)
		}
	} else {
		if config.Config.DebugMode {
			log.Println("D! Not excluding any mounts.")
		}
	}

	if len(s.IncludeOperations) > 0 {
		if config.Config.DebugMode {
			log.Println("D! Including these operations:", s.IncludeOperations)
		}
	} else {
		if config.Config.DebugMode {
			log.Println("D! Including all operations.")
		}
	}

	if len(s.ExcludeOperations) > 0 {
		if config.Config.DebugMode {
			log.Println("D! Excluding these mount patterns:", s.ExcludeOperations)
		}
	} else {
		if config.Config.DebugMode {
			log.Println("D! Not excluding any operations.")
		}
	}

	return nil
}

func (s *NfsClient) Gather(slist *types.SampleList) {
	file, err := os.Open(s.mountstatsPath)
	if err != nil {
		if config.Config.DebugMode {
			log.Println("D! Failed opening the", file, "file:", err)
		}
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if err := s.processText(scanner, slist); err != nil {
		return
	}

	if err := scanner.Err(); err != nil {
		log.Println("E!", err)
	}
}

func convertToUint64(line []string) ([]uint64, error) {
	/* A "line" of input data (a pre-split array of strings) is
	   processed one field at a time.  Each field is converted to
	   an uint64 value, and appened to an array of return values.
	   On an error, check for ErrRange, and returns an error
	   if found.  This situation indicates a pretty major issue in
	   the /proc/self/mountstats file, and returning faulty data
	   is worse than no data.  Other errors are ignored, and append
	   whatever we got in the first place (probably 0).
	   Yes, this is ugly. */

	var nline []uint64

	if len(line) < 2 {
		return nline, nil
	}

	// Skip the first field; it's handled specially as the "first" variable
	for _, l := range line[1:] {
		val, err := strconv.ParseUint(l, 10, 64)
		if err != nil {
			if numError, ok := err.(*strconv.NumError); ok {
				if numError.Err == strconv.ErrRange {
					return nil, fmt.Errorf("errrange: line:[%v] raw:[%v] -> parsed:[%v]", line, l, val)
				}
			}
		}
		nline = append(nline, val)
	}
	return nline, nil
}

func (s *NfsClient) parseStat(mountpoint string, export string, version string, line []string, slist *types.SampleList) error {
	tags := map[string]string{"mountpoint": mountpoint, "serverexport": export}
	nline, err := convertToUint64(line)
	if err != nil {
		return err
	}

	if len(nline) == 0 {
		log.Println("W! Parsing Stat line with one field:", line)
		return nil
	}

	first := strings.Replace(line[0], ":", "", 1)

	var eventsFields = []string{
		"inoderevalidates",
		"dentryrevalidates",
		"datainvalidates",
		"attrinvalidates",
		"vfsopen",
		"vfslookup",
		"vfsaccess",
		"vfsupdatepage",
		"vfsreadpage",
		"vfsreadpages",
		"vfswritepage",
		"vfswritepages",
		"vfsgetdents",
		"vfssetattr",
		"vfsflush",
		"vfsfsync",
		"vfslock",
		"vfsrelease",
		"congestionwait",
		"setattrtrunc",
		"extendwrite",
		"sillyrenames",
		"shortreads",
		"shortwrites",
		"delay",
		"pnfsreads",
		"pnfswrites",
	}

	var bytesFields = []string{
		"normalreadbytes",
		"normalwritebytes",
		"directreadbytes",
		"directwritebytes",
		"serverreadbytes",
		"serverwritebytes",
		"readpages",
		"writepages",
	}

	var xprtudpFields = []string{
		"bind_count",
		"rpcsends",
		"rpcreceives",
		"badxids",
		"inflightsends",
		"backlogutil",
	}

	var xprttcpFields = []string{
		"bind_count",
		"connect_count",
		"connect_time",
		"idle_time",
		"rpcsends",
		"rpcreceives",
		"badxids",
		"inflightsends",
		"backlogutil",
	}

	var nfsopFields = []string{
		"ops",
		"trans",
		"timeouts",
		"bytes_sent",
		"bytes_recv",
		"queue_time",
		"response_time",
		"total_time",
		"errors",
	}

	var fields = make(map[string]interface{})

	switch first {
	case "READ", "WRITE":
		fields["nfsstat_ops"] = nline[0]
		fields["nfsstat_retrans"] = nline[1] - nline[0]
		fields["nfsstat_bytes"] = nline[3] + nline[4]
		fields["nfsstat_rtt"] = nline[6]
		fields["nfsstat_exe"] = nline[7]
		fields["nfsstat_rtt_per_op"] = 0.0
		if nline[0] > 0 {
			fields["nfsstat_rtt_per_op"] = float64(nline[6]) / float64(nline[0])
		}
		tags["nfsstat_operation"] = first
		slist.PushSamples(inputName, fields, tags)
	}

	if s.Fullstat {
		switch first {
		case "events":
			if len(nline) >= len(eventsFields) {
				for i, t := range eventsFields {
					fields["nfs_events_"+t] = nline[i]
				}
				slist.PushSamples(inputName, fields, tags)
			}

		case "bytes":
			if len(nline) >= len(bytesFields) {
				for i, t := range bytesFields {
					fields["nfs_bytes_"+t] = nline[i]
				}
				slist.PushSamples(inputName, fields, tags)
			}

		case "xprt":
			if len(line) > 1 {
				switch line[1] {
				case "tcp":
					if len(nline)+2 >= len(xprttcpFields) {
						for i, t := range xprttcpFields {
							fields["nfs_xprt_tcp_"+t] = nline[i+2]
						}
						slist.PushSamples(inputName, fields, tags)
					}
				case "udp":
					if len(nline)+2 >= len(xprtudpFields) {
						for i, t := range xprtudpFields {
							fields["nfs_xprt_udp_"+t] = nline[i+2]
						}
						slist.PushSamples(inputName, fields, tags)
					}
				}
			}
		}
		if (version == "3" && s.nfs3Ops[first]) || (version == "4" && s.nfs4Ops[first]) {
			tags["operation"] = first
			if len(nline) <= len(nfsopFields) {
				for i, t := range nline {
					fields["nfs_ops_"+nfsopFields[i]] = t
				}
				slist.PushSamples(inputName, fields, tags)
			}
		}
	}

	return nil
}

func (s *NfsClient) processText(scanner *bufio.Scanner, slist *types.SampleList) error {
	var mount string
	var version string
	var export string
	var skip bool

	for scanner.Scan() {
		lineString := scanner.Text()
		line := strings.Fields(lineString)
		lineLength := len(line)

		if lineLength == 0 {
			continue
		}

		skip = false

		// This denotes a new mount has been found, so set
		// mount and export, and stop skipping (for now)
		if lineLength > 4 && strings.Contains(lineString, "fstype") && (strings.Contains(lineString, "nfs") || strings.Contains(lineString, "nfs4")) {
			mount = line[4]
			export = line[1]
		} else if lineLength > 5 && (strings.Contains(lineString, "(nfs)") || strings.Contains(lineString, "(nfs4)")) {
			version = strings.Split(strings.Split(lineString, "/")[2], " ")[0]
		}
		if mount == "" {
			continue
		}

		if len(s.IncludeMounts) > 0 {
			skip = true
			for _, RE := range s.IncludeMounts {
				matched, _ := regexp.MatchString(RE, mount)
				if matched {
					skip = false
					break
				}
			}
		}

		if !skip && len(s.ExcludeMounts) > 0 {
			for _, RE := range s.ExcludeMounts {
				matched, _ := regexp.MatchString(RE, mount)
				if matched {
					skip = true
					break
				}
			}
		}

		if !skip {
			err := s.parseStat(mount, export, version, line, slist)
			if err != nil {
				return fmt.Errorf("could not parseStat: %w", err)
			}
		}
	}

	return nil
}

func (s *NfsClient) getMountStatsPath() string {
	path := "/proc/self/mountstats"
	if os.Getenv("MOUNT_PROC") != "" {
		path = os.Getenv("MOUNT_PROC")
	}
	if config.Config.DebugMode {
		log.Println("D! using [", path, "] for mountstats")
	}
	return path
}
