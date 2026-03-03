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

	_ "github.com/lib/pq"
)

/*
TODO
Write code that interacts with the database:

	1 - Initialize the db
	2 - Connect to the db
	3 - Setup schema
	4 - Write queries
		a - Store which users can interact ? User registration? Or integrate with Authelia OIDC?
		b - Store which feeds users want to pull from
		c - Store the posts that have been archived

Write code that handles synchronization

	1 - Read from the database which feeds are stored (distinct)
	2 - Fan out go funcs to pull from those rss feeds
		a - Wait group
		b - Maybe semaphore pattern (only so many go coroutines)?
	3 - Store results from channel in db
		a - Probably do so asynchronously.
		b - Will need to implement mutex locking (I think it's sync in golang)

Write code that handles cleanup

	1 - Stale posts (maybe archive just 180 days worth?)
	2 - Stale users (if a user has been soft deleted, then yeet after 90 days)

Write code that provides the data from the database to users as a static webpage

	1 - On start up provide the data via port forwarding ####.
	2 - Write the handlers for the links?
	3 - Add a simple timer for the next refresh?
	4 - Maybe write a handler to get the newest links?
	5 - Refresh on completion of database synchronization
*/
func main() {
	config, err := config.ReadConfig()
	if err != nil {
		fmt.Println("Error reading config: %v", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	db, err := sql.Open("postgres", config.DbURL)
	defer db.Close()

	var wg sync.WaitGroup

	// Start background synchronization
	go func() {
		defer wg.Done()
		sync.Start(ctx, feedService)
	}()

	// Start web server
	srv := web.NewServer(db)
	go func() {
		if err := srv.ListenAndServe(":8888"); err != http.ErrServerClosed {
			log.Fatalf("HTTP server crash: %v", err)
		}
	}()

	// Block until we're done
	<-ctx.Done()
	fmt.Println("Shutting down...")

	// Graceful shutdowns
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)

	wg.Wait()
	fmt.Println("Gator exited cleanly")
}
