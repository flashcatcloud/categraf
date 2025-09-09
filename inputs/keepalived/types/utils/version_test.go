package utils

import (
	"testing"

	"github.com/hashicorp/go-version"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	versionOutput := `Keepalived v2.0.20 (05/04,2020), git commit v20200428-362-g114364588e

	Copyright(C) 2001-2020 Alexandre Cassen, <acassen@gmail.com>
	
	Built with kernel headers for Linux 5.4.5
	Running on Linux 5.4.0-58-generic #64-Ubuntu SMP Wed Dec 9 08:16:25 UTC 2020
	
	configure options: --build=x86_64-alpine-linux-musl --host=x86_64-alpine-linux-musl --prefix=/usr --sysconfdir=/etc --mandir=/usr/share/man --localstatedir=/var --enable-vrrp --enable-sha1 build_alias=x86_64-alpine-linux-musl host_alias=x86_64-alpine-linux-musl CC=gcc CFLAGS=-Os -fomit-frame-pointer LDFLAGS=-Wl,--as-needed CPPFLAGS=-Os -fomit-frame-pointer
	
	Config options:  LVS VRRP VRRP_AUTH OLD_CHKSUM_COMPAT FIB_ROUTING
	
	System options:  PIPE2 SIGNALFD INOTIFY_INIT1 VSYSLOG EPOLL_CREATE1 IPV4_DEVCONF IPV6_ADVANCED_API LIBNL1 RTA_ENCAP RTA_EXPIRES RTA_NEWDST RTA_PREF FRA_SUPPRESS_PREFIXLEN FRA_SUPPRESS_IFGROUP FRA_TUN_ID RTAX_CC_ALGO RTAX_QUICKACK RTEXT_FILTER_SKIP_STATS FRA_L3MDEV FRA_UID_RANGE RTAX_FASTOPEN_NO_COOKIE RTA_VIA FRA_OIFNAME FRA_PROTOCOL FRA_IP_PROTO FRA_SPORT_RANGE FRA_DPORT_RANGE RTA_TTL_PROPAGATE IFA_FLAGS IP_MULTICAST_ALL LWTUNNEL_ENCAP_MPLS LWTUNNEL_ENCAP_ILA NET_LINUX_IF_H_COLLISION NETINET_LINUX_IF_ETHER_H_COLLISION LIBIPTC_LINUX_NET_IF_H_COLLISION LIBIPVS_NETLINK IPVS_DEST_ATTR_ADDR_FAMILY IPVS_SYNCD_ATTRIBUTES IPVS_64BIT_STATS IPVS_TUN_TYPE IPVS_TUN_CSUM IPVS_TUN_GRE VRRP_VMAC VRRP_IPVLAN IFLA_LINK_NETNSID CN_PROC SOCK_NONBLOCK SOCK_CLOEXEC O_PATH INET6_ADDR_GEN_MODE VRF SO_MARK SCHED_RESET_ON_FORK
	`
	excpectedVersion := version.Must(version.NewVersion("2.0.20"))

	v, err := ParseVersion(versionOutput)
	if err != nil {
		t.Fail()
	}

	if v.Compare(excpectedVersion) != 0 {
		t.Fail()
	}

	versionOutput = "Keepalived v2.0.20 (05/04,2020), git commit v20200428-362-g114364588e"
	if _, err := ParseVersion(versionOutput); err == nil {
		t.Fail()
	}

	versionOutput = `Keepalived

	Copyright(C) 2001-2020 Alexandre Cassen, <acassen@gmail.com>
	`
	if _, err := ParseVersion(versionOutput); err == nil {
		t.Fail()
	}

	versionOutput = `Keepalived keepalived

	Copyright(C) 2001-2020 Alexandre Cassen, <acassen@gmail.com>
	`
	if _, err := ParseVersion(versionOutput); err == nil {
		t.Fail()
	}
}
