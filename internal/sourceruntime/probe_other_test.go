//go:build !windows

package sourceruntime

func windowsAFDControlMain() int     { return 2 }
func windowsWinsockControlMain() int { return 2 }
func windowsProbeMain() int          { return 2 }
func windowsBrokerMain() int         { return 2 }
