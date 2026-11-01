//go:build !windows

package sourceruntime

import (
	"context"
	"io"
	"os"
	"os/exec"
)

const EngineVersion = "lx-inspect-v1"

func workerExecutable() (string, error) { return os.Executable() }

func startWorker(ctx context.Context, executable, flag string, extra ...string) (*workerProcess, error) {
	command := exec.CommandContext(ctx, executable, append([]string{flag}, extra...)...)
	command.Env = []string{"GOMEMLIMIT=64MiB", "GOGC=50"}
	command.Dir = "/"
	command.Stderr = io.Discard
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err = command.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	kill := func() { _ = command.Process.Kill() }
	return &workerProcess{input: input, output: output, pid: command.Process.Pid, wait: command.Wait, kill: kill, close: func() { input.Close(); output.Close() }}, nil
}
