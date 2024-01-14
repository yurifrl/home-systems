package main

import (
	"log"

	"github.com/yurifrl/home-systems/pkg/nixops"
)

func main() {

	n, err := nixops.FetchEverything()
	if err != nil {
		log.Fatal(err)
	}

	// Use the fetched data as needed
	// For example, printing some of it:
	n.PrintDeployments()
}
