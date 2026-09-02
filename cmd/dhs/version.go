package main

import (
	"fmt"
	"runtime"

	"github.com/necta/dhs/internal/system"
)

func runVersion() error {
	info, err := system.Detect()
	if err != nil {
		return err
	}
	fmt.Printf("dhs %s  (%s)\n", version, runtime.Version())
	fmt.Printf("sistem: %s\n", info)
	fmt.Printf("acasă:  %s\n", info.Home)
	return nil
}
