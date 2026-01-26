package main

import (
	"flag"
	_ "time/tzdata"

	"github.com/acepanel/helper/pkg/config"
)

func main() {
	verbose := flag.String("v", "", "verbose mode, optionally specify log file path")
	flag.Parse()

	if *verbose != "" {
		config.Global.Verbose = true
		// 如果不是 "true" 或 "1"，则作为文件路径
		if *verbose != "true" && *verbose != "1" {
			config.Global.LogFile = *verbose
			if err := config.Global.InitLogFile(); err != nil {
				panic(err)
			}
			defer config.Global.CloseLogFile()
		}
	}

	helper, err := initHelper()
	if err != nil {
		panic(err)
	}

	if err = helper.Run(); err != nil {
		panic(err)
	}
}
