package main

import (
	"github.com/saucesteals/shop/internal/cli"

	// Register providers via init().
	_ "github.com/saucesteals/shop/provider/amazon"
	_ "github.com/saucesteals/shop/provider/facebook"
)

func main() {
	cli.Execute()
}
