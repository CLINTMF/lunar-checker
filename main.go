package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed index.html style.css app.js
var frontendFiles embed.FS

const cacheTTL = 60 * time.Second

var minecraftUsername = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)

type cacheEntry struct {
	Data      any
	ExpiresAt time.Time
}

type Server struct {
	Client *http.Client
	Mu     sync.RWMutex
	Cache  map[string]cacheEntry
}

type MinecraftStats struct {
	OK         bool              `json:"ok"`
	Server     string            `json:"server"`
	Username   string            `json:"username"`
	ProfileURL string            `json:"profileUrl"`
	SkinURL    string            `json:"skinUrl"`
	Status     string            `json:"status"`
	Source     string            `json:"source"`
	Stats      map[string]string `json:"stats"`
	FetchedAt  string            `json:"fetchedAt"`
}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return e.Message }

func (s *Server) fetchText(endpoint string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html, text/plain, application/json")
	req.Header.Set("User-Agent", "Lunar-Checker/1.0")
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", &apiError{Status: http.StatusNotFound, Message: "player not found"}
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		return "", &apiError{Status: http.StatusTooManyRequests, Message: "stats source rate limit reached"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &apiError{Status: http.StatusBadGateway, Message: fmt.Sprintf("stats source returned %s", resp.Status)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(body), err
}

func (s *Server) cached(key string, loader func() (any, error)) (any, bool, error) {
	s.Mu.RLock()
	entry, ok := s.Cache[key]
	s.Mu.RUnlock()
	if ok && time.Now().Before(entry.ExpiresAt) {
		return entry.Data, true, nil
	}
	data, err := loader()
	if err != nil {
		return nil, false, err
	}
	s.Mu.Lock()
	s.Cache[key] = cacheEntry{Data: data, ExpiresAt: time.Now().Add(cacheTTL)}
	s.Mu.Unlock()
	return data, false, nil
}

func readMetric(page, heading string) string {
	pattern := `(?ms)###\s+` + regexp.QuoteMeta(heading) + `\s*\n+\s*([^\n]+)`
	match := regexp.MustCompile(pattern).FindStringSubmatch(page)
	if len(match) < 2 {
		return "0"
	}
	return strings.TrimSpace(match[1])
}

func readStatus(page string) string {
	status := regexp.MustCompile(`(?m)^(Online|Offline)$`).FindStringSubmatch(page)
	if len(status) == 2 {
		return status[1]
	}
	return "Unknown"
}

func (s *Server) scrapeDonut(username string) (MinecraftStats, error) {
	source := "https://r.jina.ai/http://www.donutstats.net/player/" +
		url.PathEscape(username) + "?ref=player-stats"
	page, err := s.fetchText(source)
	if err != nil {
		return MinecraftStats{}, err
	}
	if strings.Contains(page, "No player named") || strings.Contains(page, "no stats to show") {
		return MinecraftStats{}, &apiError{Status: http.StatusNotFound, Message: "player has no DonutSMP stats"}
	}
	if !strings.Contains(page, "DonutSMP Player Stats") {
		return MinecraftStats{}, errors.New("DonutSMP returned an unreadable player page")
	}

	kills := readMetric(page, "Kills")
	deaths := readMetric(page, "Deaths")
	kd := regexp.MustCompile(`K/D:\s*([0-9.]+)`).FindStringSubmatch(page)
	kdValue := "0"
	if len(kd) == 2 {
		kdValue = kd[1]
	}
	return MinecraftStats{
		OK: true, Server: "DonutSMP", Username: username,
		ProfileURL: "https://www.donutstats.net/player/" + url.PathEscape(username),
		SkinURL:    "https://mc-heads.net/avatar/" + url.PathEscape(username) + "/128",
		Status:     readStatus(page), Source: "DonutStats",
		Stats: map[string]string{
			"money":        readMetric(page, "Money"),
			"shards":       readMetric(page, "Shards"),
			"playtime":     readMetric(page, "Playtime"),
			"kills":        kills,
			"deaths":       deaths,
			"kd":           kdValue,
			"blocksPlaced": readMetric(page, "Blocks Placed"),
			"blocksBroken": readMetric(page, "Blocks Broken"),
			"mobsKilled":   readMetric(page, "Mobs Killed"),
			"moneySpent":   readMetric(page, "Money Spent"),
			"moneyMade":    readMetric(page, "Money Made"),
		},
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Server) scrapeHypixel(username string) (MinecraftStats, error) {
	if os.Getenv("HYPIXEL_API_KEY") == "" {
		return MinecraftStats{}, &apiError{
			Status:  http.StatusNotImplemented,
			Message: "Hypixel stats require a HYPIXEL_API_KEY secret",
		}
	}
	return MinecraftStats{}, &apiError{
		Status:  http.StatusNotImplemented,
		Message: "Hypixel adapter is ready for the official API key",
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) serveFrontend(w http.ResponseWriter, name, contentType string) {
	content, err := frontendFiles.ReadFile(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "frontend file not found"})
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) respondStats(w http.ResponseWriter, serverName, username string) {
	if !minecraftUsername.MatchString(username) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid Minecraft username"})
		return
	}
	key := strings.ToLower(serverName + ":" + username)
	data, cached, err := s.cached(key, func() (any, error) {
		if serverName == "donut" {
			return s.scrapeDonut(username)
		}
		if serverName == "hypixel" {
			return s.scrapeHypixel(username)
		}
		return nil, &apiError{Status: http.StatusNotFound, Message: "unsupported Minecraft server"}
	})
	if err != nil {
		status := http.StatusBadGateway
		if apiErr, ok := err.(*apiError); ok {
			status = apiErr.Status
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": err.Error(), "server": serverName})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cached": cached, "data": data})
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "only GET is supported"})
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	switch path {
	case "":
		s.serveFrontend(w, "index.html", "text/html; charset=utf-8")
		return
	case "style.css":
		s.serveFrontend(w, "style.css", "text/css; charset=utf-8")
		return
	case "app.js":
		s.serveFrontend(w, "app.js", "application/javascript; charset=utf-8")
		return
	case "health":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "lunar-checker", "source": "Minecraft stats"})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "minecraft" {
		s.respondStats(w, parts[2], parts[3])
		return
	}
	if len(parts) == 3 && parts[0] == "api" && (parts[1] == "donut" || parts[1] == "hypixel") {
		s.respondStats(w, parts[1], parts[2])
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "route not found"})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	s := &Server{
		Client: &http.Client{Timeout: 12 * time.Second},
		Cache:  make(map[string]cacheEntry),
	}
	http.HandleFunc("/", s.handler)
	log.Printf("Lunar Checker listening on http://0.0.0.0:%s", port)
	log.Printf("Minecraft stats scraper: DonutSMP enabled")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
