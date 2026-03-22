package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type errorResponse struct {
	ErrorMsg string `json:"error"`
}

type healthResponse struct {
	Healthy bool   `json:"healthy"`
	Db      string `json:"database_status"`
	Uptime  string `json:"uptime"`
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
		Db:      dbStatus,
		Uptime:  s.uptime(),
	}

	data, err := json.Marshal(response)
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
