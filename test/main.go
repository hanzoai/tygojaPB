package main

import (
	"log"
	"os"

	"github.com/hanzoai/tygojaPB"
)

func main() {
	gen := tygojaPB.New(tygojaPB.Config{
		Packages: map[string][]string{
			"github.com/hanzoai/tygojaPB/test/a": {"*"},
			"github.com/hanzoai/tygojaPB/test/b": {"*"},
			"github.com/hanzoai/tygojaPB/test/c": {"Example2", "Handler"},
		},
		Heading:              `declare var $app: c.Handler;`,
		WithPackageFunctions: true,
		// enable if you want to be able to import them
		// StartModifier: "export",
	})

	result, err := gen.Generate()
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile("./types.d.ts", []byte(result), 0644); err != nil {
		log.Fatal(err)
	}

	// run `npx typedoc` to generate HTML docs from the above declarations
}
