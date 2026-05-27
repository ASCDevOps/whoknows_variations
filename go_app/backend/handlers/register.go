package handlers

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"whoknows_backend/metrics"
	"whoknows_backend/security"
	"whoknows_backend/structs"
)

var registerTemplate = template.Must(template.ParseFiles("templates/layout.html", "templates/register.html"))

type RegisterHandler struct {
	DB *sql.DB
}

func (h *RegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = registerTemplate.ExecuteTemplate(w, "layout", nil)
		return
	case http.MethodPost:
	// nothing here, falls through to your existing logic below
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	req := structs.BodyRegisterAPIRegisterPost{
		Username:  strings.TrimSpace(r.FormValue("username")),
		Email:     strings.TrimSpace(r.FormValue("email")),
		Password:  strings.TrimSpace(r.FormValue("password")),
		Password2: strings.TrimSpace(r.FormValue("password2")),
	}
	// Validation
	if req.Username == "" || req.Email == "" || req.Password == "" {
		msg := "missing fields"

		writeJSON(w, http.StatusUnprocessableEntity, structs.AuthResponse{
			StatusCode: intPtr(422),
			Message:    &msg,
		})
		return
	}

	if req.Password != req.Password2 {
		msg := "passwords do not match"

		writeJSON(w, http.StatusUnprocessableEntity, structs.AuthResponse{
			StatusCode: intPtr(422),
			Message:    &msg,
		})
		return
	}

	if h.DB == nil {
		msg := "database not configured"
		writeRegisterJSON(w, http.StatusInternalServerError, structs.AuthResponse{
			StatusCode: intPtr(500),
			Message:    &msg,
		})
		return
	}

	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		msg := "failed to hash password"
		writeRegisterJSON(w, http.StatusInternalServerError, structs.AuthResponse{
			StatusCode: intPtr(500),
			Message:    &msg,
		})
		return
	}

	_, err = h.DB.Exec(
		"INSERT INTO users (username, email, password) VALUES ($1, $2, $3)",
		req.Username, req.Email, hashedPassword,
	)
	if err != nil {
		log.Printf("DB insert error: %v", err)
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			msg := "username or email already exists"
			writeRegisterJSON(w, http.StatusConflict, structs.AuthResponse{
				StatusCode: intPtr(409),
				Message:    &msg,
			})
			return
		}

		msg := "user creation failed"
		writeRegisterJSON(w, http.StatusInternalServerError, structs.AuthResponse{
			StatusCode: intPtr(500),
			Message:    &msg,
		})
		return
	}

	metrics.RegisterSuccessTotal.Inc()

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    req.Username,
		Path:     "/",
		HttpOnly: true,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func intPtr(i int) *int {
	return &i
}

func writeRegisterJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
	}
}
