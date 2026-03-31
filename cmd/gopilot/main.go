package main

import (
	"fmt"
	"os"

	"github.com/JakobAIOdev/GoPilot/internal/app"
)

var version = "dev"

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if opts.showVersion {
		fmt.Println(version)
		return
	}

	sessionID, err := app.Run(opts.loadSessionID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if sessionID != "" {
		fmt.Printf("\nSaved session: %s\nResume later:\n  gopilot --load %s\n", sessionID, sessionID)
	}
}

type cliOptions struct {
	showVersion   bool
	loadSessionID string
}

func parseArgs(args []string) (cliOptions, error) {
	if len(args) == 0 {
		return cliOptions{}, nil
	}

	switch args[0] {
	case "--version", "version", "-v":
		return cliOptions{showVersion: true}, nil
	case "--load":
		if len(args) < 2 {
			return cliOptions{}, fmt.Errorf("missing session id for --load")
		}
		return cliOptions{loadSessionID: args[1]}, nil
	default:
		return cliOptions{}, fmt.Errorf("unknown argument %q", args[0])
	}
}
