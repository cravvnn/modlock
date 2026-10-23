// Command proto-export applies the shared generator's C++ include and build-tag rules.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/common/protogen"
)

// main normalizes protoc output after SDK export annotations are generated.
func main() {
	// Resolve the module root selected by the generation script.
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Reuse the canonical C++ postprocessor for module imports and Go build tags.
	processor := protogen.NewPostProcessor(root, filepath.Join(root, "vendor"), "github.com/paralin/modlock", nil, false)
	if err := processor.ProcessAllCppFiles(filepath.Join(root, "src")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
