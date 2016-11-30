// Copyright 2014 Jonas mg
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

var taskUsage = func() {
	fmt.Fprintf(os.Stderr, `Usage: gake [-c] [-x] [-keep] [task flags] path 
[extra arguments to be passed to a task]

  -c=false: compile but do not run the binary
  -x=false: print command lines as they are executed
  -keep=false: keep the compiled binary

  // These flags (used by gake/tasking) can be passed with or without a "task."
  // prefix: -v or -task.v
  -cpu="": passes -task.cpu
  -parallel=0: passes -task.parallel
  -run="": passes -task.run
  -short=false: passes -task.short
  -timeout=0: passes -task.timeout
  -v=false: passes -task.v
`)
	os.Exit(2)
}

var (
	taskC = flag.Bool("c", false, "compile but do not run the binary")
	taskX = flag.Bool("x", false, "print command lines as they are executed")

	taskCPU      string
	taskParallel int
	taskRun      string
	taskShort    bool
	taskTimeout  time.Duration
	taskV        bool
)

func init() {
	flag.StringVar(&taskCPU, "cpu", "", "passes -task.cpu")
	flag.StringVar(&taskCPU, "task.cpu", "", "")

	flag.IntVar(&taskParallel, "parallel", 0, "passes -task.parallel")
	flag.IntVar(&taskParallel, "task.parallel", 0, "")

	flag.StringVar(&taskRun, "run", "", "passes -task.run")
	flag.StringVar(&taskRun, "task.run", "", "")

	flag.BoolVar(&taskShort, "short", false, "passes -task.short")
	flag.BoolVar(&taskShort, "task.short", false, "")

	flag.DurationVar(&taskTimeout, "timeout", 0, "passes -task.timeout")
	flag.DurationVar(&taskTimeout, "task.timeout", 0, "")

	flag.BoolVar(&taskV, "v", false, "passes -task.v")
	flag.BoolVar(&taskV, "task.v", false, "")

	flag.Usage = taskUsage
}

var (
	taskKeepBinary = flag.Bool("keep", false, "keep the compiled binary")
	//taskShowPass     bool // show passing output
	//taskStreamOutput bool // show output as it is generated

	//taskKillTimeout = 3 * time.Minute
)

// getTaskArgs returns the arguments to be passed to "gake/tasking".
func getTaskArgs() []string {
	args := make([]string, 0)

	flag.Visit(func(f *flag.Flag) {
		isBoolean := false

		switch f.Name {
		case "c", "x", "keep": // Flags skipped
			return

		// Rewrite known flags to have "task" before them
		case "cpu", "parallel", "run", "short", "timeout", "v":
			f.Name = "task." + f.Name
			fallthrough
		case "task.short", "task.v":
			isBoolean = true
		}

		args = append(args, "-"+f.Name)
		if !isBoolean {
			args = append(args, f.Value.String())
		}
	})

	fargs := flag.Args()
	if len(fargs) > 1 {
		args = append(args, "-task.args")
		args = append(args, fargs[1:]...)
	}

	return args
}
