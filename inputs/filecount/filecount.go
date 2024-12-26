package filecount

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/globpath"
	"flashcat.cloud/categraf/types"
	"github.com/karrick/godirwalk"
)

const inputName = "filecount"

type FileCount struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type Instance struct {
	config.InstanceConfig
	Directories    []string `toml:"directories"`
	FileName       string   `toml:"file_name"`
	Recursive      *bool    `toml:"recursive"`
	RegularOnly    *bool    `toml:"regular_only"`
	FollowSymlinks bool     `toml:"follow_symlinks"`
	Size           Size     `toml:"size"`
	MTime          Duration `toml:"mtime"`
	fileFilters    []fileFilterFunc
	globPaths      []globpath.GlobPath
	Fs             fileSystem
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &FileCount{}
	})
}

func (fc *FileCount) Clone() inputs.Input {
	return &FileCount{}
}

func (fc *FileCount) Name() string {
	return inputName
}

func (fc *FileCount) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(fc.Instances))
	for i := 0; i < len(fc.Instances); i++ {
		ret[i] = fc.Instances[i]
	}
	return ret
}

func (ins *Instance) Init() error {
	if len(ins.Directories) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.FileName == "" {
		ins.FileName = "*"
	}

	if ins.Recursive == nil {
		flag := true
		ins.Recursive = &flag
	}

	if ins.RegularOnly == nil {
		flag := true
		ins.RegularOnly = &flag
	}

	if ins.Fs == nil {
		ins.Fs = osFS{}
	}

	if ins.globPaths == nil {
		dir, err := ins.initGlobPaths()
		if err != nil {
			return fmt.Errorf("failed to init glob path: %s, error: %v", dir, err)
		}
	}

	return nil
}

func (ins *Instance) initGlobPaths() (string, error) {
	ins.globPaths = []globpath.GlobPath{}
	for _, directory := range ins.getDirs() {
		glob, err := globpath.Compile(directory)
		if err != nil {
			return directory, err
		} else {
			ins.globPaths = append(ins.globPaths, *glob)
		}
	}

	return "", nil
}

func (ins *Instance) getDirs() []string {
	dirs := make([]string, 0, len(ins.Directories)+1)
	for _, dir := range ins.Directories {
		dirs = append(dirs, filepath.Clean(dir))
	}

	return dirs
}

func (ins *Instance) onlyDirectories(directories []string) []string {
	out := make([]string, 0)
	for _, path := range directories {
		info, err := ins.Fs.Stat(path)
		if err == nil && info.IsDir() {
			out = append(out, path)
		}
	}
	return out
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup

	for _, glob := range ins.globPaths {
		wg.Add(1)
		go func(g globpath.GlobPath) {
			for _, dir := range ins.onlyDirectories(g.GetRoots()) {
				ins.count(slist, dir, g)
			}
			wg.Done()
		}(glob)
	}

	wg.Wait()
}

type fileFilterFunc func(os.FileInfo) (bool, error)

func rejectNilFilters(filters []fileFilterFunc) []fileFilterFunc {
	filtered := make([]fileFilterFunc, 0, len(filters))
	for _, f := range filters {
		if f != nil {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

func (ins *Instance) nameFilter() fileFilterFunc {
	if ins.FileName == "*" {
		return nil
	}

	return func(f os.FileInfo) (bool, error) {
		match, err := filepath.Match(ins.FileName, f.Name())
		if err != nil {
			return false, err
		}
		return match, nil
	}
}

func (ins *Instance) regularOnlyFilter() fileFilterFunc {
	if !*ins.RegularOnly {
		return nil
	}

	return func(f os.FileInfo) (bool, error) {
		return f.Mode().IsRegular(), nil
	}
}

func (ins *Instance) sizeFilter() fileFilterFunc {
	if ins.Size == 0 {
		return nil
	}

	return func(f os.FileInfo) (bool, error) {
		if !f.Mode().IsRegular() {
			return false, nil
		}
		if ins.Size < 0 {
			return f.Size() < -int64(ins.Size), nil
		}
		return f.Size() >= int64(ins.Size), nil
	}
}

func (ins *Instance) mtimeFilter() fileFilterFunc {
	if time.Duration(ins.MTime) == 0 {
		return nil
	}

	return func(f os.FileInfo) (bool, error) {
		age := absDuration(time.Duration(ins.MTime))
		mtime := time.Now().Add(-age)
		if time.Duration(ins.MTime) < 0 {
			return f.ModTime().After(mtime), nil
		}
		return f.ModTime().Before(mtime), nil
	}
}

func absDuration(x time.Duration) time.Duration {
	if x < 0 {
		return -x
	}
	return x
}

func (ins *Instance) initFileFilters() {
	filters := []fileFilterFunc{
		ins.nameFilter(),
		ins.regularOnlyFilter(),
		ins.sizeFilter(),
		ins.mtimeFilter(),
	}
	ins.fileFilters = rejectNilFilters(filters)
}

func (ins *Instance) count(slist *types.SampleList, basedir string, glob globpath.GlobPath) {
	childCount := make(map[string]int64)
	childSize := make(map[string]int64)
	oldestFileTimestamp := make(map[string]int64)
	newestFileTimestamp := make(map[string]int64)

	walkFn := func(path string, de *godirwalk.Dirent) error {
		rel, err := filepath.Rel(basedir, path)
		if err == nil && rel == "." {
			return nil
		}
		file, err := ins.Fs.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		match, err := ins.filter(file)
		if err != nil {
			log.Println("E! filter file fail:", err)
			return nil
		}
		if match {
			parent := filepath.Dir(path)
			childCount[parent]++
			childSize[parent] += file.Size()
			if oldestFileTimestamp[parent] == 0 || oldestFileTimestamp[parent] > file.ModTime().UnixNano() {
				oldestFileTimestamp[parent] = file.ModTime().UnixNano()
			}
			if newestFileTimestamp[parent] == 0 || newestFileTimestamp[parent] < file.ModTime().UnixNano() {
				newestFileTimestamp[parent] = file.ModTime().UnixNano()
			}
		}
		if file.IsDir() && !*ins.Recursive && !glob.HasSuperMeta {
			return filepath.SkipDir
		}
		return nil
	}

	postChildrenFn := func(path string, de *godirwalk.Dirent) error {
		if glob.MatchString(path) {

			tags := map[string]string{
				"directory": path,
			}

			gauge := map[string]interface{}{
				"count":      childCount[path],
				"size_bytes": childSize[path],
			}

			gauge["oldest_file_timestamp"] = oldestFileTimestamp[path]
			gauge["newest_file_timestamp"] = newestFileTimestamp[path]

			slist.PushSamples(inputName, gauge, tags)
		}
		parent := filepath.Dir(path)
		if *ins.Recursive {
			childCount[parent] += childCount[path]
			childSize[parent] += childSize[path]
			if oldestFileTimestamp[parent] == 0 || oldestFileTimestamp[parent] > oldestFileTimestamp[path] {
				oldestFileTimestamp[parent] = oldestFileTimestamp[path]
			}
			if newestFileTimestamp[parent] == 0 || newestFileTimestamp[parent] < newestFileTimestamp[path] {
				newestFileTimestamp[parent] = newestFileTimestamp[path]
			}
		}
		delete(childCount, path)
		delete(childSize, path)
		delete(oldestFileTimestamp, path)
		delete(newestFileTimestamp, path)
		return nil
	}

	err := godirwalk.Walk(basedir, &godirwalk.Options{
		Callback:             walkFn,
		PostChildrenCallback: postChildrenFn,
		Unsorted:             true,
		FollowSymbolicLinks:  ins.FollowSymlinks,
		ErrorCallback: func(osPathname string, err error) godirwalk.ErrorAction {
			if errors.Is(err, fs.ErrPermission) {
				log.Println("E! no permission to walk dir:", err)
				return godirwalk.SkipNode
			}
			return godirwalk.Halt
		},
	})
	if err != nil {
		log.Println("E! count dir error:", err)
	}
}

func (ins *Instance) filter(file os.FileInfo) (bool, error) {
	if ins.fileFilters == nil {
		ins.initFileFilters()
	}

	for _, fileFilter := range ins.fileFilters {
		match, err := fileFilter(file)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}

	return true, nil
}
