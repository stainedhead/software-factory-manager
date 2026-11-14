// Package appinfo holds the application's identity: its name, title, description
// and the text shown by --version and --about. It has no dependencies.
package appinfo

import "fmt"

const (
	// Name is the binary and command users type.
	Name = "facman"
	// Title is the product's full name; the repo is software-factory-manager.
	Title = "Agentic Software Factory Manager"
	// Repository is the name of the source repository.
	Repository = "software-factory-manager"
	// Description says what the application is for.
	Description = "Gives every lab machine and CI runner the same toolset for working in software factories."
)

// VersionLine returns the one-line text printed by --version.
func VersionLine(version string) string {
	return fmt.Sprintf("%s %s (%s)", Name, orDev(version), Title)
}

// About returns the multi-line application information printed by --about.
func About(version string) string {
	return fmt.Sprintf("%s — %s\n\n%s\n\nVersion:     %s\nRepository:  %s\n",
		Name, Title, Description, orDev(version), Repository)
}

func orDev(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}
