package main

import (
	"os"

	"pgtwig/internal/cli"
)

func main() {
	os.Exit(cli.NewApp().Run(os.Args[1:]))
}
