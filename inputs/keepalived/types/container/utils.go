package container

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

func (k *KeepalivedContainerCollectorHost) dockerExecCmd(cmd []string) (*bytes.Buffer, error) {
	rst, err := k.dockerCli.ExecCreate(context.Background(), k.containerName, client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	})
	if err != nil {
		slog.Debug("Error creating exec container", "CMD", cmd, "error", err)

		return nil, err
	}

	response, err := k.dockerCli.ExecAttach(context.Background(), rst.ID, client.ExecAttachOptions{})
	if err != nil {
		slog.Debug("Error attaching a connection to an exec process", "CMD", cmd, "error", err)

		return nil, err
	}
	defer response.Close()

	// Non-TTY exec uses Docker's multiplexed stdout/stderr framing.
	var output bytes.Buffer
	_, err = stdcopy.StdCopy(&output, &output, response.Reader)
	if err != nil {
		slog.Debug("Error reading response from docker command",
			"error", err,
			"CMD", cmd,
		)

		return nil, err
	}

	return &output, nil
}
