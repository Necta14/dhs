package main

import (
	"fmt"
	"runtime"

	"github.com/Necta14/dhs/internal/i18n"
	"github.com/Necta14/dhs/internal/system"
)

func runVersion() error {
	info, err := system.Detect()
	if err != nil {
		return err
	}
	fmt.Printf("dhs %s  (%s)\n", version, runtime.Version())
	fmt.Printf(i18n.T("system: %s\n"), info)
	fmt.Printf(i18n.T("home:   %s\n"), info.Home)
	return nil
}
