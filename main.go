package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gator/config"
	"gator/database"
	"gator/feedservice"
	"gator/web"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

/*
TODO
Bugs:

User selector disappears after cookie redirect on page return
Pagination offset lost after add/unsubscribe redirect — should preserve ?offset= in redirect URL
BBC RSS feed (and potentially others) have malformed XML — consider gofeed library for more lenient parsing

Features:

Split into two pages: /feeds for subscription management, /posts for reading
Feed discovery page — show all feeds in db, allow subscribing to existing feeds without re-adding URL
POST /feeds/subscribe route — complement to existing /feeds/unsubscribe
Read/unread tracking — users_posts junction table, change post styling rather than hiding
Stale post archival — delete posts older than 180 days via background job
  - Make it so that in the environment file the period is configurable and whether or not deletion is just archival.

# Stale user cleanup — soft delete then hard delete after 90 days
CSS make it look perty

Infrastructure:

Containerisation with Docker and docker-compose
OIDC authentication via Authelia — replaces current manual user seeding
*/
func main() {
	godotenv.Load()

	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Open database and then check if connection can be established
	db, err := sql.Open(config.DBDriver, config.DBURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err = database.WaitForDB(config.DBConnWait, config.DBConnAttempts, db); err != nil {
		log.Fatalf("could not connect to database after 10 * %q: %v", config.DBConnWait, err)
	}

	// Ensure DB is up-to-date
	if err := database.RunMigrations(config.MigrationDir, config.DBDriver, db); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	queries := database.New(db)
	feedService := feedservice.NewService(queries, config.HTTPClient, config.MaxConcurrency)

	var wg sync.WaitGroup

	// Start background synchronization
	wg.Add(1)
	go func() {
		defer wg.Done()
		feedservice.Start(ctx, feedService)
	}()

	// Start web server
	srv, err := web.NewServer(queries, config.TemplateDir, config.ServerPort)
	if err != nil {
		log.Fatalf("Failed to create new server: %v", err)
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server crash: %v", err)
		}
	}()

	log.Printf("Server up and running on port: %s", config.ServerPort)
	// Block until we're done
	<-ctx.Done()
	fmt.Println("Shutting down...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)

	wg.Wait()
	fmt.Println("Gator exited cleanly")
}
