//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubelet

import (
	"context"
	"net"
	"time"

	"flashcat.cloud/categraf/logs/util/docker"
	"k8s.io/klog/v2"
)

// connectionInfo contains potential kubelet's ips and hostnames
type connectionInfo struct {
	ips       []string
	hostnames []string
}

func getPotentialKubeletHosts(kubeletHost string) *connectionInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hosts := connectionInfo{ips: nil, hostnames: nil}
	if kubeletHost != "" {
		configIps, configHostnames := getKubeletHostFromConfig(ctx, kubeletHost)
		hosts.ips = append(hosts.ips, configIps...)
		hosts.hostnames = append(hosts.hostnames, configHostnames...)
		klog.V(1).Infof("Got potential kubelet connection info from config, ips: %v, hostnames: %v", configIps, configHostnames)
	}

	dockerIps, dockerHostnames := getKubeletHostFromDocker(ctx)
	hosts.ips = append(hosts.ips, dockerIps...)
	hosts.hostnames = append(hosts.hostnames, dockerHostnames...)
	klog.V(1).Infof("Got potential kubelet connection info from docker, ips: %v, hostnames: %v", dockerIps, dockerHostnames)

	dedupeConnectionInfo(&hosts)

	return &hosts
}

func getKubeletHostFromConfig(ctx context.Context, kubeletHost string) ([]string, []string) {
	var ips []string
	var hostnames []string
	if kubeletHost == "" {
		klog.V(1).Info("kubernetes_kubelet_host is not set")
		return ips, hostnames
	}

	klog.V(1).Infof("Trying to parse kubernetes_kubelet_host: %s", kubeletHost)
	kubeletIP := net.ParseIP(kubeletHost)
	if kubeletIP == nil {
		klog.V(1).Infof("Parsing kubernetes_kubelet_host: %s is a hostname, cached, trying to resolve it to ip...", kubeletHost)
		hostnames = append(hostnames, kubeletHost)
		ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, kubeletHost)
		if err != nil {
			klog.Warningf("Cannot LookupIP hostname %s: %v", kubeletHost, err)
		} else {
			klog.V(1).Infof("kubernetes_kubelet_host: %s is resolved to: %v", kubeletHost, ipAddrs)
			for _, ipAddr := range ipAddrs {
				ips = append(ips, ipAddr.IP.String())
			}
		}
	} else {
		klog.V(1).Infof("Parsed kubernetes_kubelet_host: %s is an address: %v, cached, trying to resolve it to hostname", kubeletHost, kubeletIP)
		ips = append(ips, kubeletIP.String())
		addrs, err := net.DefaultResolver.LookupAddr(ctx, kubeletHost)
		if err != nil {
			klog.Warningf("Cannot LookupHost ip %s: %v", kubeletHost, err)
		} else {
			klog.V(1).Infof("kubernetes_kubelet_host: %s is resolved to: %v", kubeletHost, addrs)
			for _, addr := range addrs {
				hostnames = append(hostnames, addr)
			}
		}
	}

	return ips, hostnames
}

func getKubeletHostFromDocker(ctx context.Context) ([]string, []string) {
	var ips []string
	var hostnames []string
	dockerHost, err := docker.HostnameProvider(ctx, nil)
	if err != nil {
		klog.Warningf("unable to get hostname from docker, make sure to set the kubernetes_kubelet_host option: %v", err)
		return ips, hostnames
	}

	klog.V(1).Infof("Trying to resolve host name %s provided by docker to ip...", dockerHost)
	hostnames = append(hostnames, dockerHost)
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, dockerHost)
	if err != nil {
		klog.Warningf("Cannot resolve host name %s, cached, provided by docker to ip: %v", dockerHost, err)
	} else {
		klog.V(1).Infof("Resolved host name %s provided by docker to %v", dockerHost, ipAddrs)
		for _, ipAddr := range ipAddrs {
			ips = append(ips, ipAddr.IP.String())
		}
	}

	return ips, hostnames
}

func dedupeConnectionInfo(hosts *connectionInfo) {
	ipsKeys := make(map[string]bool)
	ips := []string{}
	for _, ip := range hosts.ips {
		if _, check := ipsKeys[ip]; !check {
			ipsKeys[ip] = true
			ips = append(ips, ip)
		}
	}

	hostnamesKeys := make(map[string]bool)
	hostnames := []string{}
	for _, hostname := range hosts.hostnames {
		if _, check := hostnamesKeys[hostname]; !check {
			hostnamesKeys[hostname] = true
			hostnames = append(hostnames, hostname)
		}
	}

	hosts.ips = ips
	hosts.hostnames = hostnames
}
