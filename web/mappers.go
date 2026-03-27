package web

import "gator/database"

func toFeedItem(row database.GetUserFeedsRow) *feedItem {
	return nil
}

func toPostItem(row database.GetPostsForUserRow) *postItem {
	return nil
}

func toUserItem(row database.User) *userItem {
	return nil
}
