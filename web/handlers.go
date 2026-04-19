package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
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
	ID         uuid.UUID
	Title      string
	URL        string
	Subscribed bool
}

type postItem struct {
	ID          uuid.UUID
	Title       string
	Description string
	URL         string
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

type navigationContext struct {
	CurrOffset  int32
	PrevOffset  int32
	NextOffset  int32
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

func (s *Server) handleGetFeeds(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var statusCode int
	var currUserFeeds []*feedItem
	var errMsg string
	templateName := getTemplateName(r, "feeds.html", "feedContent")

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

	currUserFeeds, err = s.getCurrUserFeeds(ctx, userCtx.CurrUser.ID)
	if err != nil {
		switch {
		case errors.Is(err, errNoFeeds):
			statusCode = http.StatusNoContent
		case errors.Is(err, errInternalFeeds):
			statusCode = http.StatusInternalServerError
			errMsg = "internal server error"
		}
	}

	prevOffset, currOffset, nextOffset := s.resolveOffsets(r)
	hasNextPage := len(currUserFeeds) == 50

	s.respondWithHTML(w, templateName, feedPageData{
		userContext: userCtx,
		navigationContext: navigationContext{
			PrevOffset:  int32(prevOffset),
			CurrOffset:  int32(currOffset),
			NextOffset:  int32(nextOffset),
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

	createFeedParams := database.CreateFeedParams{
		ID:          uuid.New(),
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

	_, currOffset, _ := s.resolveOffsets(r)

	redirectURL := fmt.Sprintf("/feeds?user_id=%s&offset=%s", feedsUsersRow.UserID.String(), strconv.Itoa(currOffset))
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
	feedTitle := r.FormValue("feed_title")
	feedURL := r.FormValue("feed_url")

	currUserUUID, _, err := s.resolveCurrentUser(r)
	if err != nil {
		// Can't parse UUID so we exit early
		statusCode = http.StatusBadRequest
		errMsg := "invalid user"
		log.Printf("Failed to parse string to UUID whilst unsubbing from %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	feed, err := s.queries.GetFeedByUrl(ctx, feedURL)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusBadRequest
		errMsg := fmt.Sprintf("Feed: %s (%s) not found.", feedTitle, feedURL)
		log.Printf("Failed to retrieve %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst deleting %s (%s)", feedTitle, feedURL)
		log.Printf("Failed to retrieve %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	unsubParams := database.DeleteFeedForUserParams{
		FeedID: feed.ID,
		UserID: currUserUUID.UUID,
	}

	_, err = s.queries.DeleteFeedForUser(ctx, unsubParams)
	if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst deleting %s (%s)", feedTitle, feedURL)
		log.Printf("Failed to retrieve %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	_, currOffsetInt, _ := s.resolveOffsets(r)

	log.Printf("Successfully unsubscribed user %s from feed: %s (%s)", currUserUUID.UUID.String(), feedTitle, feedURL)

	if r.Header.Get("HX-Request") == "true" {
		s.respondWithHTML(w, "feedItem", feedItem{ID: feed.ID, Title: feed.Title, URL: feed.Url, Subscribed: false}, http.StatusOK)
		return
	}

	redirectURL := fmt.Sprintf("/feeds?user_id=%s&offset=%s", currUserUUID.UUID.String(), strconv.Itoa(currOffsetInt))
	http.Redirect(w, r, redirectURL, statusCode)
}

func (s *Server) handleSubscribeUserToFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer r.Body.Close()

	var statusCode int
	statusCode = http.StatusSeeOther
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	feedTitle := r.FormValue("feed_title")
	feedURL := r.FormValue("feed_url")

	currUserUUID, _, err := s.resolveCurrentUser(r)
	if err != nil {
		// Can't parse UUID so we exit early
		statusCode = http.StatusBadRequest
		errMsg := "invalid user"
		log.Printf("Failed to parse string to UUID whilst unsubbing from %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	feed, err := s.queries.GetFeedByUrl(ctx, feedURL)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusBadRequest
		errMsg := fmt.Sprintf("Feed: %s (%s) not found.", feedTitle, feedURL)
		log.Printf("Failed to retrieve %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst subscribing to %s (%s)", feedTitle, feedURL)
		log.Printf("Failed to retrieve %s (%s), error: %v", feedTitle, feedURL, err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	_, err = s.subscribeUserToFeed(ctx, currUserUUID.UUID, feed.ID)
	if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Unexpected error whilst subscribing to %s (%s)", feedTitle, feedURL)
		log.Printf("Failed to add %s (%s) for user (%s), error: %v", feedTitle, feedURL, currUserUUID.UUID.String(), err)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	log.Printf("Successfully subscribed user (%s) to %s (%s)", currUserUUID.UUID.String(), feedTitle, feedURL)

	_, currOffsetInt, _ := s.resolveOffsets(r)

	if r.Header.Get("HX-Request") == "true" {
		s.respondWithHTML(w, "feedItem", feedItem{ID: feed.ID, Title: feed.Title, URL: feed.Url, Subscribed: true}, http.StatusOK)
		return
	}

	redirectURL := fmt.Sprintf("/feeds?user_id=%s&offset=%s", currUserUUID.UUID.String(), strconv.Itoa(currOffsetInt))
	http.Redirect(w, r, redirectURL, statusCode)
}

func (s *Server) subscribeUserToFeed(ctx context.Context, userID uuid.UUID, feedID uuid.UUID) (database.FeedsUser, error) {
	addFeedParams := database.AddFeedForUserParams{
		ID:     uuid.New(),
		FeedID: feedID,
		UserID: userID,
	}
	feedUserRow, err := s.queries.AddFeedForUser(ctx, addFeedParams)
	if err != nil {
		return database.FeedsUser{}, err
	}

	return feedUserRow, nil
}

func (s *Server) getCurrUserFeeds(ctx context.Context, currUser uuid.UUID) ([]*feedItem, error) {
	var feeds []*feedItem

	feedRows, err := s.queries.GetDistinctFeedsForUser(ctx, currUser)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoFeeds
	} else if err != nil {
		return nil, errInternalFeeds
	} else {
		for _, row := range feedRows {
			feeds = append(feeds, toFeedItem(row))
		}
	}

	return feeds, nil
}

func (s *Server) handleGetPosts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var statusCode int
	var currUserPosts []*postItem
	var availableFeeds []*feedItem
	var errMsg string
	templateName := getTemplateName(r, "posts.html", "postContent")

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

	prevOffset, currOffset, nextOffset := s.resolveOffsets(r)

	params := database.GetPostsForUserParams{
		UserID: userCtx.CurrUser.ID,
		Offset: int32(currOffset),
	}
	userPostRows, err := s.queries.GetPostsForUser(ctx, params)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusNoContent
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg = "internal server error"
	} else {
		statusCode = http.StatusOK
		for _, row := range userPostRows {
			currUserPosts = append(currUserPosts, toPostItem(row))
		}
	}

	availableFeeds, err = s.getCurrUserFeeds(ctx, userCtx.CurrUser.ID)
	if err != nil {
		switch {
		case errors.Is(err, errNoFeeds):
			statusCode = http.StatusNoContent
		case errors.Is(err, errInternalFeeds):
			statusCode = http.StatusInternalServerError
			errMsg = "internal server error"
		}
	}

	hasNextPage := len(currUserPosts) == 50
	s.respondWithHTML(w, templateName, postPageData{
		userContext: userCtx,
		navigationContext: navigationContext{
			PrevOffset:  int32(prevOffset),
			CurrOffset:  int32(currOffset),
			NextOffset:  int32(nextOffset),
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

	// We always setup the params as if we are inserting a new row (e.g. reading for first time).
	// The query handles if there already is a row (e.g. we've already read it) and flips the bool.
	markReadParams := database.TogglePostReadStatusParams{
		ID:        uuid.New(),
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
		IsRead:      updatedPost.IsRead.Bool,
		PublishedAt: updatedPost.PublishedAt.Format("02-01-2006 15:04"),
	}, http.StatusOK)
}

func (s *Server) resolveOffsets(r *http.Request) (prevOffset int, currOffset int, nextOffset int) {
	var offsetStr string
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		offsetStr = r.FormValue("offset")
	} else {
		offsetStr = r.URL.Query().Get("offset")
	}

	offsetInt, err := strconv.Atoi(offsetStr)
	if err != nil {
		// We just default to 0 since the reasons for Atoi failing are:
		// 1) There is no offset passed with the query -> first page
		// 2) Invalid offset passed -> go to first page again
		offsetInt = 0
	}

	currOffset = offsetInt
	prevOffset = currOffset - 50
	nextOffset = currOffset + 50

	if prevOffset < 0 {
		prevOffset = 0
	}

	return prevOffset, currOffset, nextOffset
}

func (s *Server) getUserContext(r *http.Request, w http.ResponseWriter) (userData userContext, fromCookie bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var users []*userItem

	usersRows, err := s.queries.GetUsers(ctx)
	if errors.Is(err, sql.ErrNoRows) {
	} else if err != nil {
		return userContext{}, false, errNoUser
	} else {
		for _, row := range usersRows {
			users = append(users, toUserItem(row))
		}
	}

	userUUID, fromCookie, err := s.resolveCurrentUser(r)
	if err != nil {
		// Can't parse UUID so we exit early
		return userContext{Users: users, CurrUser: nil}, false, errInvalidUser
	}

	// Exit early as we don't have a user and don't need to request the rest of the data
	if !userUUID.Valid {
		http.SetCookie(w, &http.Cookie{
			Name:   "user_id",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})

		return userContext{Users: users, CurrUser: nil}, false, nil
	}

	currUser, err := s.queries.GetUserById(ctx, userUUID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return userContext{Users: users, CurrUser: nil}, false, errInvalidUser
	} else if err != nil {
		return userContext{Users: users, CurrUser: nil}, false, errInternalUser
	}

	http.SetCookie(w, &http.Cookie{
		Name:  "user_id",
		Value: currUser.ID.String(),
		Path:  "/",
	})

	if fromCookie {
		return userContext{Users: users, CurrUser: toUserItem(currUser)}, true, nil
	}

	return userContext{Users: users, CurrUser: toUserItem(currUser)}, false, nil
}

func (s *Server) resolveCurrentUser(r *http.Request) (userID uuid.NullUUID, fromCookie bool, err error) {
	var userStr string

	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		userStr = r.FormValue("user_id")
	} else {
		userStr = r.URL.Query().Get("user_id")
	}

	// user explicitly deselected the user - skip checking cookie
	_, keyPresent := r.URL.Query()["user_id"]
	if userStr == "" && keyPresent {
		return uuid.NullUUID{Valid: false}, false, nil
	}

	var userCookie *http.Cookie
	if userStr == "" {
		userCookie, _ = r.Cookie("user_id")
	}

	if userStr == "" && userCookie == nil {
		// nothings valid
		return uuid.NullUUID{Valid: false}, fromCookie, nil
	}

	if userCookie != nil && userCookie.Value != "" {
		// cookie is valid we can use this
		userStr = userCookie.Value
		fromCookie = true
	}

	parsedUUID, err := uuid.Parse(userStr)
	if err != nil {
		return uuid.NullUUID{Valid: false}, fromCookie, errInvalidUser
	}

	return uuid.NullUUID{
		UUID:  parsedUUID,
		Valid: true,
	}, fromCookie, nil
}

func (s *Server) respondWithHTML(w http.ResponseWriter, templateName string, responseData any, statusCode int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := s.template.ExecuteTemplate(w, templateName, responseData); err != nil {
		log.Printf("template execution failed: %v", err)
		fmt.Fprintf(w, "rendering error")
	}
}

func respondWithJSON(w http.ResponseWriter, responseData any, statusCode int) {
	data, err := json.Marshal(responseData)
	if err != nil {
		statusCode = http.StatusInternalServerError
		http.Error(w, "failed to marshal json on health check", statusCode)
		log.Print("error marshalling response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, err = w.Write(data)
	if err != nil {
		log.Print("error writing data")
	}
}

func getTemplateName(r *http.Request, fullPage string, partialPage string) string {
	if r.Header.Get("HX-Request") == "true" {
		return partialPage
	}
	return fullPage
}
