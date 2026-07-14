package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

var databaseNames = []string{"whatsapp_ai", "whatsapp_ai_test"}

func main() {
	if err := run(context.Background(), os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, output io.Writer) error {
	adminURL := strings.TrimSpace(getenv("DATABASE_ADMIN_URL"))
	if adminURL == "" {
		return fmt.Errorf("DATABASE_ADMIN_URL is required")
	}
	appPassword := getenv("DATABASE_APP_PASSWORD")
	if appPassword == "" {
		return fmt.Errorf("DATABASE_APP_PASSWORD is required")
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect to database administrator endpoint failed")
	}
	defer func() { _ = conn.Close(context.Background()) }()

	var roleExists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'whatsapp_app')").Scan(&roleExists); err != nil {
		return fmt.Errorf("check application role failed")
	}
	if !roleExists {
		statement := "CREATE ROLE whatsapp_app LOGIN PASSWORD " + quoteLiteral(appPassword) +
			" NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOINHERIT"
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("create application role failed")
		}
		fmt.Fprintln(output, "whatsapp_app created")
	} else {
		var restricted bool
		if err := conn.QueryRow(ctx, `
			SELECT rolcanlogin AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolbypassrls AND NOT rolinherit
			FROM pg_roles WHERE rolname = 'whatsapp_app'
		`).Scan(&restricted); err != nil || !restricted {
			return fmt.Errorf("existing whatsapp_app role is not restricted")
		}
		fmt.Fprintln(output, "whatsapp_app already exists")
	}

	for _, name := range databaseNames {
		var exists bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
			return fmt.Errorf("check database %s failed", name)
		}
		if exists {
			fmt.Fprintf(output, "%s already exists\n", name)
		} else {
			if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
				return fmt.Errorf("create database %s failed", name)
			}
			fmt.Fprintf(output, "%s created\n", name)
		}
		if _, err := conn.Exec(ctx, "GRANT CONNECT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" TO whatsapp_app"); err != nil {
			return fmt.Errorf("grant database %s access failed", name)
		}
	}
	return nil
}

func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
