package main

import (
	_ "time/tzdata"
)

func main() {
	helper, err := initHelper()
	if err != nil {
		panic(err)
	}

	if err = helper.Run(); err != nil {
		panic(err)
	}
}
