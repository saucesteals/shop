package main

import (
	"github.com/saucesteals/shop/internal/cli"

	// Register providers via init().
	_ "github.com/saucesteals/shop/internal/provider/amazon"
)

func main() {
	cli.Execute()
}
