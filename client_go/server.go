//go:build windows

package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// cachedHostname slaat de hostnaam op die bij opstart eenmalig wordt bepaald.
// HTTP-handlers roepen mustHostname() aan via deze cache; geen syscall per request.
var cachedHostname string

func init() {
	h, err := os.Hostname()
	if err != nil {
		cachedHostname = "" // valt terug op T("hostname.unknown") in mustHostname()
		return
	}
	cachedHostname = h
}

// maxBodyBytes is de maximale JSON-body-grootte die we accepteren (8 KB).
// Voorkomt dat een aanvaller de server laat vastlopen met een enorme body.
const maxBodyBytes = 8 * 1024

// validShutdownTypes is de lijst van geaccepteerde afsluittype-sleutels.
// De API is altijd Engelstalig zodat Home Assistant de responses correct verwerkt.
var validShutdownTypes = map[string]struct{}{
	"shutdown":  {},
	"restart":   {},
	"hibernate": {},
	"sleep":     {},
	"logoff":    {},
}

// typeKeys is de gesorteerde slice van geldige afsluittypen — dezelfde data als
// validShutdownTypes maar als geordende slice voor gebruik in de UI-combobox,
// CLI-validatie en foutmeldingen. Afgeleid van validShutdownTypes zodat er maar
// één waarheidsbron is: voeg een type toe aan validShutdownTypes en het verschijnt
// automatisch ook in de UI.
var typeKeys = func() []string {
	keys := make([]string, 0, len(validShutdownTypes))
	for k := range validShutdownTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}()

// validTypeList is de gesorteerde kommalijst van geldige typen voor foutmeldingen.
var validTypeList = strings.Join(typeKeys, ", ")

// ── Router ────────────────────────────────────────────────────────────────────

// shutdownRateLimiter voorkomt dat een aanvaller op het LAN razendsnel
// opeenvolgende shutdown-requests stuurt. Maximaal één verzoek per 10 seconden
// wordt verwerkt; eerder aankomende verzoeken krijgen HTTP 429.
var shutdownRateLimiter = newRateLimiter(10 * time.Second)

// rateLimiter staat maximaal één aanroep per interval toe.
type rateLimiter struct {
	interval time.Duration
	mu       sync.Mutex
	lastOK   time.Time
}

func newRateLimiter(interval time.Duration) *rateLimiter {
	return &rateLimiter{interval: interval}
}

// allow geeft true terug als het verzoek verwerkt mag worden.
func (r *rateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if now.Sub(r.lastOK) >= r.interval {
		r.lastOK = now
		return true
	}
	return false
}

// newServeMux maakt de HTTP-router aan en geeft een http.Server terug met
// alle aanbevolen timeouts ingesteld.
//
// Timeouts:
//   - ReadTimeout:     tijd voor het volledig lezen van de request (headers + body)
//   - WriteTimeout:    tijd voor het volledig versturen van de response
//   - IdleTimeout:     keep-alive verbindingen worden na deze tijd gesloten
//   - ReadHeaderTimeout: aparte header-read timeout (beschermt tegen Slowloris)
func newServer(cfg *Config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/verify", withAuth(handleVerify))
	mux.HandleFunc("/shutdown", withAuth(func(w http.ResponseWriter, r *http.Request) {
		handleShutdown(w, r, cfg)
	}))

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// ── Hulpfuncties ──────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// withAuth controleert de X-API-Key header (of ?api_key= query-parameter).
//
// De vergelijking gebruikt crypto/subtle.ConstantTimeCompare zodat de
// looptijd niet afhangt van de lengte van de overeenkomst (timing-safe).
// Dit voorkomt timing-aanvallen waarbij een aanvaller via responstijden
// karakter voor karakter de sleutel kan afleiden.
func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-API-Key")
		if token == "" {
			token = r.URL.Query().Get("api_key")
		}
		// Verwijder eventuele onbedoelde witruimte (copy-paste uit config).
		token = strings.TrimSpace(token)

		// Haal het actieve wachtwoord op uit de in-memory atomic cache.
		// Dit is thread-safe en ~nanoseconden snel; geen registry-toegang.
		pw := getCurrentPassword()
		pwBytes := []byte(pw)
		tokenBytes := []byte(token)
		match := len(tokenBytes) == len(pwBytes) &&
			subtle.ConstantTimeCompare(tokenBytes, pwBytes) == 1

		if !match {
			// Diagnostische log: toon ontvangen én verwachte sleutellengte.
			// Als recv=0: header ontbreekt volledig.
			// Als recv==want maar toch mismatch: inhoud klopt niet.
			// Als recv!=want: sleutel is ingekort, verlengd of bevat witruimte.
			log.Printf(
				"Unauthorized: method=%s path=%s remote=%s recv_key_len=%d want_key_len=%d",
				r.Method, r.URL.Path, r.RemoteAddr, len(token), len(pw),
			)
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Invalid API key",
			})
			return
		}
		next(w, r)
	}
}

// requireJSON controleert of de Content-Type header aangeeft dat de body
// JSON is. Geeft false terug (en schrijft een 415-respons) als dat niet zo is.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error": "Content-Type must be application/json",
		})
		return false
	}
	return true
}

// ── Eindpunten ────────────────────────────────────────────────────────────────

// GET /status — geen authenticatie.
//
// Geeft `online: true` terug als boolean (niet als string "online") zodat
// Home Assistant de waarde direct kan evalueren zonder string-vergelijking.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed. Use GET.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"online":   true,
		"hostname": mustHostname(),
		"version":  version,
	})
}

// GET /verify — authenticatie vereist.
// Test de API-sleutel zonder actie.
func handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed. Use GET.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "authenticated",
		"hostname": mustHostname(),
	})
}

// POST /shutdown — authenticatie vereist.
//
// Optionele JSON-body (Content-Type: application/json):
//
//	{
//	    "delay": 30,         // seconden vóór afsluiten (overschrijft standaard)
//	    "type":  "shutdown"  // shutdown|restart|hibernate|sleep|logoff
//	}
//
// Opmerking delay per type:
//   - shutdown / restart : delay wordt doorgegeven aan Windows shutdown.exe (/t)
//   - hibernate / sleep  : delay wordt als Go-sleep uitgevoerd (OS-commando kent geen /t)
//   - logoff             : delay wordt als Go-sleep uitgevoerd (shutdown /l kent geen /t)
func handleShutdown(w http.ResponseWriter, r *http.Request, cfg *Config) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed. Use POST.",
		})
		return
	}

	// Rate-limiting: max. één shutdown-verzoek per 10 seconden.
	// Beschermt tegen flood-aanvallen van op het LAN.
	if !shutdownRateLimiter.allow() {
		log.Printf("Rate limit overschreden: shutdown geweigerd (remote=%s)", r.RemoteAddr)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many requests. Please wait before sending another shutdown command.",
		})
		return
	}

	var body struct {
		Delay *int   `json:"delay"`
		Type  string `json:"type"`
	}

	// Lees de body als ContentLength > 0 OF als de lengte onbekend is (-1,
	// bij chunked transfer-encoding). In beide gevallen passen we de limiet toe.
	if r.ContentLength != 0 {
		// Content-Type controleren vóór het parsen van de body.
		if !requireJSON(w, r) {
			return
		}

		// Begrens de body tot maxBodyBytes. Door maxBodyBytes+1 te gebruiken
		// als limiet kunnen we daarna via lr.N==0 detecteren of de body te groot
		// was: als de reader volledig leeg is (N==0), waren er ≥ maxBodyBytes+1
		// bytes aangeboden.
		lr := &io.LimitedReader{R: r.Body, N: maxBodyBytes + 1}
		dec := json.NewDecoder(lr)
		dec.DisallowUnknownFields()

		if err := dec.Decode(&body); err != nil {
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
		// Succesvolle decode: check alsnog op te-grote body (decoder leest
		// soms precies tot de limiet zonder een fout te geven).
		if lr.N == 0 {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
				"error": fmt.Sprintf("Request body too large (max %d bytes)", maxBodyBytes),
			})
			return
		}
	}

	delay := cfg.Delay
	if body.Delay != nil {
		delay = *body.Delay
	}
	shutdownType := cfg.ShutdownType
	if body.Type != "" {
		shutdownType = body.Type
	}

	// Validatie
	if _, ok := validShutdownTypes[shutdownType]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Invalid type %q. Valid values: %s", shutdownType, validTypeList),
		})
		return
	}
	if delay < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "delay must be >= 0",
		})
		return
	}

	go executeShutdown(shutdownType, delay)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "accepted",
		"type":          shutdownType,
		"delay_seconds": delay,
	})
}

// ── Afsluitlogica ─────────────────────────────────────────────────────────────

// executeShutdown voert de afsluitopdracht uit in een achtergrond-goroutine.
//
// Delay-implementatie:
//   - shutdown/restart : /t <delay> wordt aan Windows shutdown.exe meegegeven
//     zodat de OS-teller zichtbaar is voor de gebruiker.
//   - hibernate/sleep  : Go-sleep vóór het OS-commando (OS kent geen /t).
//   - logoff           : Go-sleep vóór shutdown /l (accepteert geen /t).
func executeShutdown(shutdownType string, delay int) {
	// Korte wachttijd zodat de HTTP-respons de client bereikt vóór de actie
	time.Sleep(300 * time.Millisecond)

	var cmd *exec.Cmd

	switch shutdownType {
	case "shutdown":
		cmd = exec.Command("shutdown", "/s", "/t", strconv.Itoa(delay), "/f")

	case "restart":
		cmd = exec.Command("shutdown", "/r", "/t", strconv.Itoa(delay), "/f")

	case "hibernate":
		// shutdown.exe heeft geen /t voor slaapstand; sleep in Go
		if delay > 0 {
			log.Printf("Hibernate: wacht %ds voor uitvoering", delay)
			time.Sleep(time.Duration(delay) * time.Second)
		}
		cmd = exec.Command("shutdown", "/h")

	case "sleep":
		// SetSuspendState kent ook geen delay-parameter
		if delay > 0 {
			log.Printf("Sleep: wacht %ds voor uitvoering", delay)
			time.Sleep(time.Duration(delay) * time.Second)
		}
		cmd = exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0")

	case "logoff":
		// shutdown /l accepteert geen /t; gebruik Go-sleep
		if delay > 0 {
			log.Printf("Logoff: wacht %ds voor uitvoering", delay)
			time.Sleep(time.Duration(delay) * time.Second)
		}
		cmd = exec.Command("shutdown", "/l")
	}

	// Opmerking: een default-geval is hier niet nodig — de validatie hierboven
	// (validShutdownTypes) garandeert dat shutdownType altijd één van de vijf
	// bekende waarden is. cmd is dan altijd gezet.

	if cmd == nil {
		// Defensieve check: zou nooit bereikt moeten worden door de validatie.
		log.Printf("Shutdown afgebroken: onbekend type %q (dit is een bug)", shutdownType)
		return
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Start(); err != nil {
		log.Printf("Shutdown failed (type=%s): %v", shutdownType, err)
	} else {
		log.Printf("Shutdown initiated: type=%s delay=%ds", shutdownType, delay)
	}
}
