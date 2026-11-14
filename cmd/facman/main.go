// Command facman is the entry point for the Agentic Software Factory Manager.
package main

import (
	"flag"
	"io"
	"os"

	"github.com/stainedhead/software-factory-manager/internal/domain/appinfo"
)

// version is the application version; overridden at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(appinfo.Name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Bool("about", false, "print application information and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	// --about and the no-argument case both print application information until the CLI grows commands.
	text := appinfo.About(version)
	if *showVersion {
		text = appinfo.VersionLine(version) + "\n"
	}
	if _, err := io.WriteString(stdout, text); err != nil {
		return 1
	}
	return 0
}
