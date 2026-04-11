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
	ID    uuid.UUID
	Title string
	URL   string
}

type postItem struct {
	ID          uuid.UUID
	Title       string
	Description string
	URL         string
	PublishedAt string
}

type userItem struct {
	ID       uuid.UUID
	Username string
}

type feedPageData struct {
	Users         []*userItem
	CurrUser      *userItem
	CurrUserFeeds []*feedItem
	CurrUserPosts []*postItem
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
	var users []*userItem
	var currUserFeeds []*feedItem
	var currUserPosts []*postItem
	var errMsg string
	templateName := getTemplateName(r, "feeds.html")

	usersRows, err := s.queries.GetUsers(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusNoContent
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg = "internal server error"
		s.respondWithHTML(w, templateName, feedPageData{
			Users:         users,
			CurrUser:      nil,
			CurrUserFeeds: currUserFeeds,
			CurrUserPosts: currUserPosts,
			ErrorString:   errMsg,
		}, statusCode)
		return
	} else {
		statusCode = http.StatusOK
		for _, row := range usersRows {
			users = append(users, toUserItem(row))
		}
	}

	userUUID, err := s.resolveCurrentUser(r)
	if err != nil {
		// Can't parse UUID so we exit early
		statusCode = http.StatusBadRequest
		errMsg = "invalid user"
		s.respondWithHTML(w, templateName, feedPageData{
			Users:         users,
			CurrUser:      nil,
			CurrUserFeeds: currUserFeeds,
			CurrUserPosts: currUserPosts,
			ErrorString:   errMsg,
		}, statusCode)
		return
	}

	// Exit early as we don't have a user and don't need to request the rest of the data
	if !userUUID.Valid {
		s.respondWithHTML(w, templateName, feedPageData{
			Users:         users,
			CurrUser:      nil,
			CurrUserFeeds: currUserFeeds,
			CurrUserPosts: currUserPosts,
			ErrorString:   errMsg,
		}, statusCode)
		return
	}

	currUser, err := s.queries.GetUserById(ctx, userUUID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusBadRequest
		errMsg = "user does not exist"
		s.respondWithHTML(w, templateName, feedPageData{
			Users:         users,
			CurrUser:      nil,
			CurrUserFeeds: currUserFeeds,
			CurrUserPosts: currUserPosts,
			ErrorString:   errMsg,
		}, statusCode)
		return
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg = "internal server error"
		s.respondWithHTML(w, templateName, feedPageData{
			Users:         users,
			CurrUser:      nil,
			CurrUserFeeds: currUserFeeds,
			CurrUserPosts: currUserPosts,
			ErrorString:   errMsg,
		}, statusCode)
		return
	}

	userFeedRows, err := s.queries.GetUserFeeds(ctx, currUser.ID)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusNoContent
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg = "internal server error"
	} else {
		statusCode = http.StatusOK
		for _, row := range userFeedRows {
			currUserFeeds = append(currUserFeeds, toFeedItem(row))
		}
	}

	offsetStr := r.URL.Query().Get("offset")
	offsetInt, err := strconv.Atoi(offsetStr)
	if err != nil {
		// We just default to 0 since the reasons for Atoi failing are:
		// 1) There is no offset passed with the query -> first page
		// 2) Invalid offset passed -> go to first page again
		offsetInt = 0
	}

	params := database.GetPostsForUserParams{
		UserID: currUser.ID,
		Offset: int32(offsetInt),
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

	s.respondWithHTML(w, templateName, feedPageData{
		Users:         users,
		CurrUser:      toUserItem(currUser),
		CurrUserFeeds: currUserFeeds,
		CurrUserPosts: currUserPosts,
		ErrorString:   errMsg,
	}, statusCode)
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer r.Body.Close()

	var statusCode int
	statusCode = http.StatusSeeOther

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	feedTitle := r.FormValue("feed-title")
	feedURL := r.FormValue("feed-url")
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
			log.Print(errMsg)
			s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
			return
		}

		feedID = feedRow.ID
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Failed to add feed %s (%s) due to unexpected error", feedTitle, feedURL)
		log.Print(errMsg)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	} else {
		feedID = feedRow.ID
	}

	feedsUsersRow, err := s.subscribeUserToFeed(ctx, r, feedID)
	if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg := fmt.Sprintf("Failed to subscribe to %s (%s) due to unexpected error", feedTitle, feedURL)
		log.Print(errMsg)
		s.respondWithHTML(w, "error", errorData{ErrorString: errMsg}, statusCode)
		return
	}

	redirectURL := fmt.Sprintf("/feeds?user_id=%s", feedsUsersRow.UserID.String())
	http.Redirect(w, r, redirectURL, statusCode)
}

/*
hadnlerDeleteFeed performs a hard delete of a feed for all users.
Not currently registered as a route, but I might do so in the future to implement admin use?
*/
func (s *Server) handlerDeleteFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer r.Body.Close()

	var statusCode int
	statusCode = http.StatusSeeOther
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	feedTitle := r.FormValue("feed-title")
	feedURL := r.FormValue("feed-url")
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
	feedTitle := r.FormValue("feed-title")
	feedURL := r.FormValue("feed-url")

	currUserUUID, err := s.resolveCurrentUser(r)
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

	log.Printf("Successfully unsubscribed user %s from feed: %s (%s)", currUserUUID.UUID.String(), feedTitle, feedURL)
	redirectURL := fmt.Sprintf("/feeds?user_id=%s", currUserUUID.UUID.String())
	http.Redirect(w, r, redirectURL, statusCode)
}

func (s *Server) subscribeUserToFeed(ctx context.Context, r *http.Request, feedID uuid.UUID) (database.FeedsUser, error) {
	userUUID, err := s.resolveCurrentUser(r)
	if err != nil {
		return database.FeedsUser{}, err
	}

	addFeedParams := database.AddFeedForUserParams{
		FeedID: feedID,
		UserID: userUUID.UUID,
	}
	feedUserRow, err := s.queries.AddFeedForUser(ctx, addFeedParams)
	if err != nil {
		return database.FeedsUser{}, err
	}

	return feedUserRow, nil
}

func (s *Server) resolveCurrentUser(r *http.Request) (uuid.NullUUID, error) {
	var userReq string
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		userReq = r.FormValue("user_id")
	} else {
		userReq = r.URL.Query().Get("user_id")
	}

	if userReq == "" {
		return uuid.NullUUID{Valid: false}, nil
	}

	parsedUUID, err := uuid.Parse(userReq)
	if err != nil {
		return uuid.NullUUID{Valid: false}, err
	}

	return uuid.NullUUID{
		UUID:  parsedUUID,
		Valid: true,
	}, nil
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

func getTemplateName(r *http.Request, fullPage string) string {
	if r.Header.Get("HX-Request") == "true" {
		return "content"
	}
	return fullPage
}
