package web

import (
	"database/sql"
	"regexp"

	"gator/database"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTML(s string) string {
	return htmlTagRegex.ReplaceAllString(s, "")
}

func toFeedItem(row database.Feed, subscribed bool) *feedItem {
	var description string
	if row.Description.Valid {
		description = row.Description.String
	}

	return &feedItem{
		ID:          row.ID,
		Title:       row.Title,
		URL:         row.Url,
		Description: stripHTML(description),
		Subscribed:  subscribed,
	}
}

func toPostItem(row database.Post, feedTitle sql.NullString, feedURL sql.NullString, isRead sql.NullBool, isBookmarked sql.NullBool, isArchived sql.NullBool) *postItem {
	var description string
	if row.Description.Valid {
		description = row.Description.String
	}

	var postFeedTitle string
	if feedTitle.Valid {
		postFeedTitle = feedTitle.String
	}

	var postFeedURL string
	if feedURL.Valid {
		postFeedURL = feedURL.String
	}

	return &postItem{
		ID:           row.ID,
		Title:        row.Title,
		SourceTitle:  postFeedTitle,
		SourceURL:    postFeedURL,
		Description:  stripHTML(description),
		URL:          row.Url,
		IsRead:       isRead.Valid && isRead.Bool,
		IsBookmarked: isBookmarked.Valid && isBookmarked.Bool,
		IsArchived:   isArchived.Valid && isArchived.Bool,
		PublishedAt:  row.PublishedAt.Format("02-01-2006 15:04"),
	}
}

func toUserItem(row database.User) *userItem {
	return &userItem{
		ID:       row.ID,
		Username: row.Name,
	}
}
