package sourceruntime

import "io"

// Both launchers expose only the two protocol pipes and lifecycle controls.
// Windows creates the lowbox token and job before resuming any worker code.
type workerProcess struct {
	input  io.WriteCloser
	output io.ReadCloser
	pid    int
	wait   func() error
	kill   func()
	close  func()
}
