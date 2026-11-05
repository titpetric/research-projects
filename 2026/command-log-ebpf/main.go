package main

import (
	"commandtrx/model"
	"commandtrx/probe"
	"context"
	"flag"
	"log"
	"os/signal"
)

func main() {
	if err := start(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func start(ctx context.Context) error {
	filter := flag.String("filter", "", "comma-separated list of commands to trace (e.g. go,git,docker)")
	flag.Parse()

	cfg := model.ParseFilter(*filter)

	ctx, stop := signal.NotifyContext(ctx, interruptSignals()...)
	defer stop()

	return probe.Run(ctx, cfg)
}
