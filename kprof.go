package main

import (
	"fmt"
	"os"

	"github.com/lvrach/kprof/internal/application"
)

func main() {
	err := application.NewApp().Run(os.Args)
	if err != nil {
		fmt.Println("fail to run: ", err)
		os.Exit(1)
	}
}
