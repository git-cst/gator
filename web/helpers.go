package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"gator/database"

	"github.com/google/uuid"
)

func paginate[T any](items []T) ([]T, bool) {
	if len(items) == 51 {
		return items[:50], true
	}
	return items, false
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

func (s *Server) resolveCursor(r *http.Request) uuid.NullUUID {
	var cursorStr string
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		cursorStr = r.FormValue("cursor")
	} else {
		cursorStr = r.URL.Query().Get("cursor")
	}

	cursorUUID, err := uuid.Parse(cursorStr)
	if err != nil {
		return uuid.NullUUID{Valid: false}
	}
	return uuid.NullUUID{Valid: true, UUID: cursorUUID}
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

func (s *Server) subscribeUserToFeed(ctx context.Context, userID uuid.UUID, feedID uuid.UUID) (database.AddFeedForUserRow, error) {
	rowID, err := uuid.NewV7()
	if err != nil {
		log.Printf("failed to subscribe user (%s) to feed (%s). error: %v", userID.String(), feedID.String(), err)
		return database.AddFeedForUserRow{}, err
	}
	addFeedParams := database.AddFeedForUserParams{
		ID:     rowID,
		FeedID: feedID,
		UserID: userID,
	}
	feedUserRow, err := s.queries.AddFeedForUser(ctx, addFeedParams)
	if err != nil {
		return database.AddFeedForUserRow{}, err
	}

	return feedUserRow, nil
}

func (s *Server) getCurrUserFeeds(ctx context.Context, currUser uuid.UUID) ([]*feedItem, bool, error) {
	var feeds []*feedItem
	var hasNextPage bool

	feedRows, err := s.queries.GetDistinctFeedsForUser(ctx, currUser)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, errNoFeeds
	} else if err != nil {
		return nil, false, errInternalFeeds
	} else {
		feedRows, hasNextPage = paginate(feedRows)
		for _, row := range feedRows {
			feeds = append(feeds, toFeedItem(row))
		}
	}

	return feeds, hasNextPage, nil
}
