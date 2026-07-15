// Command budgetctl is a small administrative CLI for the Budget Tracker's
// SQLite database. It is intended for local development and testing, and
// operates on the demo user (test).
//
// Usage:
//
//	budgetctl <command> [flags]
//
// Commands:
//
//	seed    Replace the demo user's data with the built-in demo dataset.
//	reset   Delete all financial data (requires -force). Users are preserved.
//	dump    Print the demo user's rows (transactions, budgets, recurring, bills).
//	stats   Print row counts and overall income/expense/net totals.
//
// Common flags:
//
//	-db <path>   Path to the SQLite database (default: budget.db or $BUDGET_DB).
//	-force       Required by 'reset' to confirm the destructive operation.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/budget-tracker/budget-tracker/internal/auth"
	"github.com/budget-tracker/budget-tracker/internal/budget"
	"github.com/budget-tracker/budget-tracker/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	defDB := os.Getenv("BUDGET_DB")
	if defDB == "" {
		defDB = "budget.db"
	}
	dbPath := fs.String("db", defDB, "path to the SQLite database file")
	force := fs.Bool("force", false, "confirm a destructive operation (reset)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	switch command {
	case "seed", "reset", "dump", "stats":
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "budgetctl: unknown command %q\n\n", command)
		usage()
		return 2
	}

	repo, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: open %q: %v\n", *dbPath, err)
		return 1
	}
	defer repo.Close()

	ctx := context.Background()
	if err := repo.EnsureSchema(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: ensure schema: %v\n", err)
		return 1
	}

	// All per-user commands operate on the demo user.
	user, err := auth.EnsureDemoUser(ctx, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: ensure demo user: %v\n", err)
		return 1
	}

	switch command {
	case "seed":
		return cmdSeed(ctx, repo, *dbPath, user)
	case "reset":
		return cmdReset(ctx, repo, *dbPath, *force)
	case "dump":
		return cmdDump(ctx, repo, user.ID)
	case "stats":
		return cmdStats(ctx, repo, user)
	}
	return 0
}

func usage() {
	fmt.Fprint(os.Stderr, `budgetctl - Budget Tracker admin CLI

Usage:
  budgetctl <command> [flags]

Commands:
  seed     Replace the demo user's data with the built-in demo dataset
  reset    Delete all financial data (requires -force); users are preserved
  dump     Print the demo user's rows (transactions, budgets, recurring, bills)
  stats    Print row counts and overall income/expense/net totals

Flags:
  -db <path>   Path to the SQLite database (default: budget.db or $BUDGET_DB)
  -force       Required by 'reset' to confirm the destructive operation

Examples:
  budgetctl seed -db budget.db
  budgetctl stats
  budgetctl dump
  budgetctl reset -force
`)
}

func cmdSeed(ctx context.Context, repo *store.Repo, dbPath string, user store.User) int {
	if err := repo.Seed(ctx, user.ID); err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: seed: %v\n", err)
		return 1
	}
	fmt.Printf("Seeded demo data for user %q into %s.\n", user.Username, dbPath)
	return cmdStats(ctx, repo, user)
}

func cmdReset(ctx context.Context, repo *store.Repo, dbPath string, force bool) int {
	if !force {
		fmt.Fprintf(os.Stderr, "budgetctl: reset will delete ALL financial data in %s. Re-run with -force to confirm.\n", dbPath)
		return 1
	}
	if err := repo.Reset(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: reset: %v\n", err)
		return 1
	}
	fmt.Printf("Cleared all financial data in %s.\n", dbPath)
	return 0
}

func cmdDump(ctx context.Context, repo *store.Repo, userID int64) int {
	txns, err := repo.ListTransactions(ctx, userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list transactions: %v\n", err)
		return 1
	}
	budgets, err := repo.ListBudgets(ctx, userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list budgets: %v\n", err)
		return 1
	}
	recurring, err := repo.ListRecurring(ctx, userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list recurring: %v\n", err)
		return 1
	}
	bills, err := repo.ListBills(ctx, userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list bills: %v\n", err)
		return 1
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)

	fmt.Fprintf(os.Stdout, "TRANSACTIONS (%d)\n", len(txns))
	fmt.Fprintln(tw, "  ID\tDATE\tTYPE\tAMOUNT\tCATEGORY\tDESCRIPTION")
	for _, t := range txns {
		fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\t%s\t%s\n",
			t.ID, t.Date.Format("2006-01-02"), t.Type, budget.FormatAmount(t.AmountCents), t.Category, t.Description)
	}
	tw.Flush()

	fmt.Fprintf(os.Stdout, "\nBUDGETS (%d)\n", len(budgets))
	fmt.Fprintln(tw, "  CATEGORY\tMONTHLY LIMIT")
	for _, b := range budgets {
		fmt.Fprintf(tw, "  %s\t%s\n", b.Category, budget.FormatAmount(b.LimitCents))
	}
	tw.Flush()

	fmt.Fprintf(os.Stdout, "\nRECURRING (%d)\n", len(recurring))
	fmt.Fprintln(tw, "  ID\tDAY\tTYPE\tAMOUNT\tCATEGORY\tDESCRIPTION")
	for _, rr := range recurring {
		fmt.Fprintf(tw, "  %d\t%d\t%s\t%s\t%s\t%s\n",
			rr.ID, rr.DayOfMonth, rr.Type, budget.FormatAmount(rr.AmountCents), rr.Category, rr.Description)
	}
	tw.Flush()

	fmt.Fprintf(os.Stdout, "\nBILLS (%d)\n", len(bills))
	fmt.Fprintln(tw, "  ID\tPAYEE\tAMOUNT\tFREQUENCY\tCATEGORY\tDUE DAY")
	for _, b := range bills {
		due := ""
		if b.DueDay > 0 {
			due = fmt.Sprintf("%d", b.DueDay)
		}
		fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\t%s\t%s\n",
			b.ID, b.Payee, budget.FormatAmount(b.AmountCents), b.Frequency.Label(), b.Category, due)
	}
	tw.Flush()
	return 0
}

func cmdStats(ctx context.Context, repo *store.Repo, user store.User) int {
	txns, err := repo.ListTransactions(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list transactions: %v\n", err)
		return 1
	}
	budgets, err := repo.ListBudgets(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list budgets: %v\n", err)
		return 1
	}
	recurring, err := repo.ListRecurring(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list recurring: %v\n", err)
		return 1
	}
	bills, err := repo.ListBills(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list bills: %v\n", err)
		return 1
	}

	sum := budget.Summary(txns)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "User\t%s (id %d)\n", user.Username, user.ID)
	fmt.Fprintf(tw, "Transactions\t%d\n", len(txns))
	fmt.Fprintf(tw, "Budgets\t%d\n", len(budgets))
	fmt.Fprintf(tw, "Recurring rules\t%d\n", len(recurring))
	fmt.Fprintf(tw, "Bills\t%d\n", len(bills))
	fmt.Fprintf(tw, "Total income\t%s\n", budget.FormatAmount(sum.TotalIncomeCents))
	fmt.Fprintf(tw, "Total expense\t%s\n", budget.FormatAmount(sum.TotalExpenseCents))
	fmt.Fprintf(tw, "Net balance\t%s\n", budget.FormatAmount(sum.NetBalanceCents))
	tw.Flush()
	return 0
}
