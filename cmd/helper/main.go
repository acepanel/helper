package main

import (
	"flag"
	"fmt"
	"os"
	_ "time/tzdata"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/internal/ui"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flag.BoolVar(&sys.Verbose, "v", false, "show executed commands and their output")
	logPath := flag.String("log", "", "write executed commands and their output to this file")
	flag.Parse()

	if *logPath != "" {
		if err := sys.OpenLog(*logPath); err != nil {
			return err
		}
		defer sys.CloseLog()
	}

	final, err := ui.Run()
	if err != nil {
		return err
	}
	if final != "" {
		fmt.Println(final)
	}
	return nil
}
