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
<<<<<<< HEAD
- User selector disappears after cookie redirect on page return.
- Malformed XML (BBC, etc.): Replace standard xml parser with gofeed for lenient, cross-spec parsing.
=======

BBC RSS feed (and potentially others) have malformed XML — consider gofeed library for more lenient parsing
>>>>>>> 6337831 (Add favicon)

Features:
Post card improvements:
 - Feed filtering on posts page (filter posts by specific feed).
 - Bookmarking (requires is_bookmarked column on posts_users).
 - Archiving (requires is_archived column on posts_users).
 - Rich Previews: Implement og:image/twitter:image extraction via html.Tokenizer.
 
Stale post archival:
 - Delete posts/files older than 180 days via background job.
 - Configurable retention period and "hard delete vs. move to archive" toggle in .env.

# Stale user cleanup — soft delete then hard delete after 90 days.

Infrastructure:
- OIDC authentication via Authelia: Replaces manual user seeding.
- NAS Storage Interface: Implement a file handler to store full article content as .md/txt on NAS.
- pgvector Integration: Add vector extension to Postgres for embedding storage.
*/

/*
NOTE
**Future Enhancements**
*Observability & Health*
Prometheus Metrics:
- Implement /metrics using prometheus/client_golang.
- Track: Fetch success/failure, DB latency, active user counts, and LLM processing queue depth.

Blackbox Monitoring:
- /health endpoint for uptime and latency checks via Prometheus Blackbox Exporter.

**Semantic Intelligence (The "Daily Brief")**
*Content Extraction:*
- Use go-readability (readeck/go-readability) to strip "noise" (ads/nav) before storage and vectorization.
*LLM Integration:*
- Background worker to pass cleaned text to local LLM (Ollama/Qwen wrapper).
*Semantic Summarization:*
- Generate a "Daily Briefing" markdown file summarizing high-priority news.
*Smart Categorization & RAG:*
- Use pgvector for semantic clustering ("More like this").
- Implement RAG (Retrieval-Augmented Generation) to allow natural language querying of the NAS-stored archive.

**User Experience & Scalability**
Live-ish Refresh:
- Implement HTMX "Soft Refresh" to poll for new items without full page reloads.
Hybrid Storage Strategy:
- Keep DB lean: Store only metadata and embeddings in Postgres.
- Store high-volume raw text on NAS/Filesystem; reference via path in DB.
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
