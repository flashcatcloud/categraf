//go:build windows

package ping

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/types"
)

type roundTripTimeStats struct {
	min int
	avg int
	max int
}

type statistics struct {
	packetsTransmitted int
	replyReceived      int
	packetsReceived    int
	roundTripTimeStats
}

func (ins *Instance) execGather(slist *types.SampleList, target string) {
	if ins.DebugMod {
		log.Println("D! ping...", target)
	}

	fields := map[string]interface{}{"result_code": 0}
	labels := map[string]string{"target": target}
	defer func() {
		for field, value := range fields {
			slist.PushFront(types.NewSample(inputName, field, value, labels))
		}
	}()
	args := ins.args(target)
	totalTimeout := 60.0
	totalTimeout = ins.timeout() * float64(ins.Count)

	out, err := ins.pingHost(ins.Binary, totalTimeout, args...)
	// ping host return exitcode != 0 also when there was no response from host but command was executed successfully
	var pendingError error
	if err != nil {
		// Combine go err + stderr output
		pendingError = errors.New(strings.TrimSpace(out) + ", " + err.Error())
	}
	stats, err := processPingOutput(out)
	if err != nil {
		// fatal error
		if pendingError != nil {
			log.Println(target, fmt.Errorf("%s: %w", target, pendingError))
		} else {
			log.Println(target, fmt.Errorf("%s: %w", target, err))
		}

		fields["result_code"] = 2
		fields["errors"] = 100.0
		return
	}
	// Calculate packet loss percentage
	lossReply := float64(stats.packetsTransmitted-stats.replyReceived) / float64(stats.packetsTransmitted) * 100.0
	lossPackets := float64(stats.packetsTransmitted-stats.packetsReceived) / float64(stats.packetsTransmitted) * 100.0

	fields["packets_transmitted"] = stats.packetsTransmitted
	fields["reply_received"] = stats.replyReceived
	fields["packets_received"] = stats.packetsReceived
	fields["percent_packet_loss"] = lossPackets
	fields["percent_reply_loss"] = lossReply
	if stats.avg >= 0 {
		fields["average_response_ms"] = float64(stats.avg)
	}
	if stats.min >= 0 {
		fields["minimum_response_ms"] = float64(stats.min)
	}
	if stats.max >= 0 {
		fields["maximum_response_ms"] = float64(stats.max)
	}
}

// args returns the arguments for the 'ping' executable
func (ins *Instance) args(url string) []string {
	args := []string{"-n", strconv.Itoa(ins.Count)}

	if ins.Timeout > 0 {
		args = append(args, "-w", strconv.FormatFloat(ins.Timeout*1000, 'f', 0, 64))
	}

	args = append(args, url)

	return args
}

// processPingOutput takes in a string output from the ping command
// based on linux implementation but using regex (multi-language support)
// It returns (<transmitted packets>, <received reply>, <received packet>, <average response>, <min response>, <max response>)
func processPingOutput(out string) (statistics, error) {
	// So find a line contain 3 numbers except reply lines
	var statsLine, aproxs []string = nil, nil
	err := errors.New("fatal error processing ping output")
	stat := regexp.MustCompile(`=\W*(\d+)\D*=\W*(\d+)\D*=\W*(\d+)`)
	aprox := regexp.MustCompile(`=\W*(\d+)\D*ms\D*=\W*(\d+)\D*ms\D*=\W*(\d+)\D*ms`)
	tttLine := regexp.MustCompile(`TTL=\d+`)
	lines := strings.Split(out, "\n")
	var replyReceived = 0
	for _, line := range lines {
		if tttLine.MatchString(line) {
			replyReceived++
		} else {
			if statsLine == nil {
				statsLine = stat.FindStringSubmatch(line)
			}
			if statsLine != nil && aproxs == nil {
				aproxs = aprox.FindStringSubmatch(line)
			}
		}
	}

	stats := statistics{
		packetsTransmitted: 0,
		replyReceived:      0,
		packetsReceived:    0,
		roundTripTimeStats: roundTripTimeStats{
			min: -1,
			avg: -1,
			max: -1,
		},
	}

	// statsLine data should contain 4 members: entireExpression + ( Send, Receive, Lost )
	if len(statsLine) != 4 {
		return stats, err
	}
	packetsTransmitted, err := strconv.Atoi(statsLine[1])
	if err != nil {
		return stats, err
	}
	packetsReceived, err := strconv.Atoi(statsLine[2])
	if err != nil {
		return stats, err
	}

	stats.packetsTransmitted = packetsTransmitted
	stats.replyReceived = replyReceived
	stats.packetsReceived = packetsReceived

	// aproxs data should contain 4 members: entireExpression + ( min, max, avg )
	if len(aproxs) != 4 {
		return stats, err
	}
	min, err := strconv.Atoi(aproxs[1])
	if err != nil {
		return stats, err
	}
	max, err := strconv.Atoi(aproxs[2])
	if err != nil {
		return stats, err
	}
	avg, err := strconv.Atoi(aproxs[3])
	if err != nil {
		return statistics{}, err
	}

	stats.avg = avg
	stats.min = min
	stats.max = max

	return stats, err
}

func (ins *Instance) timeout() float64 {
	// According to MSDN, default ping timeout for windows is 4 second
	// Add also one second interval

	if ins.Timeout > 0 {
		return ins.Timeout + 1
	}
	return 4 + 1
}
