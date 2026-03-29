package main

import (
	"fmt"
	"os"

	"github.com/JakobAIOdev/GoPilot/internal/app"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "version", "-v":
			fmt.Println(version)
			return
		}
	}

	if err := app.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
