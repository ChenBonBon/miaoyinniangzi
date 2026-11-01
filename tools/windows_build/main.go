// Windows development bundle helper. No source scripts or user profiles are read.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"miaoyinniangzi/internal/mpvaudio"
	"miaoyinniangzi/internal/winbundle"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Windows build:", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("expected prepare, verify, native, sandbox or package")
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	repo := flags.String("repo", ".", "repository root")
	directory := flags.String("dir", "", "verified dependency directory")
	local := flags.String("libmpv", "", "separately extracted pinned libmpv DLL, for cross-host checks")
	exe := flags.String("exe", "", "Windows x64 application executable")
	output := flags.String("out", "", "new build/bin/windows-x64-* output folder")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	lock, err := winbundle.ReadLock(filepath.Join(root, "tools", "windows-dependencies.json"))
	if err != nil {
		return err
	}
	if *directory == "" {
		*directory = filepath.Join(root, "build", "deps", "windows-x64")
	}
	switch command {
	case "prepare":
		path, err := winbundle.Prepare(root, lock, *local)
		if err != nil {
			return err
		}
		fmt.Println("Verified dependencies:", path)
	case "verify", "native":
		report, err := winbundle.Verify(*directory, lock)
		if err != nil {
			return err
		}
		if command == "native" {
			if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
				return errors.New("native DLL loading must be tested on Windows x64")
			}
			library, err := filepath.Abs(filepath.Join(*directory, "libmpv-2.dll"))
			if err != nil {
				return err
			}
			if err = os.Setenv("MIAOYIN_MPV_LIBRARY", library); err != nil {
				return err
			}
			if err = mpvaudio.Warmup(); err != nil {
				return errors.New("native libmpv could not load or bind its client API; check bundle integrity and Windows system dependencies")
			}
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
	case "sandbox":
		if *exe == "" {
			return errors.New("sandbox requires --exe application executable")
		}
		if err := sandboxSmoke(*exe); err != nil {
			return err
		}
		fmt.Println("Packaged source worker: offline inspection and broker protocol passed (no network)")
	case "package":
		if *exe == "" || *output == "" {
			return errors.New("package requires --exe and --out")
		}
		path, err := winbundle.Package(root, *exe, *output, lock)
		if err != nil {
			return err
		}
		fmt.Println("Development portable ZIP:", path)
	default:
		return errors.New("unknown Windows build command")
	}
	return nil
}
