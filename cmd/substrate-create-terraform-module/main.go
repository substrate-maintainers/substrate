package main

import (
	"flag"
	"log"

	"github.com/src-bin/substrate/terraform"
	"github.com/src-bin/substrate/ui"
	"github.com/src-bin/substrate/version"
)

func main() {
	flag.Parse()
	version.Flag()
	if flag.NArg() == 0 {
		ui.Fatal("need at least one module name to create")
	}

	for _, name := range flag.Args() {
		if err := terraform.Scaffold(name); err != nil {
			log.Fatal(err)
		}
	}
}
