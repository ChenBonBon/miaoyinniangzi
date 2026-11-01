// Minimal source worker: no Wails, SQLite, credential store or native player.
package main

import (
	"miaoyinniangzi/internal/sourceruntime"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case sourceruntime.WorkerFlag:
		os.Exit(sourceruntime.WorkerMain())
	case sourceruntime.ResolveWorkerFlag:
		os.Exit(sourceruntime.ResolveWorkerMain())
	default:
		os.Exit(2)
	}
}
