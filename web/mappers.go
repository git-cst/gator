package web

import (
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

func toPostItem(row database.GetPostsForUserRow) *postItem {
	var description string
	if row.Description.Valid {
		description = row.Description.String
	}

	var feedTitle string
	if row.Feedtitle.Valid {
		feedTitle = row.Feedtitle.String
	}

	var feedURL string
	if row.Feedurl.Valid {
		feedURL = row.Feedurl.String
	}

	return &postItem{
		ID:          row.ID,
		Title:       row.Title,
		SourceTitle: feedTitle,
		SourceURL:   feedURL,
		Description: stripHTML(description),
		URL:         row.Url,
		IsRead:      row.IsRead.Valid && row.IsRead.Bool,
		PublishedAt: row.PublishedAt.Format("02-01-2006 15:04"),
	}
}

func toUserItem(row database.User) *userItem {
	return &userItem{
		ID:       row.ID,
		Username: row.Name,
	}
}
