package gnmi

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func TestICMPConfig(t *testing.T) {
	c := &Instance{EnableICMP: true}
	if err := c.initICMP(); err != nil {
		t.Fatal(err)
	}
	if c.ICMPCount != 3 || c.ICMPConcurrency != 10 || time.Duration(c.ICMPTimeout) != 3*time.Second {
		t.Fatalf("defaults: %+v", c)
	}
	for _, c := range []*Instance{{EnableICMP: true, ICMPCount: -1}, {EnableICMP: true, ICMPCount: 65537, ICMPPacketInterval: config.Duration(time.Nanosecond)}, {EnableICMP: true, ICMPTimeout: config.Duration(-1)}, {EnableICMP: true, ICMPInterval: config.Duration(time.Second)}} {
		if c.initICMP() == nil {
			t.Fatal("accepted invalid probe config")
		}
	}
	if err := (&Instance{ICMPCount: -1}).initICMP(); err != nil {
		t.Fatal("disabled should ignore options")
	}
}
func TestGatherICMP(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tc := range []struct {
		name             string
		result           icmpResult
		err              error
		wantUp, wantLoss float64
		valid            bool
	}{
		{"partial", icmpResult{Sent: 3, Received: 2, AvgRTT: 2 * time.Millisecond}, nil, 1, 1.0 / 3, true},
		{"lost", icmpResult{Sent: 3}, nil, 0, 1, true},
		{"permission", icmpResult{}, errors.New("permission"), 0, 0, false},
		{"short", icmpResult{Sent: 1, Received: 1}, nil, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Instance{Addresses: []string{"host:123"}, AgentHostTag: "agent_host", EnableICMP: true, DisableGNMIUp: true}
			if err := c.initICMP(); err != nil {
				t.Fatal(err)
			}
			c.icmp.store("host", tc.result, tc.err, now)
			sl := types.NewSampleList()
			c.gatherHealth(sl, now)
			got := map[string]float64{}
			for _, s := range sl.PopBackAll() {
				switch v := s.Value.(type) {
				case float64:
					got[s.Metric] = v
				case int:
					got[s.Metric] = float64(v)
				}
			}
			if got["gnmi_icmp_last_probe_timestamp_seconds"] != 100 {
				t.Fatal(got)
			}
			up, present := got["gnmi_icmp_up"]
			if tc.valid {
				if got["gnmi_icmp_probe_success"] != 1 || !present || up != tc.wantUp || got["gnmi_icmp_packet_loss_ratio"] != tc.wantLoss {
					t.Fatal(got)
				}
			} else if present || got["gnmi_icmp_probe_success"] != 0 {
				t.Fatal(got)
			}
			if _, ok := got["gnmi_icmp_rtt_seconds"]; ok && tc.result.Received == 0 {
				t.Fatal("RTT without replies")
			}
			sl = types.NewSampleList()
			c.gatherHealth(sl, now.Add(64*time.Second))
			for _, s := range sl.PopBackAll() {
				if s.Metric == "gnmi_icmp_up" {
					t.Fatal("stale up emitted")
				}
			}
		})
	}
}
func TestICMPSelectAddress(t *testing.T) {
	ip, err := selectICMPAddress([]net.IPAddr{{IP: net.ParseIP("::1")}, {IP: net.ParseIP("192.0.2.1")}})
	if err != nil || ip.String() != "192.0.2.1" {
		t.Fatalf("%v %v", ip, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (nativeICMPProbe{}).Probe(ctx, "127.0.0.1", icmpOptions{Count: 3, Timeout: time.Second, PacketInterval: time.Millisecond}); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestICMPNativeLoopback(t *testing.T) {
	if os.Getenv("GNMI_TEST_ICMP") != "1" {
		t.Skip("set GNMI_TEST_ICMP=1 for privileged loopback integration test")
	}
	result, err := (nativeICMPProbe{}).Probe(context.Background(), "127.0.0.1", icmpOptions{Count: 3, Timeout: time.Second, PacketInterval: 100 * time.Millisecond, Privileged: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 3 || result.Received != 3 || result.AvgRTT < 0 {
		t.Fatalf("loopback probe: %+v", result)
	}
}

func TestICMPFailedRoundReplacesSuccess(t *testing.T) {
	c := &Instance{EnableICMP: true, DisableGNMIUp: true, Addresses: []string{"host:1"}, AgentHostTag: "agent_host"}
	if err := c.initICMP(); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	c.icmp.store("host", icmpResult{Sent: 3, Received: 3}, nil, now)
	c.icmp.store("host", icmpResult{}, errors.New("DNS"), now.Add(time.Second))
	sl := types.NewSampleList()
	c.gatherHealth(sl, now.Add(time.Second))
	for _, s := range sl.PopBackAll() {
		if s.Metric == "gnmi_icmp_up" || s.Metric == "gnmi_icmp_rtt_seconds" || s.Metric == "gnmi_icmp_packet_loss_ratio" {
			t.Fatal("failed round reused previous values")
		}
	}
}

func TestICMPNativeCancellation(t *testing.T) {
	if os.Getenv("GNMI_TEST_ICMP") != "1" {
		t.Skip("set GNMI_TEST_ICMP=1 for native cancellation integration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	started := time.Now()
	_, err := (nativeICMPProbe{}).Probe(ctx, "127.0.0.1", icmpOptions{Count: 10, Timeout: 5 * time.Second, PacketInterval: 200 * time.Millisecond, Privileged: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("native probe did not stop promptly")
	}
}

type failingICMPConn struct {
	writes   int
	deadline time.Time
}

func (c *failingICMPConn) SetWriteDeadline(t time.Time) error { c.deadline = t; return nil }
func (c *failingICMPConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *failingICMPConn) Close() error                       { return nil }
func (c *failingICMPConn) WriteTo(b []byte, a net.Addr) (int, error) {
	c.writes++
	if c.deadline.IsZero() {
		return 0, errors.New("missing write deadline")
	}
	if c.writes == 2 {
		return 0, syscall.ENOBUFS
	}
	return len(b), nil
}
func (c *failingICMPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	return 0, nil, &net.DNSError{IsTimeout: true}
}
func TestICMPIntermediateSendFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn := &failingICMPConn{}
	result, err := runICMPRound(ctx, conn, &net.IPAddr{IP: net.ParseIP("127.0.0.1")}, icmpOptions{Count: 3, Timeout: time.Second, PacketInterval: time.Millisecond, Privileged: true})
	if !errors.Is(err, syscall.ENOBUFS) || result.Sent != 1 || conn.writes != 2 {
		t.Fatalf("send error retried or hidden: result=%+v err=%v writes=%d", result, err, conn.writes)
	}
}

func TestICMPNativeIPv6Loopback(t *testing.T) {
	if os.Getenv("GNMI_TEST_ICMP") != "1" {
		t.Skip("set GNMI_TEST_ICMP=1 for native IPv6 integration")
	}
	result, err := (nativeICMPProbe{}).Probe(context.Background(), "::1", icmpOptions{Count: 3, Timeout: time.Second, PacketInterval: 100 * time.Millisecond, Privileged: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 3 || result.Received != 3 || result.AvgRTT < 0 {
		t.Fatalf("IPv6 loopback: %+v", result)
	}
}
