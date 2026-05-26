package handlers

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"
	"whoknows_backend/metrics"
)

var searchTemplate = template.Must(template.ParseFiles("templates/layout.html", "templates/search.html"))

type SearchHandler struct {
	DB *sql.DB
}

type SearchPageData struct {
	User          any
	Flash         string
	Query         string
	SearchResults []SearchResult
}

type SearchResult struct {
	Title       string
	URL         string
	Description string
}

func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	data := SearchPageData{
		Query: query,
	}

	cookie, err := r.Cookie("session")
	if err == nil {
		data.User = cookie.Value
	}

	if query != "" {
		data.SearchResults = queryPages(h.DB, query, r.URL.Path, r.Method)
	}

	_ = searchTemplate.ExecuteTemplate(w, "layout", data)
}

type SearchAPIHandler struct {
	DB *sql.DB
}

type SearchResponse struct {
	Data []map[string]any `json:"data"`
}

func (h *SearchAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	results := queryPages(h.DB, query, r.URL.Path, r.Method)

	data := make([]map[string]any, len(results))
	for i, r := range results {
		data[i] = map[string]any{
			"title":       r.Title,
			"url":         r.URL,
			"description": r.Description,
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(SearchResponse{Data: data}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func queryPages(db *sql.DB, query, path, method string) []SearchResult {
	metrics.SearchRequestsTotal.Inc()
	metrics.SearchQueriesTotal.WithLabelValues(query).Inc()

	start := time.Now()
	defer func() {
		metrics.SearchRequestDuration.WithLabelValues(path, method).Observe(time.Since(start).Seconds())
	}()

	rows, err := db.Query(`
		SELECT title, url, content
		FROM pages
		WHERE content ILIKE $1 OR title ILIKE $1
		ORDER BY title
		LIMIT 20
	`, "%"+query+"%")
	if err != nil {
		metrics.SearchErrorsTotal.Inc()
		return nil
	}

	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	var results []SearchResult
	for rows.Next() {
		var sr SearchResult
		var content string
		if err := rows.Scan(&sr.Title, &sr.URL, &content); err != nil {
			continue
		}
		if len(content) > 200 {
			sr.Description = content[:200] + "..."
		} else {
			sr.Description = content
		}
		results = append(results, sr)
	}
	if err := rows.Err(); err != nil {
		metrics.SearchErrorsTotal.Inc()
		log.Printf("rows iteration error: %v", err)
	}

	if len(results) == 0 {
		metrics.SearchNoResultsTotal.Inc()
	} else {
		metrics.SearchResultsTotal.Add(float64(len(results)))
	}

	return results
}
