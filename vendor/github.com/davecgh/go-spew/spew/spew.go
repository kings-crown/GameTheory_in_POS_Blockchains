package spew

import (
	"fmt"
)

// Dump provides a lightweight stand-in for github.com/davecgh/go-spew/spew.Dump.
// It simply prints each argument using the Go-syntax representation.
func Dump(args ...interface{}) {
	for _, arg := range args {
		fmt.Printf("%#v\n", arg)
	}
}
