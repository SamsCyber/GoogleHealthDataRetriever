package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
)

// appendToCSV writes a titled table to path. If the file already exists
// (for example the MFP macros export), the table goes underneath the
// existing content after a blank line. Otherwise a new file is created.
func appendToCSV(path string, title string, header []string, rows [][]string) error {
	_, err := os.Stat(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", path, err)
	}

	// O_APPEND: always write at the end, never overwrite existing content
	// O_CREATE: create the file if it doesn't exist
	// O_WRONLY: we only need to write
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.UseCRLF = true // Windows line endings, matching the Python export

	if exists {
		if err := w.Write([]string{}); err != nil { // blank separator line
			return err
		}
	}
	if err := w.Write([]string{title}); err != nil {
		return err
	}
	if err := w.Write(header); err != nil {
		return err
	}

	// WriteAll writes every row, then flushes the buffer to the file
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	if exists {
		fmt.Println("Appended to", path)
	} else {
		fmt.Println("Created", path)
	}
	return nil
}
