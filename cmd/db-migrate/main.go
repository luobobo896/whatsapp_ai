package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/migrations"
)

func main() {
	if err := run(context.Background(), os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, output *os.File) error {
	databaseURL := strings.TrimSpace(getenv("DATABASE_MIGRATION_URL"))
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_MIGRATION_URL is required")
	}
	applied, err := database.Migrate(ctx, databaseURL, migrations.FS)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		_, err = fmt.Fprintln(output, "no migrations applied")
		return err
	}
	for _, version := range applied {
		if _, err := fmt.Fprintln(output, version); err != nil {
			return err
		}
	}
	return nil
}
