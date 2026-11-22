package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/webappsgo/wthr/src/swagger/generator"
)

func main() {
	printRoutes := flag.Bool("print-routes", false, "print the deterministic registered API route inventory")
	flag.Parse()
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if filepath.Base(root) == "swagger" && filepath.Base(filepath.Dir(root)) == "src" {
		root = filepath.Dir(filepath.Dir(root))
	}
	if *printRoutes {
		routes, err := generator.ExtractRegisteredAPIRoutes(root)
		if err != nil {
			log.Fatal(err)
		}
		for _, route := range routes {
			if _, err := fmt.Fprintf(os.Stdout, "%s\t%s\n", route.Method, route.Path); err != nil {
				log.Fatal(err)
			}
		}
		return
	}
	if err := generator.GenerateOpenAPI(root); err != nil {
		log.Fatal(err)
	}
}
