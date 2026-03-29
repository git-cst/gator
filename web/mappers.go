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
	return &postItem{
		ID:          row.ID,
		Title:       row.Title,
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
