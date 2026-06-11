// Command tracker is the entry point for the local tui ticket tracker.
package main

import (
	"fmt"
	"os"

	"tttracker/internal/app"
	"tttracker/internal/clock"
	"tttracker/internal/db"
	"tttracker/internal/paths"
	"tttracker/internal/tui"
)

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "init":
		if err := runInit(""); err != nil {
			fail(err)
		}
	case "", "tui":
		if err := runTUI(""); err != nil {
			fail(err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(2)
	}
}

func runInit(override string) error {
	dir, err := paths.DataDir(override)
	if err != nil {
		return err
	}
	if err := paths.EnsureLayout(dir); err != nil {
		return err
	}
	database, err := db.Open(paths.DBPath(dir))
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Printf("Error closing database: %v", err)
		}
	}()
	if err := db.Migrate(database); err != nil {
		return err
	}
	fmt.Printf("Initialized tracker data at %s\n", dir)
	return nil
}

// runTUI resolves the data dir, ensures the database exists and is migrated,
// then launches the terminal UI.
func runTUI(override string) error {
	dir, err := paths.DataDir(override)
	if err != nil {
		return err
	}
	if err := paths.EnsureLayout(dir); err != nil {
		return err
	}
	database, err := db.Open(paths.DBPath(dir))
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Printf("Error closing database: %v", err)
		}
	}()
	if err := db.Migrate(database); err != nil {
		return err
	}
	application := app.New(database, clock.Real{}, paths.AttachmentsDir(dir))
	return tui.Run(application, paths.KeysPath(dir))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
