// Command yze-go-boolname runs the boolname analyzer as a standalone go/analysis
// checker (text and -json output, and as a `go vet -vettool`).
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	boolname "github.com/gomatic/yze-go-boolname"
)

// run is the analysis entry point, indirected so the binary's wiring is testable
// without invoking the real driver (which loads packages and exits the process).
var run = singlechecker.Main

func main() { run(boolname.Analyzer) }
