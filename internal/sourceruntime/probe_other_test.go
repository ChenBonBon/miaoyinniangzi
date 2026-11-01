//go:build !windows

package sourceruntime

func windowsProbeMain() int  { return 2 }
func windowsBrokerMain() int { return 2 }
