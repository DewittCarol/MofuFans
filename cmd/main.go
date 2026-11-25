package main

import (
	log "github.com/sirupsen/logrus"
	"mofufans/internal/database"
	"mofufans/internal/router"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] != "security" {
			log.Fatal("unknown command")
		}
		if err := securityCLI(os.Args[2:], os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}
	database.Init()
	router.Run()
}
