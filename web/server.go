package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"gator/database"
)

type Server struct {
	queries   *database.Queries
	server    *http.Server
	mux       *http.ServeMux
	template  *template.Template
	startTime time.Time
}

func NewServer(queries *database.Queries, templateDir string, srvPort string) (*Server, error) {
	globPattern := filepath.Join(templateDir, "*.html")
	parsedTemplate, err := template.ParseGlob(globPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template files with pattern %q: %w", globPattern, err)
	}

	mux := http.NewServeMux()
	srv := http.Server{
		Addr:    srvPort,
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

	return serverStruct, nil
}

func (s *Server) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /feeds", s.handleGetFeeds)
}

func (s *Server) uptime() string {
	d := time.Since(s.startTime)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}
