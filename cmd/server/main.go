// Command server is the entry point for the Budget Tracker web application.
// It opens the SQLite data store, ensures the schema exists, and starts the
// HTTP server.
//
// Startup sequence (Requirements 9.2, 9.3, 9.4):
//   - Open the SQLite database and run EnsureSchema (idempotent) BEFORE
//     accepting transaction requests.
//   - On success, mount the full application routes and begin serving normally.
//   - On failure, still start listening but in a degraded mode that refuses all
//     transaction requests with a Data_Store-unavailable error, until the
//     failure is resolved by manual intervention.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	budgethttp "github.com/budget-tracker/budget-tracker/internal/http"
	"github.com/budget-tracker/budget-tracker/internal/store"
)

// config holds the resolved runtime configuration.
type config struct {
	dbPath string
	addr   string
}

// loadConfig resolves configuration from flags, falling back to environment
// variables and then to sensible defaults. Kept intentionally simple.
func loadConfig() config {
	defDB := envOr("BUDGET_DB", "budget.db")
	defAddr := envOr("BUDGET_ADDR", ":8080")

	dbPath := flag.String("db", defDB, "path to the SQLite database file")
	addr := flag.String("addr", defAddr, "HTTP listen address")
	flag.Parse()

	return config{dbPath: *dbPath, addr: *addr}
}

// envOr returns the environment variable value for key, or def if it is unset
// or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	cfg := loadConfig()
	os.Exit(run(cfg))
}

// run wires up the store and HTTP server and blocks until shutdown. It returns
// a process exit code. Even when the data store cannot be initialized, the
// process stays up serving the degraded error rather than crashing silently
// (Requirement 9.4).
func run(cfg config) int {
	handler, cleanup := buildHandler(cfg)
	defer cleanup()

	srv := &http.Server{
		Addr:    cfg.addr,
		Handler: handler,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	idleClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server: graceful shutdown failed: %v", err)
		}
		close(idleClosed)
	}()

	log.Printf("server: listening on %s", cfg.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server: listen failed: %v", err)
		return 1
	}

	<-idleClosed
	return 0
}

// buildHandler opens the store and runs schema bootstrap. On success it returns
// the full application handler (Requirements 9.2, 9.3). If opening the database
// or ensuring the schema fails, it returns the degraded handler so the server
// still starts and refuses transaction requests with a Data_Store-unavailable
// error (Requirement 9.4). The returned cleanup closes the database if one was
// opened.
func buildHandler(cfg config) (http.Handler, func()) {
	noop := func() {}

	repo, err := store.Open(cfg.dbPath)
	if err != nil {
		log.Printf("server: cannot open data store %q: %v; entering degraded mode", cfg.dbPath, err)
		return budgethttp.NewDegradedHandler(), noop
	}

	cleanup := func() {
		if cerr := repo.Close(); cerr != nil {
			log.Printf("server: closing data store: %v", cerr)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := repo.EnsureSchema(ctx); err != nil {
		log.Printf("server: schema initialization failed: %v; entering degraded mode", err)
		return budgethttp.NewDegradedHandler(), cleanup
	}

	log.Printf("server: schema ready; accepting requests")
	return budgethttp.NewServer(repo).Routes(), cleanup
}
