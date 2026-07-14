package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"whatsapp-ai-poc/internal/legacy"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, output io.Writer) error {
	flags := flag.NewFlagSet("import-legacy", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dryRun := flags.Bool("dry-run", false, "inspect legacy sources without writing")
	accounts := flags.String("accounts", "config/accounts.json", "legacy accounts JSON")
	knowledge := flags.String("knowledge", "config/knowledge.json", "legacy knowledge JSON")
	sqlitePath := flags.String("sqlite", getenv("DB_PATH"), "optional legacy SQLite database")
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid legacy inventory arguments")
	}
	if !*dryRun {
		return errors.New("formal import is not part of the foundation stage")
	}
	report, err := legacy.Inspect(ctx, legacy.Paths{
		AccountsJSON: *accounts, KnowledgeJSON: *knowledge, SQLite: *sqlitePath,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode legacy inventory report failed")
	}
	return nil
}
