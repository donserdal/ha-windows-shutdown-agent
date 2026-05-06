//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lxn/walk"
)

// notifyRateLimiter staat maximaal één notificatie per 2 seconden toe.
var notifyRateLimiter = newRateLimiter(2 * time.Second)

// notifyRequest is de verwachte JSON-body voor POST /notify.
type notifyRequest struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// POST /notify — authenticatie vereist.
//
// Verwacht JSON-body:
//
//	{"title": "...", "message": "..."}
//
// Toont een Windows berichtvenster op het bureaublad.
func handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed. Use POST.",
		})
		return
	}

	if !notifyRateLimiter.allow() {
		log.Printf("Rate limit overschreden: notify geweigerd (remote=%s)", r.RemoteAddr)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many requests. Please wait before sending another notification.",
		})
		return
	}

	if !requireJSON(w, r) {
		return
	}

	lr := &io.LimitedReader{R: r.Body, N: maxBodyBytes + 1}
	dec := json.NewDecoder(lr)
	dec.DisallowUnknownFields()

	var req notifyRequest
	if err := dec.Decode(&req); err != nil {
		if lr.N == 0 {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
				"error": fmt.Sprintf("Request body too large (max %d bytes)", maxBodyBytes),
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON: " + err.Error(),
		})
		return
	}
	if lr.N == 0 {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": fmt.Sprintf("Request body too large (max %d bytes)", maxBodyBytes),
		})
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Message = strings.TrimSpace(req.Message)

	if req.Title == "" || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Both 'title' and 'message' are required and must not be empty.",
		})
		return
	}

	const maxTitle = 256
	const maxMessage = 1024
	if len(req.Title) > maxTitle {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("'title' exceeds maximum length of %d characters", maxTitle),
		})
		return
	}
	if len(req.Message) > maxMessage {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("'message' exceeds maximum length of %d characters", maxMessage),
		})
		return
	}

	// MB_SETFOREGROUND (0x00010000) zorgt dat het venster bovenop alle andere
	// vensters verschijnt, ook als de client op de achtergrond draait.
	const mbSetForeground = 0x00010000
	go walk.MsgBox(nil, req.Title, req.Message, walk.MsgBoxOK|walk.MsgBoxIconInformation|mbSetForeground)

	log.Printf("Notificatie verstuurd: title=%q", req.Title)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "accepted",
	})
}
