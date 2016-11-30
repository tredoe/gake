// Copyright 2009 The Go Authors.
// Copyright 2014 Jonas mg
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// http://golang.org/src/pkg/testing/testing.go

// Package tasking provides support for automated run of Go packages.
// It is intended to be used in concert with the "gake" command, which
// automates execution of any function of the form
//     func TaskXxx(*tasking.T)
// where Xxx can be any alphanumeric string (but the first letter must not be in
// [a-z]) and serves to identify the task routine.
//
// Within these functions, use the Error, Fail or related methods to signal failure.
//
// To write a new task suite, create a file whose name ends _task.go that
// contains the TaskXxx functions as described here. The file will be excluded
// from regular package builds but will be included when the ``gake'' command
// is run.
//
// Tasks may be skipped if not applicable with a call to the Skip method of *T:
//     func TaskTimeConsuming(t *tasking.T) {
//         if tasking.Short() {
//             t.Skip("skipping task in short mode.")
//         }
//         ...
//     }
//
// For detail about flags, run "gake -help".
package tasking

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	//"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// The short flag requests that tasks run more quickly, but its functionality
	// is provided by task writers themselves.  The tasking package is just its
	// home.  By default the flag is off so a plain "gake" will do a
	// full run of the package.
	short = flag.Bool("task.short", false, "run smaller task suite to save time")

	// The directory in which to create profile files and the like. When run from
	// "gake", the binary always runs in the source directory for the package;
	// this flag lets "gake" tell the binary to write the files in the directory where
	// the "gake" command is run.
	//outputDir = flag.String("task.outputdir", "", "directory in which to write profiles")

	// Report as tasks are run; default is silent for success.
	chatty = flag.Bool("task.v", false, "verbose: print additional output")
	//coverProfile     = flag.String("task.coverprofile", "", "write a coverage profile to the named file after execution")
	match = flag.String("task.run", "", "regular expression to select tasks to run")
	//memProfile       = flag.String("task.memprofile", "", "write a memory profile to the named file after execution")
	//memProfileRate   = flag.Int("task.memprofilerate", 0, "if >=0, sets runtime.MemProfileRate")
	//cpuProfile       = flag.String("task.cpuprofile", "", "write a cpu profile to the named file during execution")
	//blockProfile     = flag.String("task.blockprofile", "", "write a goroutine blocking profile to the named file after execution")
	//blockProfileRate = flag.Int("task.blockprofilerate", 1, "if >= 0, calls runtime.SetBlockProfileRate()")
	timeout    = flag.Duration("task.timeout", 0, "if positive, sets an aggregate time limit for all tasks")
	cpuListStr = flag.String("task.cpu", "", "comma-separated list of number of CPUs to use for each task")
	parallel   = flag.Int("task.parallel", runtime.GOMAXPROCS(0), "maximum task parallelism")

	//haveExamples bool // are there examples?

	cpuList []int
)

var eargs = flag.String("task.args", "", "comma-separated list of extra arguments to be used by some task")

// Args returns the extra arguments, if any.
func Args() []string { return strings.Split(*eargs, ",") }

// common holds the elements common for M and captures common methods
// such as Errorf.
type common struct {
	mu       sync.RWMutex // guards output and failed
	output   []byte       // Output generated by task.
	failed   bool         // Task has failed.
	skipped  bool         // Task has been skipped.
	finished bool

	start    time.Time // Time task started
	duration time.Duration
	self     interface{}      // To be sent on signal channel when done.
	signal   chan interface{} // Output for serial tasks.
}

// Short reports whether the -task.short flag is set.
func Short() bool {
	return *short
}

// Verbose reports whether the -task.v flag is set.
func Verbose() bool {
	return *chatty
}

// decorate prefixes the string with the file and line of the call site
// and inserts the final newline if needed and indentation tabs for formatting.
func decorate(s string) string {
	_, file, line, ok := runtime.Caller(3) // decorate + log + public function.
	if ok {
		// Truncate file name at last file name separator.
		if index := strings.LastIndex(file, "/"); index >= 0 {
			file = file[index+1:]
		} else if index = strings.LastIndex(file, "\\"); index >= 0 {
			file = file[index+1:]
		}
	} else {
		file = "???"
		line = 1
	}
	buf := new(bytes.Buffer)
	// Every line is indented at least one tab.
	buf.WriteByte('\t')
	fmt.Fprintf(buf, "%s:%d: ", file, line)
	lines := strings.Split(s, "\n")
	if l := len(lines); l > 1 && lines[l-1] == "" {
		lines = lines[:l-1]
	}
	for i, line := range lines {
		if i > 0 {
			// Second and subsequent lines are indented an extra tab.
			buf.WriteString("\n\t\t")
		}
		buf.WriteString(line)
	}
	buf.WriteByte('\n')
	return buf.String()
}

// TB is the interface common to T.
/*type TB interface {
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fail()
	FailNow()
	Failed() bool
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Skip(args ...interface{})
	SkipNow()
	Skipf(format string, args ...interface{})
	Skipped() bool

	// A private method to prevent users implementing the
	// interface and so future additions to it will not
	// violate Go 1 compatibility.
	private()
}

var _ TB = (*T)(nil)*/

// T is a type passed to Task functions to manage task state and support formatted task logs.
// Logs are accumulated during execution and dumped to standard error when done.
type T struct {
	common
	name          string    // Name of task.
	startParallel chan bool // Parallel tasks will wait on this.
}

func (c *common) private() {}

// Fail marks the function as having failed but continues execution.
func (c *common) Fail() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failed = true
}

// Failed reports whether the function has failed.
func (c *common) Failed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.failed
}

// FailNow marks the function as having failed and stops its execution.
// Execution will continue at the next task.
// FailNow must be called from the goroutine running the task function,
// not from other goroutines created during the task. Calling FailNow
// does not stop those other goroutines.
func (c *common) FailNow() {
	c.Fail()

	// Calling runtime.Goexit will exit the goroutine, which
	// will run the deferred functions in this goroutine,
	// which will eventually run the deferred lines in tRunner,
	// which will signal to the task loop that this task is done.
	//
	// A previous version of this code said:
	//
	//	c.duration = ...
	//	c.signal <- c.self
	//	runtime.Goexit()
	//
	// This previous version duplicated code (those lines are in
	// tRunner no matter what), but worse the goroutine teardown
	// implicit in runtime.Goexit was not guaranteed to complete
	// before the task exited.  If a task deferred an important cleanup
	// function (like removing temporary files), there was no guarantee
	// it would run on a task failure.  Because we send on c.signal during
	// a top-of-stack deferred function now, we know that the send
	// only happens after any other stacked defers have completed.
	c.finished = true
	runtime.Goexit()
}

// log generates the output. It's always at the same stack depth.
func (c *common) log(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.output = append(c.output, decorate(s)...)
}

// Log formats its arguments using default formatting, analogous to Println,
// and records the text in the error log. The text will be printed only if
// the task fails or the -task.v flag is set.
func (c *common) Log(args ...interface{}) { c.log(fmt.Sprintln(args...)) }

// Logf formats its arguments according to the format, analogous to Printf,
// and records the text in the error log. The text will be printed only if
// the task fails or the -task.v flag is set.
func (c *common) Logf(format string, args ...interface{}) { c.log(fmt.Sprintf(format, args...)) }

// Error is equivalent to Log followed by Fail.
func (c *common) Error(args ...interface{}) {
	c.log(fmt.Sprintln(args...))
	c.Fail()
}

// Errorf is equivalent to Logf followed by Fail.
func (c *common) Errorf(format string, args ...interface{}) {
	c.log(fmt.Sprintf(format, args...))
	c.Fail()
}

// Fatal is equivalent to Log followed by FailNow.
func (c *common) Fatal(args ...interface{}) {
	c.log(fmt.Sprintln(args...))
	c.FailNow()
}

// Fatalf is equivalent to Logf followed by FailNow.
func (c *common) Fatalf(format string, args ...interface{}) {
	c.log(fmt.Sprintf(format, args...))
	c.FailNow()
}

// Skip is equivalent to Log followed by SkipNow.
func (c *common) Skip(args ...interface{}) {
	c.log(fmt.Sprintln(args...))
	c.SkipNow()
}

// Skipf is equivalent to Logf followed by SkipNow.
func (c *common) Skipf(format string, args ...interface{}) {
	c.log(fmt.Sprintf(format, args...))
	c.SkipNow()
}

// SkipNow marks the task as having been skipped and stops its execution.
// Execution will continue at the next task. See also FailNow.
// SkipNow must be called from the goroutine running the task, not from
// other goroutines created during the task. Calling SkipNow does not stop
// those other goroutines.
func (c *common) SkipNow() {
	c.skip()
	c.finished = true
	runtime.Goexit()
}

func (c *common) skip() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skipped = true
}

// Skipped reports whether the task was skipped.
func (c *common) Skipped() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.skipped
}

// Parallel signals that this task is to be run in parallel with (and only with)
// other parallel tasks.
func (t *T) Parallel() {
	t.signal <- (*T)(nil) // Release main run tasks loop
	<-t.startParallel     // Wait for serial tasks to finish
	// Assuming Parallel is the first thing a task does, which is reasonable,
	// reinitialize the task's start time because it's actually starting now.
	t.start = time.Now()
}

// An internal type but exported because it is cross-package; part of the
// implementation of the "gake" command.
type InternalTask struct {
	Name string
	F    func(*T)
}

func tRunner(t *T, task *InternalTask) {
	// When this goroutine is done, either because task.F(t)
	// returned normally or because a task failure triggered
	// a call to runtime.Goexit, record the duration and send
	// a signal saying that the task is done.
	defer func() {
		t.duration = time.Now().Sub(t.start)
		// If the task panicked, print any task output before dying.
		err := recover()
		if !t.finished && err == nil {
			err = fmt.Errorf("task executed panic(nil) or runtime.Goexit")
		}
		if err != nil {
			t.Fail()
			t.report()
			panic(err)
		}
		t.signal <- t
	}()

	t.start = time.Now()
	task.F(t)
	t.finished = true
}

// An internal function but exported because it is cross-package;
// part of the implementation of the "gake" command.
func Main(matchString func(pat, str string) (bool, error), tasks []InternalTask) {
	flag.Parse()
	parseCpuList()

	//before()
	startAlarm()
	//haveExamples = len(examples) > 0
	taskOk := RunTasks(matchString, tasks)
	//exampleOk := RunExamples(matchString, examples)
	stopAlarm()
	if !taskOk /*|| !exampleOk*/ {
		fmt.Println("FAIL")
		//after()
		os.Exit(1)
	}
	fmt.Println("PASS")
	//RunBenchmarks(matchString, benchmarks)
	//after()
}

func (t *T) report() {
	tstr := fmt.Sprintf("(%.2f seconds)", t.duration.Seconds())
	format := "--- %s: %s %s\n%s"
	if t.Failed() {
		fmt.Printf(format, "FAIL", t.name, tstr, t.output)
	} else if *chatty {
		if t.Skipped() {
			fmt.Printf(format, "SKIP", t.name, tstr, t.output)
		} else {
			fmt.Printf(format, "PASS", t.name, tstr, t.output)
		}
	}
}

func RunTasks(matchString func(pat, str string) (bool, error), tasks []InternalTask) (ok bool) {
	ok = true
	if len(tasks) == 0 /*&& !haveExamples*/ {
		fmt.Fprintln(os.Stderr, "tasking: warning: no tasks to run")
		return
	}
	for _, procs := range cpuList {
		runtime.GOMAXPROCS(procs)
		// We build a new channel tree for each run of the loop.
		// collector merges in one channel all the upstream signals from parallel tasks.
		// If all tasks pump to the same channel, a bug can occur where a task
		// kicks off a goroutine that Fails, yet the task still delivers a completion signal,
		// which skews the counting.
		var collector = make(chan interface{})

		numParallel := 0
		startParallel := make(chan bool)

		for i := 0; i < len(tasks); i++ {
			matched, err := matchString(*match, tasks[i].Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tasking: invalid regexp for -task.run: %s\n", err)
				os.Exit(1)
			}
			if !matched {
				continue
			}
			taskName := tasks[i].Name
			if procs != 1 {
				taskName = fmt.Sprintf("%s-%d", tasks[i].Name, procs)
			}
			t := &T{
				common: common{
					signal: make(chan interface{}),
				},
				name:          taskName,
				startParallel: startParallel,
			}
			t.self = t
			if *chatty {
				fmt.Printf("=== RUN %s\n", t.name)
			}
			go tRunner(t, &tasks[i])
			out := (<-t.signal).(*T)
			if out == nil { // Parallel run.
				go func() {
					collector <- <-t.signal
				}()
				numParallel++
				continue
			}
			t.report()
			ok = ok && !out.Failed()
		}

		running := 0
		for numParallel+running > 0 {
			if running < *parallel && numParallel > 0 {
				startParallel <- true
				running++
				numParallel--
				continue
			}
			t := (<-collector).(*T)
			t.report()
			ok = ok && !t.Failed()
			running--
		}
	}
	return
}

// before runs before all run tasks.
/*func before() {
	if *memProfileRate > 0 {
		runtime.MemProfileRate = *memProfileRate
	}
	if *cpuProfile != "" {
		f, err := os.Create(toOutputDir(*cpuProfile))
		if err != nil {
			fmt.Fprintf(os.Stderr, "tasking: %s", err)
			return
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "tasking: can't start cpu profile: %s", err)
			f.Close()
			return
		}
		// Could save f so after can call f.Close; not worth the effort.
	}
	if *blockProfile != "" && *blockProfileRate >= 0 {
		runtime.SetBlockProfileRate(*blockProfileRate)
	}
	if *coverProfile != "" && cover.Mode == "" {
		fmt.Fprintf(os.Stderr, "tasking: cannot use -task.coverprofile because task binary was not built with coverage enabled\n")
		os.Exit(2)
	}
}*/

// after runs after all run tasks.
/*func after() {
	if *cpuProfile != "" {
		pprof.StopCPUProfile() // flushes profile to disk
	}
	if *memProfile != "" {
		f, err := os.Create(toOutputDir(*memProfile))
		if err != nil {
			fmt.Fprintf(os.Stderr, "tasking: %s\n", err)
			os.Exit(2)
		}
		if err = pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "tasking: can't write %s: %s\n", *memProfile, err)
			os.Exit(2)
		}
		f.Close()
	}
	if *blockProfile != "" && *blockProfileRate >= 0 {
		f, err := os.Create(toOutputDir(*blockProfile))
		if err != nil {
			fmt.Fprintf(os.Stderr, "tasking: %s\n", err)
			os.Exit(2)
		}
		if err = pprof.Lookup("block").WriteTo(f, 0); err != nil {
			fmt.Fprintf(os.Stderr, "tasking: can't write %s: %s\n", *blockProfile, err)
			os.Exit(2)
		}
		f.Close()
	}
	if cover.Mode != "" {
		coverReport()
	}
}*/

// toOutputDir returns the file name relocated, if required, to outputDir.
// Simple implementation to avoid pulling in path/filepath.
/*func toOutputDir(path string) string {
	if *outputDir == "" || path == "" {
		return path
	}
	if runtime.GOOS == "windows" {
		// On Windows, it's clumsy, but we can be almost always correct
		// by just looking for a drive letter and a colon.
		// Absolute paths always have a drive letter (ignoring UNC).
		// Problem: if path == "C:A" and outputdir == "C:\Go" it's unclear
		// what to do, but even then path/filepath doesn't help.
		// TODO: Worth doing better? Probably not, because we're here only
		// under the management of "gake".
		if len(path) >= 2 {
			letter, colon := path[0], path[1]
			if ('a' <= letter && letter <= 'z' || 'A' <= letter && letter <= 'Z') && colon == ':' {
				// If path starts with a drive letter we're stuck with it regardless.
				return path
			}
		}
	}
	if os.IsPathSeparator(path[0]) {
		return path
	}
	return fmt.Sprintf("%s%c%s", *outputDir, os.PathSeparator, path)
}*/

var timer *time.Timer

// startAlarm starts an alarm if requested.
func startAlarm() {
	if *timeout > 0 {
		timer = time.AfterFunc(*timeout, func() {
			panic(fmt.Sprintf("task timed out after %v", *timeout))
		})
	}
}

// stopAlarm turns off the alarm.
func stopAlarm() {
	if *timeout > 0 {
		timer.Stop()
	}
}

func parseCpuList() {
	for _, val := range strings.Split(*cpuListStr, ",") {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		cpu, err := strconv.Atoi(val)
		if err != nil || cpu <= 0 {
			fmt.Fprintf(os.Stderr, "tasking: invalid value %q for -task.cpu\n", val)
			os.Exit(1)
		}
		cpuList = append(cpuList, cpu)
	}
	if cpuList == nil {
		cpuList = append(cpuList, runtime.GOMAXPROCS(-1))
	}
}
