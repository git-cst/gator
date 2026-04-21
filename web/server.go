package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"gator/config"
	"gator/database"
)

//go:embed static
var embeddedFiles embed.FS

type Server struct {
	queries   *database.Queries
	server    *http.Server
	mux       *http.ServeMux
	template  *template.Template
	startTime time.Time
}

func NewServer(queries *database.Queries, serviceConfig *config.ServiceConfig) (*Server, error) {
	parsedTemplate, err := template.ParseFS(embeddedFiles, "static/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse template files: %w", err)
	}

	mux := http.NewServeMux()
	srv := http.Server{
		Addr:    ":" + serviceConfig.ServerPort,
		Handler: mux,
	}

	serverStruct := &Server{
		queries:   queries,
		server:    &srv,
		mux:       mux,
		template:  parsedTemplate,
		startTime: time.Now(),
	}

	serverStruct.registerRoutes()
	serverStruct.setupStaticFiles()

	return serverStruct, nil
}

func (s *Server) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) setupStaticFiles() {
	s.mux.Handle("GET /static/", http.FileServer(http.FS(embeddedFiles)))
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)

	s.mux.HandleFunc("GET /", s.handleNotFound)

	s.mux.HandleFunc("GET /feeds", s.handleGetFeeds)
	s.mux.HandleFunc("POST /feeds", s.handleAddFeed)
	s.mux.HandleFunc("POST /feeds/{id}/unsubscribe", s.handleUnsubscribeUserFromFeed)
	s.mux.HandleFunc("POST /feeds/{id}/subscribe", s.handleSubscribeUserToFeed)

	s.mux.HandleFunc("GET /posts", s.handleGetPosts)
	s.mux.HandleFunc("POST /posts/{id}/toggle_read", s.handleTogglePostReadStatus)
	s.mux.HandleFunc("POST /posts/{id}/mark_read", s.handleMarkPostRead)
}

func (s *Server) uptime() string {
	d := time.Since(s.startTime)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}
