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

	"github.com/toolkits/pkg/container/list"
)

const inputName = "nfsclient"

type NfsClient struct {
	config.Interval
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NfsClient{}
	})
}

func (r *NfsClient) Prefix() string {
	return inputName
}

func (r *NfsClient) Init() error {
	if len(r.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(r.Instances); i++ {
		if err := r.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (r *NfsClient) Drop() {}

func (r *NfsClient) Gather(slist *list.SafeList) {

}
func (r *NfsClient) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	Labels map[string]string `toml:"labels"`

	Fullstat          bool     `toml:"fullstat"`
	IncludeMounts     []string `toml:"include_mounts"`
	ExcludeMounts     []string `toml:"exclude_mounts"`
	IncludeOperations []string `toml:"include_operations"`
	ExcludeOperations []string `toml:"exclude_operations"`

	nfs3Ops        map[string]bool
	nfs4Ops        map[string]bool
	mountstatsPath string
}

func (ins *Instance) Init() error {
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

	ins.mountstatsPath = ins.getMountStatsPath()

	if len(ins.IncludeOperations) == 0 {
		for _, Op := range nfs3Fields {
			nfs3Ops[Op] = true
		}
		for _, Op := range nfs4Fields {
			nfs4Ops[Op] = true
		}
	} else {
		for _, Op := range ins.IncludeOperations {
			nfs3Ops[Op] = true
		}
		for _, Op := range ins.IncludeOperations {
			nfs4Ops[Op] = true
		}
	}

	if len(ins.ExcludeOperations) > 0 {
		for _, Op := range ins.ExcludeOperations {
			if nfs3Ops[Op] {
				delete(nfs3Ops, Op)
			}
			if nfs4Ops[Op] {
				delete(nfs4Ops, Op)
			}
		}
	}

	ins.nfs3Ops = nfs3Ops
	ins.nfs4Ops = nfs4Ops

	if len(ins.IncludeMounts) > 0 {
		log.Println("Including these mount patterns: ", ins.IncludeMounts)
	} else {
		log.Println("Including all mounts.")
	}

	if len(ins.ExcludeMounts) > 0 {
		log.Println("Excluding these mount patterns: ", ins.ExcludeMounts)
	} else {
		log.Println("Not excluding any mounts.")
	}

	if len(ins.IncludeOperations) > 0 {
		log.Println("Including these operations: ", ins.IncludeOperations)
	} else {
		log.Println("Including all operations.")
	}

	if len(ins.ExcludeOperations) > 0 {
		log.Println("Excluding these mount patterns: ", ins.ExcludeOperations)
	} else {
		log.Println("Not excluding any operations.")
	}

	return nil
}

func (ins *Instance) Gather(slist *list.SafeList) {
	file, err := os.Open(ins.mountstatsPath)
	if err != nil {
		log.Println("Failed opening the [%s] file: %s ", file, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if err := ins.processText(scanner, slist); err != nil {
		return
	}

	if err := scanner.Err(); err != nil {
		log.Println("%s", err)
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

func (ins *Instance) parseStat(mountpoint string, export string, version string, line []string, slist *list.SafeList) error {
	tags := map[string]string{"mountpoint": mountpoint, "serverexport": export}
	nline, err := convertToUint64(line)
	if err != nil {
		return err
	}

	if len(nline) == 0 {
		log.Println("W! Parsing Stat line with one field: %s\n", line)
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
		fields["ops"] = nline[0]
		fields["retrans"] = nline[1] - nline[0]
		fields["bytes"] = nline[3] + nline[4]
		fields["rtt"] = nline[6]
		fields["exe"] = nline[7]
		fields["rtt_per_op"] = 0.0
		if nline[0] > 0 {
			fields["rtt_per_op"] = float64(nline[6]) / float64(nline[0])
		}
		tags["operation"] = first
		for key, val := range fields {
			slist.PushFront(types.NewSample("nfsstat_"+key, val, tags, ins.Labels))
		}
	}

	if ins.Fullstat {
		switch first {
		case "events":
			if len(nline) >= len(eventsFields) {
				for i, t := range eventsFields {
					fields[t] = nline[i]
				}
				for key, val := range fields {
					slist.PushFront(types.NewSample("nfs_events_"+key, val, tags, ins.Labels))
				}
			}

		case "bytes":
			if len(nline) >= len(bytesFields) {
				for i, t := range bytesFields {
					fields[t] = nline[i]
				}
				for key, val := range fields {
					slist.PushFront(types.NewSample("nfs_bytes_"+key, val, tags, ins.Labels))
				}
			}

		case "xprt":
			if len(line) > 1 {
				switch line[1] {
				case "tcp":
					if len(nline)+2 >= len(xprttcpFields) {
						for i, t := range xprttcpFields {
							fields[t] = nline[i+2]
						}
						for key, val := range fields {
							slist.PushFront(types.NewSample("nfs_xprt_tcp_"+key, val, tags, ins.Labels))
						}
					}
				case "udp":
					if len(nline)+2 >= len(xprtudpFields) {
						for i, t := range xprtudpFields {
							fields[t] = nline[i+2]
						}
						for key, val := range fields {
							slist.PushFront(types.NewSample("nfs_xprt_udp_"+key, val, tags, ins.Labels))
						}
					}
				}
			}
		}
		if (version == "3" && ins.nfs3Ops[first]) || (version == "4" && ins.nfs4Ops[first]) {
			tags["operation"] = first
			if len(nline) <= len(nfsopFields) {
				for i, t := range nline {
					fields[nfsopFields[i]] = t
				}
				for key, val := range fields {
					slist.PushFront(types.NewSample("nfs_ops_"+key, val, tags, ins.Labels))
				}
			}
		}
	}

	return nil
}

func (ins *Instance) processText(scanner *bufio.Scanner, slist *list.SafeList) error {
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

		if len(ins.IncludeMounts) > 0 {
			skip = true
			for _, RE := range ins.IncludeMounts {
				matched, _ := regexp.MatchString(RE, mount)
				if matched {
					skip = false
					break
				}
			}
		}

		if !skip && len(ins.ExcludeMounts) > 0 {
			for _, RE := range ins.ExcludeMounts {
				matched, _ := regexp.MatchString(RE, mount)
				if matched {
					skip = true
					break
				}
			}
		}

		if !skip {
			err := ins.parseStat(mount, export, version, line, slist)
			if err != nil {
				return fmt.Errorf("could not parseStat: %w", err)
			}
		}
	}

	return nil
}

func (ins *Instance) getMountStatsPath() string {
	path := "/proc/self/mountstats"
	if os.Getenv("MOUNT_PROC") != "" {
		path = os.Getenv("MOUNT_PROC")
	}
	log.Println("using [%s] for mountstats", path)
	return path
}
