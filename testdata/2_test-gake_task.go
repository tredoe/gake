// +build gake

package main

import (
	"fmt"

	"github.com/tredoe/gake/tasking"
)

// TaskBye says something.
func TaskBye(t *tasking.T) {
	fmt.Println("Bye!")
	//t.Log(`Testing "Bye" function`)
}
