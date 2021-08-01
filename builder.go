// Copyright 2014 Jonas mg
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"
)

// BuildAndRun uses the tool "go build" to compile the task files to file "cmdPath".
func BuildAndRun(pkg *taskPackage, cmdPath string) error {
	file, err := os.CreateTemp("", "gake-")
	if err != nil {
		return err
	}
	workDir := file.Name()

	defer os.RemoveAll(workDir)

	// Copy all files to the temporary directory.
	for _, f := range pkg.Files {
		src, err := os.ReadFile(f.Name)
		if err != nil {
			return err
		}
		err = os.WriteFile(workDir+string(os.PathSeparator)+filepath.Base(f.Name), src, 0644)
		if err != nil {
			return err
		}
	}

	// Write the main file.
	f, err := os.Create(workDir + string(os.PathSeparator) + "main_.go")
	if err != nil {
		return err
	}
	defer f.Close()
	if err = taskmainTmpl.Execute(f, pkg); err != nil {
		return err
	}

	// == Build
	if !*taskC && !*taskKeepBinary {
		cmdPath = workDir + string(os.PathSeparator) + BIN_NAME
		if runtime.GOOS == "windows" {
			cmdPath += ".exe"
		}
	}

	cmd := new(exec.Cmd)
	if !*taskX {
		cmd = exec.Command("go", "build", "--tags", "gake", "-o", cmdPath)
	} else {
		cmd = exec.Command("go", "build", "--tags", "gake", "-o", cmdPath, "-x")
	}
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr

	if err = cmd.Run(); err != nil {
		return err
	}
	// ==

	Run(cmdPath)
	return nil
}

func Run(path string) {
	if *taskC {
		return
	}
	cmd := exec.Command(path, getTaskArgs()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

var taskmainTmpl = template.Must(template.New("main").Parse(`
package main

import (
	"regexp"

	"github.com/tredoe/gake/tasking"
)

var tasks = []tasking.InternalTask{
{{range $_, $f := .Files}}{{range $f.TaskFuncs}}
	{"{{.Name}}", {{.Name}}},{{end}}{{end}}
}

var matchPat string
var matchRe *regexp.Regexp

func matchString(pat, str string) (result bool, err error) {
	if matchRe == nil || matchPat != pat {
		matchPat = pat
		matchRe, err = regexp.Compile(matchPat)
		if err != nil {
			return
		}
	}
	return matchRe.MatchString(str), nil
}

func main() {
	tasking.Main(matchString, tasks)
}
`))
