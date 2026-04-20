package web

import (
	"regexp"

	"gator/database"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTML(s string) string {
	return htmlTagRegex.ReplaceAllString(s, "")
}

func toFeedItem(row database.GetDistinctFeedsForUserRow) *feedItem {
	var description string
	if row.Description.Valid {
		description = row.Description.String
	}

	return &feedItem{
		ID:          row.ID,
		Title:       row.Title,
		URL:         row.Url,
		Description: stripHTML(description),
		Subscribed:  row.UserID.Valid,
	}
}

func toPostItem(row database.GetPostsForUserRow) *postItem {
	var description string
	if row.Description.Valid {
		description = row.Description.String
	}

	return &postItem{
		ID:          row.ID,
		Title:       row.Title,
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
