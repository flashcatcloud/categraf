//go:build ignore
#include "common.h"
#include "bpf_endian.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define IP_UDP 17
#define UDP_RDMA 4791
#define UDP_HLEN 8
#define ETH_HLEN 14

struct counters {
  __u64 pkts;
  __u64 bytes;
} ;

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 1024);
    __type(key, __u32);
    __type(value, struct counters);
} packet_cnt SEC(".maps");

static __always_inline int parse_udp_dst_port(struct xdp_md *ctx, __u32 *ip_src_addr, __u64 *bytes) {
	void *data_end = (void *)(long)ctx->data_end;
	void *data     = (void *)(long)ctx->data;

	// First, parse the ethernet header.
	struct ethhdr *eth = data;
	if ((void *)(eth + 1) > data_end) {
		return 0;
	}

	if (eth->h_proto != bpf_htons(ETH_P_IP)) {
		// The protocol is not IPv4
		return 0;
	}

	// Then parse the IP header.
	struct iphdr *ip = (void *)(eth + 1);
	if ((void *)(ip + 1) > data_end) {
		return 0;
	}

	if (ip->protocol != IP_UDP) {
        // The protocol is not udp
        return 0;
    }

    // finally parse the UDP header.
    struct udphdr *udp = (void *)(ip + 1);
	if ((void *)(udp + 1) > data_end) {
		return 0;
	}

    if(udp->dport != bpf_htons(UDP_RDMA)){
        // The protocol is not rdma
        return 0;
    }

    // Return the source IP address in network byte order.
    *ip_src_addr = (__u32)(ip->saddr);
    *bytes = (__u64)bpf_htons(ip->tot_len) + ETH_HLEN;
	return 1;
}

SEC("xdp")
int packet_monitor(struct xdp_md *ctx) {
    __u32 ip;
    __u64 bytes;
	if (!parse_udp_dst_port(ctx, &ip, &bytes)) {
		return XDP_PASS;
	}

	struct counters *c = bpf_map_lookup_elem(&packet_cnt, &ip);
    if (!c) {
        struct counters init_c = {
            .pkts = 1,
            .bytes = bytes
        };
        bpf_map_update_elem(&packet_cnt, &ip, &init_c, BPF_ANY);
    } else {
        __sync_fetch_and_add(&c->pkts, 1);
        __sync_fetch_and_add(&c->bytes, bytes);
    }
	return XDP_PASS;
}

