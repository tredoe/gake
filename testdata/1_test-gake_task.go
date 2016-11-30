// +build gake

package main

import (
	"fmt"

	"github.com/tredoe/gake/tasking"
)

// TaskHello says something.
func TaskHello(t *tasking.T) {
	fmt.Println("Hello!")
	t.Log(`Testing "Hello" function`)
}
