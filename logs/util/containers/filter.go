//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	coreconfig "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/logs/util"
)

const (
	// Pause container image names that should be filtered out.
	// Where appropriate, each constant is loosely structured as
	// image:domain.*/pause.*

	pauseContainerKubernetes = "image:kubernetes/pause"
	pauseContainerECS        = "image:amazon/amazon-ecs-pause"
	pauseContainerOpenshift  = "image:openshift/origin-pod"
	pauseContainerOpenshift3 = "image:.*rhel7/pod-infrastructure"

	// - asia.gcr.io/google-containers/pause-amd64
	// - gcr.io/google-containers/pause
	// - *.gcr.io/google_containers/pause
	// - *.jfrog.io/google_containers/pause
	pauseContainerGoogle = "image:google(_|-)containers/pause.*"

	// - k8s.gcr.io/pause-amd64:3.1
	// - asia.gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/gke-release/pause-win:1.1.0
	// - eu.gcr.io/k8s-artifacts-prod/pause:3.3
	// - k8s.gcr.io/pause
	pauseContainerGCR = `image:.*gcr\.io(.*)/pause.*`

	// - k8s-gcrio.azureedge.net/pause-amd64
	// - gcrio.azureedge.net/google_containers/pause-amd64
	pauseContainerAzure = `image:.*azureedge\.net(.*)/pause.*`

	// amazonaws.com/eks/pause-windows:latest
	// eks/pause-amd64
	pauseContainerEKS = `image:(amazonaws\.com/)?eks/pause.*`
	// rancher/pause-amd64:3.0
	pauseContainerRancher = `image:rancher/pause.*`
	// - mcr.microsoft.com/k8s/core/pause-amd64
	pauseContainerMCR = `image:mcr\.microsoft\.com(.*)/pause.*`
	// - aksrepos.azurecr.io/mirror/pause-amd64
	pauseContainerAKS = `image:aksrepos\.azurecr\.io(.*)/pause.*`
	// - kubeletwin/pause:latest
	pauseContainerWin = `image:kubeletwin/pause.*`
	// - ecr.us-east-1.amazonaws.com/pause
	pauseContainerECR = `image:ecr(.*)amazonaws\.com/pause.*`
	// - *.ecr.us-east-1.amazonaws.com/upstream/pause
	pauseContainerUpstream = `image:upstream/pause.*`
	// - cdk/pause-amd64
	pauseContainerCDK = `image:cdk/pause.*`
	categrafContainer = `image:flashcatcloud/categraf.*`

	// filter prefixes for inclusion/exclusion
	imageFilterPrefix         = `image:`
	nameFilterPrefix          = `name:`
	kubeNamespaceFilterPrefix = `kube_namespace:`
)

// Filter holds the state for the container filtering logic
type Filter struct {
	Enabled              bool
	ImageIncludeList     []*regexp.Regexp
	NameIncludeList      []*regexp.Regexp
	NamespaceIncludeList []*regexp.Regexp
	ImageExcludeList     []*regexp.Regexp
	NameExcludeList      []*regexp.Regexp
	NamespaceExcludeList []*regexp.Regexp
	Errors               map[string]struct{}
}

var sharedFilter *Filter

func parseFilters(filters []string) (imageFilters, nameFilters, namespaceFilters []*regexp.Regexp, filterErrs []string, err error) {
	var filterWarnings []string
	for _, filter := range filters {
		switch {
		case strings.HasPrefix(filter, imageFilterPrefix):
			r, err := filterToRegex(filter, imageFilterPrefix)
			if err != nil {
				filterErrs = append(filterErrs, err.Error())
				continue
			}
			imageFilters = append(imageFilters, r)
		case strings.HasPrefix(filter, nameFilterPrefix):
			r, err := filterToRegex(filter, nameFilterPrefix)
			if err != nil {
				filterErrs = append(filterErrs, err.Error())
				continue
			}
			nameFilters = append(nameFilters, r)
		case strings.HasPrefix(filter, kubeNamespaceFilterPrefix):
			r, err := filterToRegex(filter, kubeNamespaceFilterPrefix)
			if err != nil {
				filterErrs = append(filterErrs, err.Error())
				continue
			}
			namespaceFilters = append(namespaceFilters, r)
		default:
			warnmsg := fmt.Sprintf("Container filter %q is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'", filter)
			log.Println(warnmsg)
			filterWarnings = append(filterWarnings, warnmsg)

		}
	}
	if len(filterErrs) > 0 {
		return nil, nil, nil, append(filterErrs, filterWarnings...), errors.New(filterErrs[0])
	}
	return imageFilters, nameFilters, namespaceFilters, filterWarnings, nil
}

// filterToRegex checks a filter's regex
func filterToRegex(filter string, filterPrefix string) (*regexp.Regexp, error) {
	pat := strings.TrimPrefix(filter, filterPrefix)
	r, err := regexp.Compile(pat)
	if err != nil {
		errormsg := fmt.Errorf("invalid regex '%s': %s", pat, err)
		return nil, errormsg
	}
	return r, nil
}

// GetSharedMetricFilter allows to share the result of NewFilterFromConfig
// for several user classes
func GetSharedMetricFilter() (*Filter, error) {
	if sharedFilter != nil {
		return sharedFilter, nil
	}
	f, err := newMetricFilterFromConfig()
	if err != nil {
		return nil, err
	}
	sharedFilter = f
	return f, nil
}

// ResetSharedFilter is only to be used in unit tests: it resets the global
// filter instance to force re-parsing of the configuration.
func ResetSharedFilter() {
	sharedFilter = nil
}

// GetFilterErrors retrieves a list of errors and warnings resulting from parseFilters
func GetFilterErrors() map[string]struct{} {
	filter, _ := newMetricFilterFromConfig()
	logFilter, _ := NewAutodiscoveryFilter(LogsFilter)
	for err := range logFilter.Errors {
		filter.Errors[err] = struct{}{}
	}
	return filter.Errors
}

// NewFilter creates a new container filter from a two slices of
// regexp patterns for a include list and exclude list. Each pattern should have
// the following format: "field:pattern" where field can be: [image, name, kube_namespace].
// An error is returned if any of the expression don't compile.
func NewFilter(includeList, excludeList []string) (*Filter, error) {
	imgIncl, nameIncl, nsIncl, filterErrsIncl, errIncl := parseFilters(includeList)
	imgExcl, nameExcl, nsExcl, filterErrsExcl, errExcl := parseFilters(excludeList)

	errors := append(filterErrsIncl, filterErrsExcl...)
	errorsMap := make(map[string]struct{})
	if len(errors) > 0 {
		for _, err := range errors {
			errorsMap[err] = struct{}{}
		}
	}

	if errIncl != nil {
		return &Filter{Errors: errorsMap}, errIncl
	}
	if errExcl != nil {
		return &Filter{Errors: errorsMap}, errExcl
	}

	return &Filter{
		Enabled:              len(includeList) > 0 || len(excludeList) > 0,
		ImageIncludeList:     imgIncl,
		NameIncludeList:      nameIncl,
		NamespaceIncludeList: nsIncl,
		ImageExcludeList:     imgExcl,
		NameExcludeList:      nameExcl,
		NamespaceExcludeList: nsExcl,
		Errors:               errorsMap,
	}, nil
}

// newMetricFilterFromConfig creates a new container filter, sourcing patterns
// from the pkg/config options, to be used only for metrics
func newMetricFilterFromConfig() (*Filter, error) {
	// We merge `container_include` and `container_include_metrics` as this filter
	// is used by all core and python checks (so components sending metrics).
	includeList := coreconfig.GetContainerIncludeList()
	excludeList := coreconfig.GetContainerExcludeList()
	if len(excludeList) == 0 {
		excludeList = append(excludeList, categrafContainer)
	}

	excludeList = append(excludeList,
		pauseContainerGCR,
		pauseContainerOpenshift,
		pauseContainerOpenshift3,
		pauseContainerKubernetes,
		pauseContainerGoogle,
		pauseContainerAzure,
		pauseContainerECS,
		pauseContainerEKS,
		pauseContainerRancher,
		pauseContainerMCR,
		pauseContainerWin,
		pauseContainerAKS,
		pauseContainerECR,
		pauseContainerUpstream,
		pauseContainerCDK,
	)
	return NewFilter(includeList, excludeList)
}

// NewAutodiscoveryFilter creates a new container filter for Autodiscovery
// It sources patterns from the pkg/config options but ignores the exclude_pause_container options
// It allows to filter metrics and logs separately
// For use in autodiscovery.
func NewAutodiscoveryFilter(filter FilterType) (*Filter, error) {
	includeList := coreconfig.GetContainerIncludeList()
	excludeList := coreconfig.GetContainerExcludeList()
	switch filter {
	case GlobalFilter:
		if len(excludeList) == 0 {
			excludeList = append(excludeList, categrafContainer)
		}
	case LogsFilter:
		if len(excludeList) == 0 {
			excludeList = append(excludeList, categrafContainer)
		}
	}
	return NewFilter(includeList, excludeList)
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
func (cf Filter) IsExcluded(containerName, containerImage, podNamespace string) bool {
	if !cf.Enabled {
		return false
	}

	// Any includeListed take precedence on excluded
	for _, r := range cf.ImageIncludeList {
		if r.MatchString(containerImage) {
			return false
		}
	}
	for _, r := range cf.NameIncludeList {
		if r.MatchString(containerName) {
			return false
		}
	}
	for _, r := range cf.NamespaceIncludeList {
		if r.MatchString(podNamespace) {
			return false
		}
	}

	// Check if excludeListed
	for _, r := range cf.ImageExcludeList {
		match := r.MatchString(containerImage)
		if util.Debug() {
			log.Printf("D!, exclude item :%+v, container image:%s, %t\n", r, containerImage, match)
		}
		if match {
			return true
		}
	}
	for _, r := range cf.NameExcludeList {
		if r.MatchString(containerName) {
			return true
		}
	}
	for _, r := range cf.NamespaceExcludeList {
		if r.MatchString(podNamespace) {
			return true
		}
	}

	return false
}
