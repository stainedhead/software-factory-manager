// Command software-factory-manager is the entry point for the software factory manager.
package main

import "fmt"

// version is the application version; overridden at build time via -ldflags.
var version = "dev"

func main() {
	fmt.Println("software-factory-manager", version)
}
