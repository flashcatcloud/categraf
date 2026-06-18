//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/status"
	"flashcat.cloud/categraf/logs/util"
)

// OpenFilesLimitWarningType is the key of the message generated when too many
// files are tailed
const openFilesLimitWarningType = "open_files_limit_warning"

var ErrMaxTraverseLimit = errors.New("max traverse limit reached")

var fileProviderPermissionDeniedTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "categraf_logs_file_permission_denied_total",
		Help: "Total number of permission denied errors encountered while walking recursive log paths",
	},
)

func init() {
	if err := prometheus.Register(fileProviderPermissionDeniedTotal); err != nil {
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			log.Println("W! failed to register fileProviderPermissionDeniedTotal metric:", err)
		}
	}
}

// File represents a file to tail
type File struct {
	Path string
	// IsWildcardPath is set to true when the File has been discovered
	// in a directory with wildcard(s) in the configuration.
	IsWildcardPath bool
	Source         *logsconfig.LogSource
}

// NewFile returns a new File
func NewFile(path string, source *logsconfig.LogSource, isWildcardPath bool) *File {
	return &File{
		Path:           path,
		Source:         source,
		IsWildcardPath: isWildcardPath,
	}
}

// GetScanKey returns a key used by the scanner to index the scanned file.
// If it is a file scanned for a container, it will use the format: <filepath>/<container_id>
// Otherwise, it will simply use the format: <filepath>
func (t *File) GetScanKey() string {
	key := t.Path
	if t.Source != nil && t.Source.Config != nil {
		if t.Source.Config.Identifier != "" {
			key = fmt.Sprintf("%s/%s", key, t.Source.Config.Identifier)
		}
	}
	return key
}

// Provider implements the logic to retrieve at most filesLimit Files defined in sources
type Provider struct {
	filesLimit       int
	maxTraverseLimit int
	maxDepthLimit    int
	shouldLogErrors  bool
}

// NewProvider returns a new Provider
func NewProvider(filesLimit, maxTraverseLimit, maxDepthLimit int) *Provider {
	if maxTraverseLimit <= 0 {
		maxTraverseLimit = config.MaxTraverseLimit()
	}
	if maxDepthLimit <= 0 {
		maxDepthLimit = config.MaxDepthLimit()
	}
	return &Provider{
		filesLimit:       filesLimit,
		maxTraverseLimit: maxTraverseLimit,
		maxDepthLimit:    maxDepthLimit,
		shouldLogErrors:  true,
	}
}

// FilesToTail returns all the Files matching paths in sources,
// it cannot return more than filesLimit Files.
// For now, there is no way to prioritize specific Files over others,
// they are just returned in reverse lexicographical order, see `searchFiles`
func (p *Provider) FilesToTail(sources []*logsconfig.LogSource) []*File {
	var filesToTail []*File
	shouldLogErrors := p.shouldLogErrors
	p.shouldLogErrors = false // Let's log errors on first run only

	for i := 0; i < len(sources); i++ {
		source := sources[i]
		tailedFileCounter := 0
		files, err := p.CollectFiles(source)
		isWildcardPath := logsconfig.ContainsWildcard(source.Config.Path) || util.ContainsDatePattern(source.Config.Path)
		if err != nil {
			source.Status.Error(err)
			if isWildcardPath {
				source.Messages.AddMessage(source.Config.Path, fmt.Sprintf("%d files tailed out of %d files matching", tailedFileCounter, len(files)))
			}
			if shouldLogErrors {
				log.Println("W! Could not collect files:", err)
			}
			continue
		}
		for j := 0; j < len(files) && len(filesToTail) < p.filesLimit; j++ {
			file := files[j]
			filesToTail = append(filesToTail, file)
			tailedFileCounter++
		}

		if len(filesToTail) >= p.filesLimit {
			status.AddGlobalWarning(
				openFilesLimitWarningType,
				fmt.Sprintf(
					"The limit on the maximum number of files in use (%d) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
					p.filesLimit,
				),
			)
		} else {
			status.RemoveGlobalWarning(openFilesLimitWarningType)
		}

		if isWildcardPath {
			source.Messages.AddMessage(source.Config.Path, fmt.Sprintf("%d files tailed out of %d files matching", tailedFileCounter, len(files)))
		}
	}

	if len(filesToTail) == p.filesLimit {
		log.Println("W! Reached the limit on the maximum number of files in use: ", p.filesLimit)
		return filesToTail
	}

	return filesToTail
}

// CollectFiles returns all the files matching the source path.
func (p *Provider) CollectFiles(source *logsconfig.LogSource) ([]*File, error) {
	path := source.Config.Path
	originalPath := path
	if util.ContainsDatePattern(path) {
		expandedPath := util.ExpandDatePattern(path, time.Now())
		log.Printf("D! Expanded date pattern: %s -> %s", originalPath, expandedPath)
		path = expandedPath
	}

	fileExists := p.exists(path)
	switch {
	case fileExists:
		return []*File{
			NewFile(path, source, false),
		}, nil
	case logsconfig.ContainsWildcard(path):
		pattern := path
		return p.searchFiles(pattern, source)
	default:
		if util.ContainsDatePattern(originalPath) {
			return nil, fmt.Errorf("file %s does not exist (expanded from %s)", path, originalPath)
		}
		return nil, fmt.Errorf("file %s does not exist", path)
	}
}

// hasDoublestar checks if a pattern contains a ** segment
func hasDoublestar(pattern string) bool {
	slashPattern := filepath.ToSlash(pattern)
	return strings.Contains(slashPattern, "/**/") ||
		strings.HasPrefix(slashPattern, "**/") ||
		strings.HasSuffix(slashPattern, "/**") ||
		slashPattern == "**"
}

// searchFiles returns all the files matching the source path pattern.
func (p *Provider) searchFiles(pattern string, source *logsconfig.LogSource) ([]*File, error) {
	var paths []string
	var err error
	if hasDoublestar(pattern) {
		paths, err = p.doublestarWalk(pattern)
	} else {
		paths, err = filepath.Glob(pattern)
	}

	if err != nil {
		return nil, fmt.Errorf("malformed pattern or walk failed: %s, error: %w", pattern, err)
	}
	if len(paths) == 0 {
		// no file was found, its parent directories might have wrong permissions or it just does not exist
		return nil, fmt.Errorf("could not find any file matching pattern %s, check that all its subdirectories are executable", pattern)
	}
	var files []*File

	// Files are sorted because of a heuristic on the filename: often the filename and/or the folder name
	// contains information in the file datetime. Most of the time we want the most recent files.
	// Here, we reverse paths to have stable sort keep reverse lexicographical order w.r.t dir names. Example:
	// [/tmp/1/2017.log, /tmp/1/2018.log, /tmp/2/2018.log] becomes [/tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log]
	// then kept as is by the sort below.
	// https://github.com/golang/go/wiki/SliceTricks#reversing
	for i := len(paths)/2 - 1; i >= 0; i-- {
		opp := len(paths) - 1 - i
		paths[i], paths[opp] = paths[opp], paths[i]
	}
	// sort paths by descending filenames
	sort.SliceStable(paths, func(i, j int) bool {
		return filepath.Base(paths[i]) > filepath.Base(paths[j])
	})

	// Resolve excluded path(s)
	now := time.Now()
	var expandedExcludes []string
	for _, excludePattern := range source.Config.ExcludePaths {
		if util.ContainsDatePattern(excludePattern) {
			expandedExcludes = append(expandedExcludes, util.ExpandDatePattern(excludePattern, now))
		} else {
			expandedExcludes = append(expandedExcludes, excludePattern)
		}
	}

	// Pre-compute slash-normalized excludes and hoist env lookup
	normalizedExcludes := make([]string, len(expandedExcludes))
	for i, ep := range expandedExcludes {
		normalizedExcludes[i] = filepath.ToSlash(ep)
	}
	hostMountPrefix := os.Getenv("HOST_MOUNT_PREFIX")

	for _, path := range paths {
		isExcluded := false
		pathSlash := filepath.ToSlash(path)
		for _, excludePattern := range normalizedExcludes {
			matched, err := doublestar.Match(excludePattern, pathSlash)
			if err != nil {
				log.Printf("W! Invalid exclude pattern %q: %v", excludePattern, err)
				continue
			}
			if matched {
				isExcluded = true
				log.Printf("D! Excluded path: %s", path)
				break
			}
		}

		if !isExcluded {
			if hostMountPrefix != "" {
				pt, err := os.Readlink(path)
				if err == nil {
					path = filepath.Join(hostMountPrefix, pt)
				}
				// If err != nil (e.g. not a symlink), we silently keep the original unprefixed path
			}
			files = append(files, NewFile(path, source, true))
		}
	}
	return files, nil
}

// doublestarWalk walks the file system based on the given pattern, supporting ** matching,
// with protection mechanisms like maxTraverseLimit, depth limiting, and permission error ignoring.
func (p *Provider) doublestarWalk(pattern string) ([]string, error) {
	slashPattern := filepath.ToSlash(pattern)
	base, _ := doublestar.SplitPattern(slashPattern)
	if base == "" {
		base = "."
	}
	base = filepath.Clean(filepath.FromSlash(base))

	var paths []string
	traverseCount := 0

	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		traverseCount++
		if traverseCount > p.maxTraverseLimit {
			return ErrMaxTraverseLimit
		}

		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				fileProviderPermissionDeniedTotal.Inc()
				log.Printf("D! Permission denied while scanning %s, ignoring", path)
				return nil // ignore and continue
			}
			return err
		}

		if d.IsDir() {
			rel, err := filepath.Rel(base, path)
			if err == nil && rel != "." {
				depth := len(strings.Split(filepath.ToSlash(rel), "/"))
				// Limit maximum recursion depth
				if depth > p.maxDepthLimit {
					log.Printf("D! Skipping directory %s: depth %d exceeds limit %d", path, depth, p.maxDepthLimit)
					return filepath.SkipDir
				}
			}
		} else {
			ok, _ := doublestar.Match(filepath.ToSlash(pattern), filepath.ToSlash(path))
			if ok {
				paths = append(paths, path)
			}
		}
		return nil
	})

	if errors.Is(err, ErrMaxTraverseLimit) {
		log.Printf("W! Max traverse limit (%d) reached while scanning pattern: %s, returning %d partial results", p.maxTraverseLimit, pattern, len(paths))
		return paths, nil
	}

	if err != nil && errors.Is(err, os.ErrNotExist) {
		log.Printf("D! Base path does not exist for pattern: %s", pattern)
		return nil, nil
	}

	return paths, err
}

// exists returns true if the file at path filePath exists
// Note: we can't rely on os.IsNotExist for windows, so we check error nullity.
// As we're tailing with *, the error is related to the path being malformed.
func (p *Provider) exists(filePath string) bool {
	if _, err := os.Stat(filePath); err != nil {
		return false
	}
	return true
}
