package web

import "gator/database"

func toFeedItem(row database.GetUserFeedsRow) *feedItem {
	return &feedItem{
		ID:    row.ID.UUID,
		Title: row.Title.String,
		URL:   row.Url.String,
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
		Description: description,
		URL:         row.Url,
		PublishedAt: row.PublishedAt,
	}
}

func toUserItem(row database.User) *userItem {
	return &userItem{
		ID:       row.ID,
		Username: row.Name,
	}
}
