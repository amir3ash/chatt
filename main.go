package main

import (
	"chat-system/api"
	"chat-system/config"
	"log/slog"
	"os"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	conf, err := config.New()
	if err != nil {
		panic(err)
	}

	cli, err := api.Initialize(conf)
	if err != nil {
		panic(err)
	}
	
	cli.Run()
}
