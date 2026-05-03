package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"gator/database"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type errorResponse struct {
	ErrorMsg string `json:"error"`
}

type healthResponse struct {
	Healthy bool   `json:"healthy"`
	DB      string `json:"database_status"`
	Uptime  string `json:"uptime"`
}

type feedItem struct {
	ID          uuid.UUID
	Title       string
	URL         string
	Description string
	Subscribed  bool
}

type postItem struct {
	ID          uuid.UUID
	Title       string
	Description string
	URL         string
	SourceTitle string
	SourceURL   string
	IsRead      bool
	PublishedAt string
}

type userItem struct {
	ID       uuid.UUID
	Username string
}

type userContext struct {
	Users    []*userItem
	CurrUser *userItem
}

type feedCursor struct {
	ID    uuid.NullUUID `json:"row_id"`
	Valid bool
}

type postCursor struct {
	PublishedAt sql.NullTime  `json:"published_at"`
	ID          uuid.NullUUID `json:"row_id"`
	Valid       bool
}

type navigationContext struct {
	Cursor      string
	HasNextPage bool
}

type feedPageData struct {
	userContext
	navigationContext
	Feeds         []*feedItem
	UserSwitchURL string
	ErrorString   string
}

type postPageData struct {
	userContext
	navigationContext
	Feeds         []*feedItem
	Posts         []*postItem
	UserSwitchURL string
	ErrorString   string
}

type errorData struct {
	ErrorString string
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	healthy := true
	var dbStatus string
	var statusCode int
	dbHealth, err := s.queries.CheckHealth(ctx)
	if err != nil {
		statusCode = http.StatusInternalServerError
		healthy = false
		dbStatus = "database not available"
	} else if dbHealth != 1 {
		statusCode = http.StatusInternalServerError
		healthy = false
		dbStatus = "database returning unexpected values"
	} else {
		statusCode = http.StatusOK
		dbStatus = "ok"
	}

	response := healthResponse{
		Healthy: healthy,
		DB:      dbStatus,
		Uptime:  s.uptime(),
	}

	respondWithJSON(w, response, statusCode)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.respondWithHTML(w, "index.html", nil, http.StatusNotFound)
		return
	}
	s.respondWithHTML(w, "index.html", nil, http.StatusOK)
}

func (s *Server) handleGetFeeds(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var statusCode int
	var currUserFeeds []*feedItem
	var errMsg string
	var partialPage string
	cursor := s.resolveFeedCursor(r)
	if cursor.Valid {
		partialPage = "nextFeedContent"
	} else {
		partialPage = "feedContent"
	}

	templateName := getTemplateName(r, "feeds.html", partialPage)

	userCtx, fromCookie, err := s.getUserContext(r, w)
	if err != nil {
		switch {
		case errors.Is(err, errNoUser):
			s.respondWithHTML(w, templateName, feedPageData{userContext: userCtx}, http.StatusOK)
		case errors.Is(err, errInvalidUser):
			s.respondWithHTML(w, templateName, feedPageData{userContext: userCtx, ErrorString: "invalid user"}, http.StatusBadRequest)
		case errors.Is(err, errInternalUser):
			s.respondWithHTML(w, templateName, feedPageData{userContext: userCtx, ErrorString: "internal server error"}, http.StatusInternalServerError)
		}
		return
	}

	if fromCookie {
		statusCode = http.StatusSeeOther
		redirectURL := fmt.Sprintf("/feeds?user_id=%s", userCtx.CurrUser.ID.String())
		http.Redirect(w, r, redirectURL, statusCode)
		return
	}

	if userCtx.CurrUser == nil {
		s.respondWithHTML(w, templateName, feedPageData{userContext: userCtx}, http.StatusOK)
		return
	}

	currUserFeeds, hasNextPage, err := s.getCurrUserFeeds(ctx, userCtx.CurrUser.ID)
	if err != nil {
		switch {
		case errors.Is(err, errNoFeeds):
			statusCode = http.StatusNoContent
		case errors.Is(err, errInternalFeeds):
			statusCode = http.StatusInternalServerError
			errMsg = "internal server error"
		}
	}

	var nextCursor feedCursor
	var cursorString string
	if hasNextPage {
		nextCursor = feedCursor{
			ID: uuid.NullUUID{
				UUID:  currUserFeeds[50].ID,
				Valid: true,
			},
			Valid: true,
		}

		cursorString, err = nextCursor.encode()
		if err != nil {
			cursorString = ""
			statusCode = http.StatusInternalServerError
			errMsg = "internal server error"
			log.Printf("could not encode cursor whilst retrieving feeds")

		}
	} else {
		nextCursor = feedCursor{}
		cursorString = ""
	}

	s.respondWithHTML(w, templateName, feedPageData{
		userContext: userCtx,
		navigationContext: navigationContext{
			Cursor:      cursorString,
			HasNextPage: hasNextPage,
		},
		Feeds:         currUserFeeds,
		UserSwitchURL: "/feeds",
		ErrorString:   errMsg,
	}, http.StatusOK)
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer r.Body.Close()

	var statusCode int
	statusCode = http.StatusSeeOther

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	feedTitle := r.FormValue("feed_title")
	feedURL := r.FormValue("feed_url")
	formDescription := strings.TrimSpace(r.FormValue("feed-description"))
	feedDescription := sql.NullString{
		String: formDescription,
		Valid:  formDescription != "",
	}

	rowID, err := uuid.NewV7()
	if err != nil {
		statusCode = http.StatusInternalServerError
		log.Printf("failed to add feed %s (%s). error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, statusCode)
		return
	}

	createFeedParams := database.CreateFeedParams{
		ID:          rowID,
		Title:       feedTitle,
		Url:         feedURL,
		Description: feedDescription,
	}

	var feedID uuid.UUID
	feedRow, err := s.queries.CreateFeed(ctx, createFeedParams)
	if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
		feedRow, err := s.queries.GetFeedByUrl(ctx, feedURL)
		if err != nil {
			statusCode = http.StatusInternalServerError
			errMsg := fmt.Sprintf("Failed to retrieve information for %s (%s) due to unexpected error", feedTitle, feedURL)
			log.Printf("%s: %v", errMsg, err)
			s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
			return
		}

		feedID = feedRow.ID
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Failed to add feed %s (%s) due to unexpected error", feedTitle, feedURL)
		log.Printf("%s: %v", errMsg, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	} else {
		feedID = feedRow.ID
	}

	userCtx, _, err := s.getUserContext(r, w)
	if err != nil {
		switch {
		case errors.Is(err, errNoUser):
			s.respondWithHTML(w, "error", errorData{ErrorString: "no user provided"}, http.StatusBadRequest)
		case errors.Is(err, errInvalidUser):
			s.respondWithHTML(w, "error", errorData{ErrorString: "invalid user"}, http.StatusBadRequest)
		case errors.Is(err, errInternalUser):
			s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)
		}
		return
	}

	feedsUsersRow, err := s.subscribeUserToFeed(ctx, userCtx.CurrUser.ID, feedID)
	if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Failed to subscribe to %s (%s) due to unexpected error", feedTitle, feedURL)
		log.Printf("%s: %v", errMsg, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	cursor := s.resolveFeedCursor(r)
	var cursorString string
	if cursor.Valid {
		cursorString, err = cursor.encode()
		if err != nil {
			cursorString = ""
		}
	} else {
		cursorString = ""
	}

	redirectURL := fmt.Sprintf("/feeds?user_id=%s&cursor=%s", feedsUsersRow.UserID.String(), cursorString)
	http.Redirect(w, r, redirectURL, statusCode)
}

/*
handlerDeleteFeed performs a hard delete of a feed for all users.
Not currently registered as a route, but I might do so in the future to implement admin use?
*/
func (s *Server) handlerDeleteFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer r.Body.Close()

	var statusCode int
	statusCode = http.StatusSeeOther
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	feedTitle := r.FormValue("feed_title")
	feedURL := r.FormValue("feed_url")
	currUser := r.FormValue("user_id")
	feedID, err := s.queries.GetFeedByUrl(ctx, feedURL)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusBadRequest
		errMsg := fmt.Sprintf("Feed: %s (%s) not found.", feedTitle, feedURL)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst deleting %s (%s)", feedTitle, feedURL)
		log.Printf("Failed to retrieve %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	deletedFeed, err := s.queries.DeleteFeed(ctx, feedID.ID)
	if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst deleting %s (%s)", feedTitle, feedURL)
		log.Printf("Failed to retrieve %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	log.Printf("Successfully deleted feed: %s (%s)", deletedFeed.Title, deletedFeed.Url)
	redirectURL := fmt.Sprintf("/feeds?user_id=%s", currUser)
	http.Redirect(w, r, redirectURL, statusCode)
}

func (s *Server) handleUnsubscribeUserFromFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer r.Body.Close()

	var statusCode int
	statusCode = http.StatusSeeOther
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	feedIDstr := r.PathValue("id")
	feedID, err := uuid.Parse(feedIDstr)
	if err != nil {
		statusCode = http.StatusBadRequest
		errMsg := "invalid user"
		log.Printf("Failed to parse feed string (%s) to UUID whilst unsubbing, error: %v", feedIDstr, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	currUserUUID, _, err := s.resolveCurrentUser(r)
	if err != nil {
		// Can't parse UUID so we exit early
		statusCode = http.StatusBadRequest
		errMsg := "invalid user"
		log.Printf("Failed to parse string to UUID whilst unsubbing from %s, error: %v", feedIDstr, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	unsubParams := database.DeleteFeedForUserParams{
		FeedID: feedID,
		UserID: currUserUUID.UUID,
	}

	deletedFeed, err := s.queries.DeleteFeedForUser(ctx, unsubParams)
	if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst deleting %s", feedID)
		log.Printf("Failed to delete %s, error: %v", feedID, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	cursor := s.resolveFeedCursor(r)
	var cursorString string
	if cursor.Valid {
		cursorString, err = cursor.encode()
		if err != nil {
			cursorString = ""
		}
	} else {
		cursorString = ""
	}
	log.Printf("Successfully unsubscribed user %s from feed: %s (%s)", currUserUUID.UUID.String(), deletedFeed.Title, deletedFeed.Url)

	if r.Header.Get("HX-Request") == "true" {
		s.respondWithHTML(w, "feedItem", feedItem{
			ID:          deletedFeed.FeedID,
			Title:       deletedFeed.Title,
			Description: deletedFeed.Description.String,
			URL:         deletedFeed.Url,
			Subscribed:  false,
		}, http.StatusOK)
		return
	}

	redirectURL := fmt.Sprintf("/feeds?user_id=%s&cursor=%s", currUserUUID.UUID.String(), cursorString)
	http.Redirect(w, r, redirectURL, statusCode)
}

func (s *Server) handleSubscribeUserToFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer r.Body.Close()

	var statusCode int
	statusCode = http.StatusSeeOther
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	feedIDstr := r.PathValue("id")
	feedID, err := uuid.Parse(feedIDstr)
	if err != nil {
		statusCode = http.StatusBadRequest
		errMsg := "invalid user"
		log.Printf("Failed to parse feed string (%s) to UUID whilst unsubbing, error: %v", feedIDstr, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	currUserUUID, _, err := s.resolveCurrentUser(r)
	if err != nil {
		// Can't parse UUID so we exit early
		statusCode = http.StatusBadRequest
		errMsg := "invalid user"
		log.Printf("Failed to parse string to UUID whilst unsubbing from %s, error: %v", feedIDstr, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	subscribedFeedRow, err := s.subscribeUserToFeed(ctx, currUserUUID.UUID, feedID)
	if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst subscribing to %s", feedID)
		log.Printf("Failed to add %s for user (%s), error: %v", feedID, currUserUUID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}
	subscribedFeed := feedItem{
		ID:          subscribedFeedRow.FeedID,
		Title:       subscribedFeedRow.Title,
		Description: subscribedFeedRow.Description.String,
		URL:         subscribedFeedRow.Url,
		Subscribed:  true,
	}

	log.Printf("Successfully subscribed user (%s) to %s (%s)", currUserUUID.UUID.String(), subscribedFeed.Title, subscribedFeed.URL)

	cursor := s.resolveFeedCursor(r)
	var cursorString string
	if cursor.Valid {
		cursorString, err = cursor.encode()
		if err != nil {
			cursorString = ""
		}
	} else {
		cursorString = ""
	}

	if r.Header.Get("HX-Request") == "true" {
		s.respondWithHTML(w, "feedItem", subscribedFeed, http.StatusOK)
		return
	}

	redirectURL := fmt.Sprintf("/feeds?user_id=%s&cursor=%s", currUserUUID.UUID.String(), cursorString)
	http.Redirect(w, r, redirectURL, statusCode)
}

func (s *Server) handleGetPosts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var statusCode int
	var currUserPosts []*postItem
	var availableFeeds []*feedItem
	var errMsg string
	var hasNextPage bool
	var partialPage string

	cursor := s.resolvePostCursor(r)
	if cursor.Valid {
		partialPage = "nextPostContent"
	} else {
		partialPage = "postContent"
	}

	templateName := getTemplateName(r, "posts.html", partialPage)

	userCtx, fromCookie, err := s.getUserContext(r, w)
	if err != nil {
		switch {
		case errors.Is(err, errNoUser):
			s.respondWithHTML(w, templateName, postPageData{userContext: userCtx}, http.StatusOK)
		case errors.Is(err, errInvalidUser):
			s.respondWithHTML(w, templateName, postPageData{userContext: userCtx, ErrorString: "invalid user"}, http.StatusBadRequest)
		case errors.Is(err, errInternalUser):
			s.respondWithHTML(w, templateName, postPageData{userContext: userCtx, ErrorString: "internal server error"}, http.StatusInternalServerError)
		}
		return
	}

	if fromCookie {
		statusCode = http.StatusSeeOther
		redirectURL := fmt.Sprintf("/posts?user_id=%s", userCtx.CurrUser.ID.String())
		http.Redirect(w, r, redirectURL, statusCode)
		return
	}

	if userCtx.CurrUser == nil {
		s.respondWithHTML(w, templateName, postPageData{userContext: userCtx}, http.StatusOK)
		return
	}

	params := database.GetPostsForUserParams{
		UserID:     userCtx.CurrUser.ID,
		CursorID:   cursor.ID,
		CursorDate: cursor.PublishedAt,
	}
	userPostRows, err := s.queries.GetPostsForUser(ctx, params)
	var paginatedUserPosts []database.GetPostsForUserRow
	var nextCursor postCursor
	var cursorString string
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusNoContent
		nextCursor = postCursor{}
	} else if err != nil {
		log.Printf("GetPostsForUser error: %v", err)
		statusCode = http.StatusInternalServerError
		errMsg = "internal server error"
		nextCursor = postCursor{}
	} else {
		paginatedUserPosts, hasNextPage = paginate(userPostRows)
		statusCode = http.StatusOK
		for _, row := range paginatedUserPosts {
			currUserPosts = append(currUserPosts, toPostItem(row))
		}

		if hasNextPage {
			nextCursor = postCursor{
				ID: uuid.NullUUID{
					UUID:  userPostRows[50].ID,
					Valid: true,
				},
				PublishedAt: sql.NullTime{
					Time:  userPostRows[50].PublishedAt,
					Valid: true,
				},
				Valid: true,
			}

			cursorString, err = nextCursor.encode()
			if err != nil {
				statusCode = http.StatusInternalServerError
				errMsg = "internal server error"
				log.Printf("could not encode cursor whilst retrieving posts")
			}
		} else {
			nextCursor = postCursor{}
			cursorString = ""
		}
	}

	availableFeeds, _, err = s.getCurrUserFeeds(ctx, userCtx.CurrUser.ID)
	if err != nil {
		switch {
		case errors.Is(err, errNoFeeds):
			statusCode = http.StatusNoContent
		case errors.Is(err, errInternalFeeds):
			statusCode = http.StatusInternalServerError
			errMsg = "internal server error"
		}
	}

	s.respondWithHTML(w, templateName, postPageData{
		userContext: userCtx,
		navigationContext: navigationContext{
			Cursor:      cursorString,
			HasNextPage: hasNextPage,
		},
		Posts:         currUserPosts,
		Feeds:         availableFeeds,
		UserSwitchURL: "/posts",
		ErrorString:   errMsg,
	}, statusCode,
	)
}

func (s *Server) handleTogglePostReadStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	postIDStr := r.PathValue("id")
	if postIDStr == "" {
		log.Print("no post provided to be marked read")
		s.respondWithHTML(w, "error", errorData{ErrorString: "no post provided"}, http.StatusBadRequest)
		return
	}

	postID, err := uuid.Parse(postIDStr)
	if err != nil {
		log.Printf("failed to mark post (%s), could not resolve postID. error: %v", postIDStr, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "invalid post"}, http.StatusBadRequest)
		return
	}

	userID, _, err := s.resolveCurrentUser(r)
	if err != nil {
		if errors.Is(err, errInvalidUser) {
			log.Printf("failed to mark post (%s) as read, could not resolve user. error: %v", postIDStr, err)
			s.respondWithHTML(w, "error", errorData{ErrorString: "invalid user"}, http.StatusBadRequest)
			return
		}

		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)
		return
	}

	rowID, err := uuid.NewV7()
	if err != nil {
		log.Printf("failed to toggle post (%s) read status for user (%s). error: %v", postIDStr, userID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)

	}
	// We always setup the params as if we are inserting a new row (e.g. reading for first time).
	// The query handles if there already is a row (e.g. we've already read it) and flips the bool.
	markReadParams := database.TogglePostReadStatusParams{
		ID:        rowID,
		PostID:    postID,
		UserID:    userID.UUID,
		IsRead:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = s.queries.TogglePostReadStatus(ctx, markReadParams)
	if err != nil {
		log.Printf("failed to mark post (%s) as read for user (%s). error: %v", postIDStr, userID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)
		return
	}

	getPostParams := database.GetPostByIDParams{
		UserID: userID.UUID,
		ID:     postID,
	}
	updatedPost, err := s.queries.GetPostByID(ctx, getPostParams)
	if err != nil {
		log.Printf("failed to mark post (%s) as read for user (%s). error: %v", postIDStr, userID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)
		return
	}

	s.respondWithHTML(w, "postItem", postItem{
		ID:          updatedPost.ID,
		Title:       updatedPost.Title,
		URL:         updatedPost.Url,
		Description: stripHTML(updatedPost.Description.String),
		SourceTitle: updatedPost.Feedtitle.String,
		SourceURL:   updatedPost.Url,
		IsRead:      updatedPost.IsRead.Bool,
		PublishedAt: updatedPost.PublishedAt.Format("02-01-2006 15:04"),
	}, http.StatusOK)
}

func (s *Server) handleMarkPostRead(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	postIDStr := r.PathValue("id")
	if postIDStr == "" {
		log.Print("no post provided to be marked read")
		s.respondWithHTML(w, "error", errorData{ErrorString: "no post provided"}, http.StatusBadRequest)
		return
	}

	postID, err := uuid.Parse(postIDStr)
	if err != nil {
		log.Printf("failed to mark post (%s), could not resolve postID. error: %v", postIDStr, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "invalid post"}, http.StatusBadRequest)
		return
	}

	userID, _, err := s.resolveCurrentUser(r)
	if err != nil {
		if errors.Is(err, errInvalidUser) {
			log.Printf("failed to mark post (%s) as read, could not resolve user. error: %v", postIDStr, err)
			s.respondWithHTML(w, "error", errorData{ErrorString: "invalid user"}, http.StatusBadRequest)
			return
		}

		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)
		return
	}

	rowID, err := uuid.NewV7()
	if err != nil {
		log.Printf("failed to mark post (%s) as read for user (%s). error: %v", postIDStr, userID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)

	}
	markReadParams := database.MarkPostAsReadParams{
		ID:        rowID,
		PostID:    postID,
		UserID:    userID.UUID,
		IsRead:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = s.queries.MarkPostAsRead(ctx, markReadParams)
	if err != nil {
		log.Printf("failed to mark post (%s) as read for user (%s). error: %v", postIDStr, userID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)
		return
	}

	getPostParams := database.GetPostByIDParams{
		UserID: userID.UUID,
		ID:     postID,
	}
	updatedPost, err := s.queries.GetPostByID(ctx, getPostParams)
	if err != nil {
		log.Printf("failed to mark post (%s) as read for user (%s). error: %v", postIDStr, userID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: "internal server error"}, http.StatusInternalServerError)
		return
	}

	s.respondWithHTML(w, "postItem", postItem{
		ID:          updatedPost.ID,
		Title:       updatedPost.Title,
		URL:         updatedPost.Url,
		Description: stripHTML(updatedPost.Description.String),
		SourceTitle: updatedPost.Feedtitle.String,
		SourceURL:   updatedPost.Feedurl.String,
		IsRead:      updatedPost.IsRead.Bool,
		PublishedAt: updatedPost.PublishedAt.Format("02-01-2006 15:04"),
	}, http.StatusOK)
}
