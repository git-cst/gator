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

/*
NOTE
**Future Enhancements**
*Observability & Health*
Prometheus Metrics:
Implement a /metrics endpoint using prometheus/client_golang to track:
- Feed fetch success/failure rates.
- Database query latency.
- Number of active users and total post counts.

Blackbox Monitoring:
Configure a /health endpoint for uptime checks and latency monitoring via Prometheus Blackbox Exporter.

**Semantic Intelligence (The "Daily Brief")**
*LLM Integration:*
- Develop a routine to pass daily retrieved posts to a local LLM (via a Claude/Ollama harness).
*Semantic Summarization:*
- Generate a single, readable "Daily Briefing" markdown file that summarizes high-priority news across all feeds.
*Smart Categorization:*
- Use pgvector to semantically cluster similar posts, allowing for "More like this" features without manual tagging.

*User Experience & Scalability*
Live-ish Refresh:
- Implement a "Soft Refresh" using HTMX every polling to check for new feed items without a manual page reload.
Hybrid Storage:
- Maybe move feed content to a filesystem-based cache while keeping metadata in Postgres to keep the DB size lean (throw the data into the NAS?).
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
	dbConfig := config.DBConfig
	db, err := sql.Open(dbConfig.DBDriver, dbConfig.DBURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err = database.WaitForDB(dbConfig, db); err != nil {
		log.Fatalf("could not connect to database after 10 * %q: %v", dbConfig.DBConnWait, err)
	}

	// Ensure DB is up-to-date
	if err := database.RunMigrations(dbConfig, db); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	queries := database.New(db)
	feedService := feedservice.NewService(queries, config.HTTPConfig, config.ServiceConfig)

	var wg sync.WaitGroup

	// Start background synchronization
	wg.Go(func() {
		defer wg.Done()
		feedservice.Start(ctx, feedService)
	})

	// Start web server
	srv, err := web.NewServer(queries, config.ServiceConfig)
	if err != nil {
		log.Fatalf("Failed to create new server: %v", err)
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server crash: %v", err)
		}
	}()

	log.Printf("Server up and running on port: %s", config.ServiceConfig.ServerPort)
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
