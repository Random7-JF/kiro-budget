// Command budgetctl is a small administrative CLI for the Budget Tracker's
// SQLite database. It is intended for local development and testing.
//
// Usage:
//
//	budgetctl <command> [flags]
//
// Commands:
//
//	seed    Replace all data with the built-in demo dataset.
//	reset   Delete all data (requires -force). The schema is preserved.
//	dump    Print all rows in the database (transactions, budgets, recurring, bills).
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

	switch command {
	case "seed":
		return cmdSeed(ctx, repo, *dbPath)
	case "reset":
		return cmdReset(ctx, repo, *dbPath, *force)
	case "dump":
		return cmdDump(ctx, repo)
	case "stats":
		return cmdStats(ctx, repo)
	}
	return 0
}

func usage() {
	fmt.Fprint(os.Stderr, `budgetctl - Budget Tracker admin CLI

Usage:
  budgetctl <command> [flags]

Commands:
  seed     Replace all data with the built-in demo dataset
  reset    Delete all data (requires -force); schema is preserved
  dump     Print all rows (transactions, budgets, recurring, bills)
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

func cmdSeed(ctx context.Context, repo *store.Repo, dbPath string) int {
	if err := repo.Seed(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: seed: %v\n", err)
		return 1
	}
	fmt.Printf("Seeded demo data into %s.\n", dbPath)
	return cmdStats(ctx, repo)
}

func cmdReset(ctx context.Context, repo *store.Repo, dbPath string, force bool) int {
	if !force {
		fmt.Fprintf(os.Stderr, "budgetctl: reset will delete ALL data in %s. Re-run with -force to confirm.\n", dbPath)
		return 1
	}
	if err := repo.Reset(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: reset: %v\n", err)
		return 1
	}
	fmt.Printf("Cleared all data in %s.\n", dbPath)
	return 0
}

func cmdDump(ctx context.Context, repo *store.Repo) int {
	txns, err := repo.ListTransactions(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list transactions: %v\n", err)
		return 1
	}
	budgets, err := repo.ListBudgets(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list budgets: %v\n", err)
		return 1
	}
	recurring, err := repo.ListRecurring(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list recurring: %v\n", err)
		return 1
	}
	bills, err := repo.ListBills(ctx)
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

func cmdStats(ctx context.Context, repo *store.Repo) int {
	txns, err := repo.ListTransactions(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list transactions: %v\n", err)
		return 1
	}
	budgets, err := repo.ListBudgets(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list budgets: %v\n", err)
		return 1
	}
	recurring, err := repo.ListRecurring(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list recurring: %v\n", err)
		return 1
	}
	bills, err := repo.ListBills(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budgetctl: list bills: %v\n", err)
		return 1
	}

	sum := budget.Summary(txns)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
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
