package main

import (
	"flag"
	_ "time/tzdata"

	"github.com/acepanel/helper/pkg/config"
)

func main() {
	verbose := flag.Bool("v", false, "verbose mode")
	flag.Parse()

	config.Global.Verbose = *verbose

	helper, err := initHelper()
	if err != nil {
		panic(err)
	}

	if err = helper.Run(); err != nil {
		panic(err)
	}
}
