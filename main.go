package main

import (
	"chat-system/api"
	"chat-system/config"
)

func main() {
	conf, err := config.New()
	if err != nil {
		panic(err)
	}
	
	cli := api.Initialize(conf)
	cli.Run()
}
