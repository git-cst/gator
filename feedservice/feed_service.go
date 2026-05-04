package feedservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gator/config"
	"gator/database"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/mmcdole/gofeed"
)

type FeedService struct {
	queries        *database.Queries
	parser         *gofeed.Parser
	maxConcurrency uint8
}

type parsedFeedResult struct {
	url  string
	feed *gofeed.Feed
}

const pgUniqueViolation = "23505"

func NewService(queries *database.Queries, httpConfig *config.HTTPConfig, serviceConfig *config.ServiceConfig) *FeedService {
	parser := gofeed.NewParser()
	parser.Client = httpConfig.HTTPClient
	parser.UserAgent = "gator0.1/git-cst"

	return &FeedService{
		queries:        queries,
		parser:         parser,
		maxConcurrency: serviceConfig.MaxConcurrency,
	}
}

func (s *FeedService) GetDistinctFeeds(ctx context.Context) ([]database.GetDistinctFeedsRow, error) {
	return s.queries.GetDistinctFeeds(ctx)
}

func (s *FeedService) StorePosts(ctx context.Context, feed *gofeed.Feed, feedURL string) error {
	lookupURL := feed.FeedLink
	if lookupURL == "" {
		lookupURL = feedURL
	}

	storedFeedRow, err := s.queries.GetFeedByUrl(ctx, lookupURL)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no feed with url %q exists: %w", lookupURL, err)
	} else if err != nil {
		return err
	}

	for _, post := range feed.Items {
		var descriptionVal string
		if post.Content != "" {
			descriptionVal = post.Content
		} else {
			descriptionVal = post.Description
		}

		postDescription := sql.NullString{
			String: descriptionVal,
			Valid:  descriptionVal != "",
		}

		UUID, err := uuid.NewV7()
		if err != nil {
			log.Printf("Could not create new UUID for post %v. Error: %v", post, err)
			continue
		}

		var publishedAt time.Time
		if post.PublishedParsed != nil {
			publishedAt = *post.PublishedParsed
		}

		_, err = s.queries.CreatePost(ctx, database.CreatePostParams{
			ID:          UUID,
			FeedID:      storedFeedRow.Feed.ID,
			Title:       post.Title,
			Description: postDescription,
			Url:         post.Link,
			PublishedAt: publishedAt,
			Now:         time.Now(),
		})
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == pgUniqueViolation {
			// OK that there is a duplicate post, silently fail
		} else if err != nil {
			log.Printf("feed post not able to be inserted: %q at link: %v\nReason: %v", post.Title, post.Link, err)
		}
	}

	return nil
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
	feedRows, err := service.GetDistinctFeeds(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("no feeds retrieved from database: %v", err)
		return
	} else if err != nil {
		log.Printf("unexpected error whilst retrieving feeds: %v", err)
		return
	}

	sem := make(chan struct{}, service.maxConcurrency)     // buffer the channel blocking if max concurrency reached
	results := make(chan *parsedFeedResult, len(feedRows)) // buffer the results channel so we can always send parsed results into the channel without blocking

	var wg sync.WaitGroup

	for _, row := range feedRows {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }() // this releases on exit for any reason so we don't have a semaphore slot leak (e.g. the semaphore slot is always taken up)

			parsedFeed, err := service.parser.ParseURLWithContext(url, ctx)
			if err != nil {
				log.Printf("failed to fetch %s: %v", url, err)
				return
			}

			parsedFeedResult := parsedFeedResult{
				feed: parsedFeed,
				url:  url,
			}

			results <- &parsedFeedResult
		}(row.Feed.Url)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for parsed := range results {
		if err := service.StorePosts(ctx, parsed.feed, parsed.url); err != nil {
			log.Printf("failed to store posts: %v", err)
		}
	}
}
