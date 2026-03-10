package feedservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"gator/database"
)

type FeedService struct {
	queries        *database.Queries
	httpClient     *http.Client
	maxConcurrency uint8
}

func NewService(queries *database.Queries, httpClient *http.Client, maxConcurrency uint8) *FeedService {
	return &FeedService{
		queries:        queries,
		httpClient:     httpClient,
		maxConcurrency: maxConcurrency,
	}
}

func (s *FeedService) GetDistinctFeeds(ctx context.Context) ([]database.GetDistinctFeedsRow, error) {
	return s.queries.GetDistinctFeeds(ctx)
}

func (s *FeedService) StorePosts(ctx context.Context, feed RSSFeed) error {
	storedFeed, err := s.queries.FetchFeedByUrl(ctx, feed.Channel.Link)
	if err != nil {
		return err
	}

	for _, post := range feed.Channel.Items {
		postDescription := sql.NullString{
			String: post.Description,
			Valid:  post.Description != "",
		}

		publishedAt, err := parseTimeString(post.PubDate)
		if err != nil {
			log.Printf("Unknown time format: %v, err: %v", post.PubDate, err)
		}

		_, err = s.queries.CreatePost(ctx, database.CreatePostParams{
			FeedID:      storedFeed.ID,
			Title:       post.Title,
			Description: postDescription,
			Url:         post.Link,
			PublishedAt: publishedAt,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
		if err != nil {
			log.Printf("feed post not able to be inserted: %q at link: %v", post.Title, post.Link)
		}
	}

	return nil
}

func parseTimeString(timeString string) (time.Time, error) {
	formats := []string{
		"Mon, 02 Jan 2006 15:04:05 -0700", // RFC1123 with timezone
		"Mon, 02 Jan 2006 15:04:05 MST",   // RFC1123 with timezone abbreviation
		"2006-01-02T15:04:05-07:00",       // ISO8601/RFC3339
		"2006-01-02T15:04:05Z",            // ISO8601/RFC3339 UTC
		"2006-01-02 15:04:05 -0700",       // Another common format
		"02 Jan 2006 15:04:05 -0700",      // Another variation
	}

	var firstErr error
	for _, format := range formats {
		publishedAt, err := time.Parse(format, timeString)
		if err == nil {
			return publishedAt, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	// If we got here, none of the formats worked
	return time.Time{}, fmt.Errorf("could not parse time '%s': %v", timeString, firstErr)
}

func Start(ctx context.Context, service *FeedService) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	runSync(ctx, service)

	for {
		select {
		case <-ticker.C:
			runSync(ctx, service)
		case <-ctx.Done():
			return
		}
	}
}

func runSync(ctx context.Context, service *FeedService) {
	feeds, err := service.GetDistinctFeeds(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("no feeds retrieved from database: %v", err)
		return
	} else if err != nil {
		log.Printf("unexpected error whilst retrieving feeds: %v", err)
		return
	}

	sem := make(chan struct{}, service.maxConcurrency)
	results := make(chan RSSFeed, len(feeds))

	var wg sync.WaitGroup

	for _, feed := range feeds {
		wg.Add(1)
		go func(url sql.NullString) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }() // this releases on exit for any reason so we don't have a leaked goroutine

			parsed, err := fetchAndParse(ctx, url)
			if err != nil {
				log.Printf("failed to fetch %s: %v", url, err)
				return
			}
			results <- parsed
		}(feed.Url)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for parsed := range results {
		if err := service.StorePosts(ctx, parsed); err != nil {
			log.Printf("failed to store posts: %v", err)
		}
	}
}
