package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/platform/database"
)

func main() {
	if err := run(context.Background(), os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string) error {
	email := strings.ToLower(strings.TrimSpace(getenv("BOOTSTRAP_ADMIN_EMAIL")))
	password := getenv("BOOTSTRAP_ADMIN_PASSWORD")
	databaseURL := strings.TrimSpace(getenv("DATABASE_URL"))
	if email == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("BOOTSTRAP_ADMIN_EMAIL is required")
	}
	if len(password) < 12 {
		return fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD must contain at least 12 characters")
	}
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	encoded, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password failed")
	}
	pool, err := database.OpenPool(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	userID := uuid.New()
	_, err = database.WithPlatformTx(ctx, pool, func(tx pgx.Tx) (struct{}, error) {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1)", email).Scan(&exists); err != nil {
			return struct{}{}, err
		}
		if exists {
			return struct{}{}, fmt.Errorf("administrator email already exists")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $2, $3, 'active')
		`, userID, email, encoded); err != nil {
			return struct{}{}, err
		}
		_, err := tx.Exec(ctx, "INSERT INTO platform_roles (user_id, role) VALUES ($1, 'platform_admin')", userID)
		return struct{}{}, err
	})
	if err != nil {
		return fmt.Errorf("bootstrap administrator failed")
	}
	fmt.Printf("%s %s\n", email, userID)
	return nil
}
