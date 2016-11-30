// Copyright 2014 Jonas mg
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command Gake is a utility that automatically builds executable programs from
// Go source code by reading files whose name ends '_task.go' that contains
// the TaskXxx functions, which are run and work just like in package "testing".
//
// By default, the binary built is temporary unless it is used -c or -keep flag;
// both flags check if the binary has to be re-compiled due to source code updated.
//
// "-keep" flag stores the compiled binaries into a global directory under
// 'HOME/.task'
package main

import (
	"flag"
	"fmt"
	"hash/adler32"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
)

const (
	CMD_EXT = ".task"

	// BIN_NAME is the name of the compiled program
	BIN_NAME = "gake" + CMD_EXT

	// SUBDIR_HOME is the directory where are stored the compiled programs
	SUBDIR_HOME = CMD_EXT
)

func main() {
	flag.Parse()

	// Get the home directory for the compiled programs
	HOME := os.Getenv(ENV_HOME)
	if HOME == "" {
		// In Unix systems, the environment variable is not set during boot init.
		if runtime.GOOS != "windows" {
			user, err := user.Current()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			} else {
				if user.Uid == "0" { // root
					HOME = "/root"
				}
			}
		}
		if HOME == "" {
			fmt.Fprintf(os.Stderr, "environment variable %s is not set\n", ENV_HOME)
			os.Exit(1)
		}
	}
	HOME = filepath.Join(HOME, SUBDIR_HOME)

	args := flag.Args()
	if len(args) == 0 {
		args = append(args, ".")
	}

	dir := args[0]
	cmdPath := ""
	isNew := false

	// Use global directory
	if !*taskC {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}
		crc := adler32.Checksum([]byte(absDir))
		homeDir := HOME + string(os.PathSeparator) + strconv.FormatUint(uint64(crc), 10)
		cmdPath = homeDir + string(os.PathSeparator) + BIN_NAME

		if _, err = os.Stat(homeDir); err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				os.Exit(1)
			}
			isNew = true

			if *taskKeepBinary {
				err = os.MkdirAll(homeDir, 0750)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err)
					os.Exit(1)
				}
			}
		}
	} else {
		// Binary is compiled in actual directory.
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}

		cmdPath = wd + string(os.PathSeparator) + filepath.Base(dir) + CMD_EXT
	}
	if runtime.GOOS == "windows" {
		cmdPath += ".exe"
	}

	if isNew || hasNewCode(dir, cmdPath) {
		pkg, err := ParseDir(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}
		if err = BuildAndRun(pkg, cmdPath); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}
	} else {
		Run(cmdPath)
	}
}

// hasNewCode checks if code in given directory has been updated; the modification
// time has to be after than the command one.
// Also, if the command does not exist and -taskC flag is set, then it returns true.
func hasNewCode(dir, cmdPath string) bool {
	files, err := filepath.Glob(dir + string(os.PathSeparator) + "*" + SUFFIX_TASKFILE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hasNewCode(): %s\n", err)
		return false
	}

	cmdInfo, err := os.Stat(cmdPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "hasNewCode(): %s\n", err)
		}

		if *taskC {
			return true
		}
		return false
	}
	cmdModTime := cmdInfo.ModTime()

	// Get last modification time for task files
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hasNewCode(): %s\n", err)
			return false
		}

		if info.ModTime().After(cmdModTime) {
			*taskKeepBinary = true
			return true
		}
	}

	return false
}
