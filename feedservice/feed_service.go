package feedservice

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"gator/database"
)

type FeedService struct {
	queries *database.Queries
}

func NewService(queries *database.Queries) *FeedService {
	return &FeedService{
		queries: queries,
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
	// Get the number of feeds there are.
	//	Fan out with go routines
	//	Read from channels as we go and then push the data to the db
	//	Wait on all go routines to complete
	//	Kill
}
