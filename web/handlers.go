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
	PublishedAt time.Time
}

type userItem struct {
	ID       uuid.UUID
	Username string
}

type pageData struct {
	Users         []*userItem
	CurrUser      *userItem
	CurrUserFeeds []*feedItem
	CurrUserPosts []*postItem
	ErrorString   string
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
	templateName := getTemplateName(r, "feeds.html", "feeds_partial.html")

	usersRows, err := s.queries.GetUsers(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusNoContent
	} else if err != nil {
		statusCode = http.StatusInternalServerError
		errMsg = "internal server error"
		s.respondWithHTML(w, templateName, pageData{
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

	userReq := r.URL.Query().Get("user_id")
	// Exit early as we don't have a user and don't need to request the rest of the data
	if userReq == "" {
		s.respondWithHTML(w, templateName, pageData{
			Users:         users,
			CurrUser:      nil,
			CurrUserFeeds: currUserFeeds,
			CurrUserPosts: currUserPosts,
			ErrorString:   errMsg,
		}, statusCode)
		return
	}

	userUUID, err := uuid.Parse(userReq)
	// Can't parse UUID so we exit early
	if err != nil {
		statusCode = http.StatusBadRequest
		errMsg = "invalid user"
		s.respondWithHTML(w, templateName, pageData{
			Users:         users,
			CurrUser:      nil,
			CurrUserFeeds: currUserFeeds,
			CurrUserPosts: currUserPosts,
			ErrorString:   errMsg,
		}, statusCode)
		return
	}

	currUser, err := s.queries.GetUserById(ctx, userUUID)
	if errors.Is(err, sql.ErrNoRows) {
		statusCode = http.StatusBadRequest
		errMsg = "user does not exist"
		s.respondWithHTML(w, templateName, pageData{
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
		s.respondWithHTML(w, templateName, pageData{
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

	params := database.GetPostsForUserParams{
		UserID: currUser.ID,
		Limit:  50, // TODO Add in ability to paramerterize this (probably through a query again. Either implement skip or an increase to limit. Skip is the proper way.
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

	s.respondWithHTML(w, templateName, pageData{
		Users:         users,
		CurrUser:      toUserItem(currUser),
		CurrUserFeeds: currUserFeeds,
		CurrUserPosts: currUserPosts,
		ErrorString:   errMsg,
	}, statusCode)
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

func getTemplateName(r *http.Request, fullPage string, partial string) string {
	if r.Header.Get("HX-Request") == "true" {
		return partial
	}
	return fullPage
}
