// image-previews warms derived images without modifying originals or business data.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"mofufans/internal/imagevariant"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	directory := flag.String("directory", "", "Image source directory")
	variants := flag.String("variants", "small-v1,thumb-v1,preview-v1", "Comma-separated display variants")
	flag.Parse()
	if *directory == "" {
		fmt.Fprintln(os.Stderr, "--directory is required")
		os.Exit(2)
	}
	entries, err := os.ReadDir(*directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var results []map[string]any
	failed := false
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.Contains("|.jpg|.jpeg|.png|.webp|.gif|", "|"+strings.ToLower(filepath.Ext(entry.Name()))+"|") {
			continue
		}
		source := filepath.Join(*directory, entry.Name())
		info, err := entry.Info()
		if err != nil {
			failed = true
			continue
		}
		row := map[string]any{"source": entry.Name(), "original_bytes": info.Size()}
		for _, kind := range strings.Split(*variants, ",") {
			path, err := imagevariant.File(context.Background(), source, kind)
			if err != nil {
				row[kind] = map[string]string{"error": err.Error()}
				failed = true
				continue
			}
			stat, err := os.Stat(path)
			if err != nil {
				failed = true
				continue
			}
			row[kind] = map[string]any{"bytes": stat.Size(), "path": path}
		}
		results = append(results, row)
	}
	_ = json.NewEncoder(os.Stdout).Encode(results)
	if failed {
		os.Exit(1)
	}
}
