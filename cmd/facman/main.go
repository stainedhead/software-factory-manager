// Command facman is the entry point for the Agentic Software Factory Manager.
// It only wires adapters into use cases (wire.go) and runs the CLI.
package main

import (
	"context"
	"os"
)

// version is the application version; overridden at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], defaultOptions()))
}

// run builds the application from o and runs args, returning the exit code.
func run(args []string, o options) int {
	return newApp(o).Execute(context.Background(), args)
}
