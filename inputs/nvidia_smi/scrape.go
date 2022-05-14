package nvidia_smi

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"flashcat.cloud/categraf/pkg/cmdx"
)

func scrape(qFields []qField, nvidiaSmiCommand string) (*table, error) {
	qFieldsJoined := strings.Join(QFieldSliceToStringSlice(qFields), ",")

	cmdAndArgs := strings.Fields(nvidiaSmiCommand)
	cmdAndArgs = append(cmdAndArgs, fmt.Sprintf("--query-gpu=%s", qFieldsJoined))
	cmdAndArgs = append(cmdAndArgs, "--format=csv")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(cmdAndArgs[0], cmdAndArgs[1:]...) //nolint:gosec
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err, timeout := cmdx.RunTimeout(cmd, time.Second*5)
	if timeout {
		return nil, fmt.Errorf("run command: %s timeout", strings.Join(cmdAndArgs, " "))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to run command: %s | error: %v | stdout: %s | stderr: %s",
			strings.Join(cmdAndArgs, " "), err, stdout.String(), stderr.String())
	}

	t, err := parseCSVIntoTable(strings.TrimSpace(stdout.String()), qFields)
	if err != nil {
		return nil, err
	}

	return &t, nil
}
