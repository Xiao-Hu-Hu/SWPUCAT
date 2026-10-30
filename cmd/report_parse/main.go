package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"SWPUCAT/internal/application/reportparse"
)

func main() {
	path := flag.String("file", "", "Markdown report file to parse")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "--file is required")
		os.Exit(2)
	}
	body, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read report: %v\n", err)
		os.Exit(1)
	}
	result := reportparse.Parse(string(body))
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode parse result: %v\n", err)
		os.Exit(1)
	}
	if result.Status == reportparse.ParseFailed {
		os.Exit(1)
	}
}
