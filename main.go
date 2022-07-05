package main

import (
	"flag"
	"log"

	"github.com/joho/godotenv"
)

func main() {
	var (
		debug  = flag.Bool("debug", false, "enable debug?")
		dotenv = flag.Bool("dotenv", false, "load .env?")
	)
	flag.Parse()

	if *dotenv {
		err := godotenv.Load()
		if err != nil {
			log.Fatal(err)
		}
	}

	app := App{Debug: *debug}
	app.Initalize()
	app.InitalizeCallbacks()
	app.InitializeSlack()
	app.RunLoop()
}
