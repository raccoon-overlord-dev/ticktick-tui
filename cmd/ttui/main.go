package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/ui"
)

// version is set at build time: go build -ldflags "-X main.version=v0.1.0"
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.StringVar(&config.Path, "config", config.Path, "config file")
	flag.Parse()

	if *showVersion {
		fmt.Println("ttui", version)
		return
	}

	if flag.Arg(0) == "dev" {
		if err := runDev(flag.Args()[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "ttui:", err)
			os.Exit(1)
		}
		return
	}

	if err := ui.Run(version); err != nil {
		fmt.Fprintln(os.Stderr, "ttui:", err)
		os.Exit(1)
	}
}
