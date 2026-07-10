package main

import (
	"fmt"
	"os"

	"github.com/ilyalavrenov/pantograph/internal/cli"
)

func main() {
	if err := cli.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "pantograph:", err)
		os.Exit(1)
	}
}
