package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed public/index.html
var indexHTML []byte

const (
	defaultPort = "3000"
	cacheTTL    = 60 * time.Second
)

type CacheEntry struct {
	Data      GitHubStats
	ExpiresAt time.Time
}

type Server struct {
	Client *http.Client
	Token  string
	Mu     sync.RWMutex
	Cache  map[string]CacheEntry
}

type Profile struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	HTMLURL     string `json:"html_url"`
	Bio         string `json:"bio"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
	PublicRepos int    `json:"public_repos"`
	PublicGists int    `json:"public_gists"`
}

type Repo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
	Stars       int    `json:"stargazers_count"`
	Forks       int    `json:"forks_count"`
	OpenIssues  int    `json:"open_issues_count"`
	Language    string `json:"language"`
	UpdatedAt   string `json:"updated_at"`
	Fork        bool   `json:"fork"`
}

type GitHubStats struct {
	Platform   string         `json:"platform"`
	Username   string         `json:"username"`
	Name       string         `json:"name"`
	Avatar     string         `json:"avatar"`
	ProfileURL string         `json:"profileUrl"`
	Bio        string         `json:"bio"`
	Company    string         `json:"company"`
	Location   string         `json:"location"`
	CreatedAt  string         `json:"createdAt"`
	UpdatedAt  string         `json:"updatedAt"`
	Stats      StatSet        `json:"stats"`
	Languages  map[string]int `json:"languages"`
	TopRepos   []RepoView     `json:"topRepos"`
	FetchedAt  string         `json:"fetchedAt"`
}

type StatSet struct {
	Followers   int `json:"followers"`
	Following   int `json:"following"`
	PublicRepos int `json:"publicRepos"`
	PublicGists int `json:"publicGists"`
	TotalStars  int `json:"totalStars"`
	TotalForks  int `json:"totalForks"`
	OpenIssues  int `json:"openIssues"`
	OwnedRepos  int `json:"ownedRepos"`
}

type RepoView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Stars       int    `json:"stars"`
	Forks       int    `json:"forks"`
	Language    string `json:"language"`
	UpdatedAt   string `json:"updatedAt"`
}

func (s *Server) githubRequest(path string, result any) error {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lunar-stats-api/1.0")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found")
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("rate limited")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (s *Server) getStats(username string) (GitHubStats, bool, error) {
	key := strings.ToLower(username)
	s.Mu.RLock()
	entry, exists := s.Cache[key]
	s.Mu.RUnlock()
	if exists && time.Now().Before(entry.ExpiresAt) {
		return entry.Data, true, nil
	}

	var profile Profile
	var repos []Repo
	var profileErr, reposErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		profileErr = s.githubRequest("/users/"+username, &profile)
	}()
	go func() {
		defer wg.Done()
		reposErr = s.githubRequest("/users/"+username+"/repos?per_page=100&sort=updated", &repos)
	}()
	wg.Wait()
	if profileErr != nil {
		return GitHubStats{}, false, profileErr
	}
	if reposErr != nil {
		return GitHubStats{}, false, reposErr
	}

	owned := make([]Repo, 0, len(repos))
	languages := map[string]int{}
	totalStars, totalForks, openIssues := 0, 0, 0
	for _, repo := range repos {
		if repo.Fork {
			continue
		}
		owned = append(owned, repo)
		totalStars += repo.Stars
		totalForks += repo.Forks
		openIssues += repo.OpenIssues
		if repo.Language != "" {
			languages[repo.Language]++
		}
	}
	sort.Slice(owned, func(i, j int) bool {
		if owned[i].Stars == owned[j].Stars {
			return owned[i].Forks > owned[j].Forks
		}
		return owned[i].Stars > owned[j].Stars
	})
	top := make([]RepoView, 0, min(10, len(owned)))
	for _, repo := range owned[:min(10, len(owned))] {
		top = append(top, RepoView{
			Name: repo.Name, Description: repo.Description, URL: repo.HTMLURL,
			Stars: repo.Stars, Forks: repo.Forks, Language: repo.Language, UpdatedAt: repo.UpdatedAt,
		})
	}
	data := GitHubStats{
		Platform: "github", Username: profile.Login, Name: profile.Name,
		Avatar: profile.AvatarURL, ProfileURL: profile.HTMLURL, Bio: profile.Bio,
		Company: profile.Company, Location: profile.Location, CreatedAt: profile.CreatedAt,
		UpdatedAt: profile.UpdatedAt, Languages: languages, TopRepos: top,
		Stats: StatSet{
			Followers: profile.Followers, Following: profile.Following,
			PublicRepos: profile.PublicRepos, PublicGists: profile.PublicGists,
			TotalStars: totalStars, TotalForks: totalForks, OpenIssues: openIssues,
			OwnedRepos: len(owned),
		},
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.Mu.Lock()
	s.Cache[key] = CacheEntry{Data: data, ExpiresAt: time.Now().Add(cacheTTL)}
	s.Mu.Unlock()
	return data, false, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexHTML)
		return
	case "health":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "lunar-stats-api", "uptime": time.Since(started).String()})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 3 && (parts[0] == "stats" || parts[0] == "github") && parts[1] == "github" {
		// /stats/github/:username
		s.respondStats(w, parts[2])
		return
	}
	if len(parts) == 2 && parts[0] == "github" {
		// /github/:username
		s.respondStats(w, parts[1])
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "route not found"})
}

func (s *Server) respondStats(w http.ResponseWriter, username string) {
	if username == "" || strings.ContainsAny(username, "/?#") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "valid GitHub username required"})
		return
	}
	data, cached, err := s.getStats(username)
	if err != nil {
		status := http.StatusBadGateway
		message := "could not fetch GitHub stats"
		if err.Error() == "not found" {
			status, message = http.StatusNotFound, "GitHub user not found"
		} else if err.Error() == "rate limited" {
			status, message = http.StatusTooManyRequests, "GitHub rate limit reached"
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": message})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cached": cached, "data": data})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var started = time.Now()

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	if _, err := strconv.Atoi(port); err != nil {
		log.Fatalf("invalid PORT: %s", port)
	}
	server := &Server{
		Client: &http.Client{Timeout: 5 * time.Second},
		Token:  os.Getenv("GITHUB_TOKEN"),
		Cache:  make(map[string]CacheEntry),
	}
	http.HandleFunc("/", server.handler)
	log.Printf("Lunar Checker listening on http://0.0.0.0:%s", port)
	log.Printf("GitHub token: %t", server.Token != "")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
