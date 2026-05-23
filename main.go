package main

import (
	"context"
	"fmt"
	"os"

	"github.com/victorjacobs/mealie-importer/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
