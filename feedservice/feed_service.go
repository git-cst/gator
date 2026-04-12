package feedservice

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"gator/database"

	"github.com/google/uuid"
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
	storedFeed, err := s.queries.GetFeedByUrl(ctx, feed.URL)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no feed with url %q exists: %w", feed.Channel.Link, err)
	} else if err != nil {
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
			ID:          uuid.New(),
			FeedID:      storedFeed.ID,
			Title:       post.Title,
			Description: postDescription,
			Url:         post.Link,
			PublishedAt: publishedAt,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
		if err != nil {
			log.Printf("feed post not able to be inserted: %q at link: %v\nReason: %v", post.Title, post.Link, err)
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

	sem := make(chan struct{}, service.maxConcurrency) // buffer the channel blocking if max concurrency reached
	results := make(chan *RSSFeed, len(feeds))         // buffer the results channel so we can always send parsed results into the channel without blocking

	var wg sync.WaitGroup

	for _, feed := range feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }() // this releases on exit for any reason so we don't have a semaphore slot leak (e.g. the semaphore slot is always taken up)

			parsed, err := fetchAndParse(ctx, service.httpClient, url)
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
		if err := service.StorePosts(ctx, *parsed); err != nil {
			log.Printf("failed to store posts: %v", err)
		}
	}
}

func fetchAndParse(ctx context.Context, httpClient *http.Client, fetchURL string) (*RSSFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", fetchURL, err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", fetchURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response for %s: %w", fetchURL, err)
	}

	var feed RSSFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("decoding feed %s: %w", fetchURL, err)
	}

	feed.Channel.Title = html.UnescapeString(feed.Channel.Title)
	feed.Channel.Description = html.UnescapeString(feed.Channel.Description)

	for i := range feed.Channel.Items {
		feed.Channel.Items[i].Title = html.UnescapeString(feed.Channel.Items[i].Title)
		feed.Channel.Items[i].Description = html.UnescapeString(feed.Channel.Items[i].Description)
	}

	feed.URL = fetchURL

	return &feed, nil
}
