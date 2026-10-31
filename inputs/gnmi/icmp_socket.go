package gnmi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"io"
	"net"
	"time"
)

// The socket boundary keeps every send bounded and makes intermediate errors observable.
type icmpPacketConn interface {
	ReadFrom([]byte) (int, net.Addr, error)
	WriteTo([]byte, net.Addr) (int, error)
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	Close() error
}

func runICMPRound(ctx context.Context, conn icmpPacketConn, target *net.IPAddr, o icmpOptions) (result icmpResult, err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return result, fmt.Errorf("ICMP round requires deadline")
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { conn.Close(); close(closed) })
	defer func() {
		if !stop() {
			<-closed
		}
	}()
	if err = conn.SetWriteDeadline(deadline); err != nil {
		return result, err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return result, err
	}
	id := int(binary.BigEndian.Uint16(nonce))
	var request, reply icmp.Type = ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply
	protocol := 1
	if target.IP.To4() == nil {
		request, reply, protocol = ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply, 58
	}
	var dst net.Addr = target
	if !o.Privileged {
		dst = &net.UDPAddr{IP: target.IP, Zone: target.Zone}
	}
	sentAt := make(map[int]time.Time)
	received := make(map[int]bool)
	var totalRTT time.Duration
	nextSend := time.Now()
	buffer := make([]byte, 65536)
	for {
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded && result.Sent == o.Count {
				return result, nil
			}
			return result, ctx.Err()
		}
		now := time.Now()
		if !now.Before(deadline) {
			if result.Sent == o.Count {
				return result, nil
			}
			return result, context.DeadlineExceeded
		}
		if result.Sent < o.Count && !now.Before(nextSend) {
			seq := result.Sent
			message := icmp.Message{Type: request, Body: &icmp.Echo{ID: id, Seq: seq, Data: nonce}}
			encoded, e := message.Marshal(nil)
			if e != nil {
				return result, e
			}
			sent := time.Now()
			n, e := conn.WriteTo(encoded, dst)
			if e != nil {
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				return result, e
			}
			if n != len(encoded) {
				return result, io.ErrShortWrite
			}
			sentAt[seq] = sent
			result.Sent++
			nextSend = sent.Add(o.PacketInterval)
		}
		readUntil := deadline
		if result.Sent < o.Count && nextSend.Before(readUntil) {
			readUntil = nextSend
		}
		if err = conn.SetReadDeadline(readUntil); err != nil {
			return result, err
		}
		n, peer, e := conn.ReadFrom(buffer)
		if e != nil {
			if ctx.Err() != nil {
				if ctx.Err() == context.DeadlineExceeded && result.Sent == o.Count {
					return result, nil
				}
				return result, ctx.Err()
			}
			if ne, ok := e.(net.Error); ok && ne.Timeout() {
				continue
			}
			return result, e
		}
		var source net.IP
		switch a := peer.(type) {
		case *net.IPAddr:
			source = a.IP
		case *net.UDPAddr:
			source = a.IP
		}
		if !source.Equal(target.IP) {
			continue
		}
		packet := buffer[:n]
		// Some raw IPv4 sockets retain the IP header (notably Windows).
		if protocol == 1 && len(packet) >= 20 && packet[0]>>4 == 4 {
			headerLen := int(packet[0]&15) * 4
			if headerLen < 20 || headerLen > len(packet) {
				continue
			}
			packet = packet[headerLen:]
		}
		message, e := icmp.ParseMessage(protocol, packet)
		if e != nil || message.Type != reply {
			continue
		}
		echo, ok := message.Body.(*icmp.Echo)
		if !ok || !bytes.Equal(echo.Data, nonce) || (o.Privileged && echo.ID != id) {
			continue
		}
		sent, ok := sentAt[echo.Seq]
		if !ok || received[echo.Seq] {
			continue
		}
		received[echo.Seq] = true
		result.Received++
		totalRTT += time.Since(sent)
		result.AvgRTT = totalRTT / time.Duration(result.Received)
		if result.Received == o.Count {
			return result, nil
		}
	}
}
