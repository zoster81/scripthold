package main

import (
	"fmt"
	"os"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func main() {
	registry, err := sourceintelligence.DefaultLanguageRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(sourceintelligence.RenderLanguageCapabilityMatrixMarkdown(registry))
}
