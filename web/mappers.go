package web

import "gator/database"

func toFeedItem(row database.GetDistinctFeedsForUserRow) *feedItem {
	return &feedItem{
		ID:         row.ID,
		Title:      row.Title,
		URL:        row.Url,
		Subscribed: row.UserID.Valid,
	}
}

func toPostItem(row database.GetPostsForUserRow) *postItem {
	var description string
	if row.Description.Valid {
		description = row.Description.String
	}

	if row.IsRead.Valid {
		isRead = row.IsRead.Bool
	}

	return &postItem{
		ID:          row.ID,
		Title:       row.Title,
		Description: description,
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
