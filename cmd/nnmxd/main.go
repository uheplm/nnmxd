package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/uheplm/nnmxd/internal/service"
)

const version = "1.0.0"

func main() {
	var configPath string
	var candidatePath string
	var showVersion bool
	flag.StringVar(&configPath, "config", "", "path to the service JSON configuration")
	flag.StringVar(&candidatePath, "apply-config", "", "validate and atomically install a candidate config")
	flag.BoolVar(&showVersion, "version", false, "print the daemon version")
	flag.Parse()

	if showVersion {
		fmt.Printf("nnmxd %s\n", version)
		return
	}
	if candidatePath != "" {
		if configPath == "" {
			var err error
			configPath, err = service.ConfigPath()
			if err != nil {
				log.Fatal(err)
			}
		}
		if err := service.WriteConfig(configPath, candidatePath); err != nil {
			log.Fatal(err)
		}
		return
	}
	if configPath == "" {
		var err error
		configPath, err = service.ConfigPath()
		if err != nil {
			log.Fatal(err)
		}
	}
	statusPath, err := service.StatusPath()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := service.Run(ctx, configPath, statusPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
