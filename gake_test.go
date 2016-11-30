// Copyright 2014 Jonas mg
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"testing"

	"github.com/tredoe/goutil/cmdutil"
)

func TestCommand(t *testing.T) {
	cmdsInfo := []cmdutil.CommandInfo{
		{
			Args: "./testdata/",
			Out:  "Hello!\nBye!\nPASS\n",
		},

		{
			Args:   "./testdata/build_cons1/",
			Stderr: BuildConsError{"testdata/build_cons1/1_test-constraint_task.go"}.Error() + "\n",
		},
		{
			Args:   "./testdata/build_cons2/",
			Stderr: BuildConsPosError{"testdata/build_cons2/2_test-constraint_task.go"}.Error() + "\n",
		},
		{
			Args:   "./testdata/func_sign/",
			Stderr: "testdata/func_sign/test-signature_task.go:3:1: main.TaskTest should have the signature func(*tasking.T)\n",
		},
		{
			Args:   "./testdata/import_path/",
			Stderr: ImportPathError{"testdata/import_path/test-import_task.go"}.Error() + "\n",
		},
		{
			Args:   "./testdata/multi_pkg/",
			Stderr: "can't load package: found packages \"main\" ('testdata/multi_pkg/1_test_task.go'), \"main2\" ('testdata/multi_pkg/3_test_task.go', 'testdata/multi_pkg/2_test_task.go') in './testdata/multi_pkg/'\n",
		},
		{
			Args:   "./testdata/no_taskfile/",
			Stderr: ErrNoTaskfile.Error() + "\n",
		},
		{
			Args:   "./testdata/no_task/",
			Stderr: ErrNoTask.Error() + "\n",
		},
	}

	err := cmdutil.TestCommand(".", cmdsInfo)
	if err != nil {
		t.Fatal(err)
	}
}
