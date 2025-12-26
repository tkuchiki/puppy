package main

import (
	"fmt"
	"github.com/tkuchiki/puppy/cli"
	"os"
)

func main() {
	c, ctx := cli.NewCLI()

	err := ctx.Run(&c)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
