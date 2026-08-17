package main

import (
	"bufio"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	port = ":3000"
	jobsDir = "data/jobs"
	leadsDir = "data/leads"
)

var mu sync.Mutex
var scraperPath string
var absJobsDir string
var absLeadsDir string

func main() {
	// compute project root and absolute data dirs
	cwd, _ := os.Getwd()
	projectRoot := cwd
	absJobsDir = filepath.Join(projectRoot, jobsDir)
	absLeadsDir = filepath.Join(projectRoot, leadsDir)
	if err := os.MkdirAll(absJobsDir, 0o755); err != nil {
		log.Fatalf("failed to create jobs dir: %v", err)
	}
	if err := os.MkdirAll(absLeadsDir, 0o755); err != nil {
		log.Fatalf("failed to create leads dir: %v", err)
	}

	// compute absolute scraper path (upstream)
	// 1) prefer explicit env var
	if v := os.Getenv("SCRAPER_PATH"); v != "" {
		scraperPath = v
		log.Printf("SCRAPER_PATH from env: %s\n", scraperPath)
	} else {
		// candidate locations to try (relative to cwd and common Codespaces path)
		candidates := []string{
			filepath.Clean(filepath.Join(cwd, "..", "google-maps-scraper", "google-maps-scraper")),
			"/workspaces/my-leads-scraper/google-maps-scraper/google-maps-scraper",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				scraperPath = c
				break
			}
		}
		if scraperPath == "" {
			// fallback to first candidate (may fail later with clear error)
			scraperPath = candidates[0]
			log.Printf("Warning: scraper binary not found in candidates, will attempt %s\n", scraperPath)
		} else {
			log.Printf("Using discovered scraper binary: %s\n", scraperPath)
		}
	}

	http.Handle("/", http.FileServer(http.Dir("static")))
	http.HandleFunc("/api/scrape", handleScrape)
	http.HandleFunc("/api/jobs/", handleJobStatus)
	// simple health
	http.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request){ w.Write([]byte("ok")) })

	log.Printf("Starting server on %s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type ScrapeRequest struct {
	Niche              string `json:"niche"`
	Location           string `json:"location"`
	ServiceDescription string `json:"serviceDescription"`
	Limit              int    `json:"limit"`
	Depth              int    `json:"depth"`
	Concurrency        int    `json:"concurrency"`
	ExtractEmail       bool   `json:"extractEmail"`
	FastMode           bool   `json:"fastMode"`
}

type JobStatus struct {
	JobID    string `json:"jobId"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

func writeJobStatus(jobDir string, s JobStatus) error {
	f, err := os.Create(filepath.Join(jobDir, "status.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

func handleScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ScrapeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Niche == "" || req.Location == "" {
		http.Error(w, "niche and location required", http.StatusBadRequest)
		return
	}
	if req.Depth <= 0 { req.Depth = 2 }
	if req.Concurrency <= 0 { req.Concurrency = 1 }
	if req.Limit <= 0 { req.Limit = 20 }

	jobId := randomID()
	jobDir := filepath.Join(absJobsDir, jobId)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		http.Error(w, "failed to create job dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// create queries.txt
	query := fmt.Sprintf("%s in %s", req.Niche, req.Location)
	if err := os.WriteFile(filepath.Join(jobDir, "queries.txt"), []byte(query+"\n"), 0o644); err != nil {
		http.Error(w, "failed to write queries: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// write job config (includes serviceDescription for AI context)
	cfgPath := filepath.Join(jobDir, "config.json")
	cfgF, err := os.Create(cfgPath)
	if err == nil {
		enc := json.NewEncoder(cfgF)
		enc.SetIndent("", "  ")
		_ = enc.Encode(req)
		cfgF.Close()
	} else {
		log.Printf("warning: failed to write job config: %v", err)
	}

	// initial status
	s := JobStatus{JobID: jobId, Status: "queued", Progress: 0, Message: "Job queued"}
	_ = writeJobStatus(jobDir, s)

	// run in background
	go runScraperJob(jobId, jobDir, req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"jobId": jobId, "status": "queued"})
}

func handleJobStatus(w http.ResponseWriter, r *http.Request) {
	// GET /api/jobs/:id
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 || parts[3] == "" {
		http.Error(w, "job id required", http.StatusBadRequest)
		return
	}
	jobId := parts[3]
	jobDir := filepath.Join(jobsDir, jobId)
	b, err := os.ReadFile(filepath.Join(jobDir, "status.json"))
	if err != nil {
		http.Error(w, "status not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func runScraperJob(jobId, jobDir string, req ScrapeRequest) {
	log.Printf("Job %s: starting\n", jobId)
	_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "running", Progress: 1, Message: "Starting scraper"})

	// build command args
	args := []string{"-input", "queries.txt", "-results", "results.csv", "-c", strconv.Itoa(req.Concurrency), "-depth", strconv.Itoa(req.Depth), "-exit-on-inactivity", "1m"}
	if req.ExtractEmail {
		args = append(args, "-email")
	}
	if req.FastMode {
		args = append(args, "-fast-mode")
	}
	cmdPath := scraperPath
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = jobDir

	stdoutF, _ := os.Create(filepath.Join(jobDir, "scraper.stdout.log"))
	stderrF, _ := os.Create(filepath.Join(jobDir, "scraper.stderr.log"))
	cmd.Stdout = io.MultiWriter(stdoutF)
	cmd.Stderr = io.MultiWriter(stderrF)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		log.Printf("Job %s: failed to start scraper: %v\n", jobId, err)
		_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "failed", Progress: 0, Message: "failed to start scraper: " + err.Error()})
		return
	}
	_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "running", Progress: 5, Message: "Scraper started"})

	// wait in goroutine
	err := cmd.Wait()
	duration := time.Since(start)
	if err != nil {
		log.Printf("Job %s: scraper finished with error: %v\n", jobId, err)
		_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "failed", Progress: 0, Message: "scraper failed: " + err.Error()})
		return
	}
	log.Printf("Job %s: scraper finished in %v\n", jobId, duration)
	_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "processing", Progress: 60, Message: "Parser starting"})

	// parse results.csv
	resultsPath := filepath.Join(jobDir, "results.csv")
	if _, err := os.Stat(resultsPath); os.IsNotExist(err) {
		log.Printf("Job %s: results.csv not found\n", jobId)
		_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "failed", Progress: 0, Message: "results.csv not found"})
		return
	}

	leads, err := parseCSVToLeads(resultsPath, req)
	if err != nil {
		log.Printf("Job %s: parse error: %v\n", jobId, err)
		_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "failed", Progress: 0, Message: "parse error: " + err.Error()})
		return
	}

	// write leads to data/leads/{jobId}.json
	f, err := os.Create(filepath.Join(absLeadsDir, jobId+".json"))
	if err != nil {
		log.Printf("Job %s: failed to create leads file: %v\n", jobId, err)
		_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "failed", Progress: 0, Message: "write leads failed: " + err.Error()})
		return
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(leads)
	f.Close()

	_ = writeJobStatus(jobDir, JobStatus{JobID: jobId, Status: "completed", Progress: 100, Message: fmt.Sprintf("Completed: %d leads", len(leads))})
	log.Printf("Job %s: completed, %d leads\n", jobId, len(leads))
}

// Lead is a minimal normalized lead structure for phase 1
type Lead struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Category string `json:"category"`
	Address string `json:"address"`
	Phone string `json:"phone"`
	Website string `json:"website"`
	Rating string `json:"rating"`
	ReviewCount string `json:"reviewCount"`
	GoogleMapsURL string `json:"googleMapsURL"`
	Raw map[string]string `json:"raw"`
}

func parseCSVToLeads(path string, req ScrapeRequest) ([]Lead, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()

	r := csv.NewReader(bufio.NewReader(f))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	// header to index
	headerIndex := map[string]int{}
	for i, h := range head {
		headerIndex[h] = i
	}

	var leads []Lead
	for {
		rec, err := r.Read()
		if err == io.EOF { break }
		if err != nil { log.Printf("csv read error: %v", err); break }
		// create map
		m := map[string]string{}
		for k, idx := range headerIndex {
			if idx < len(rec) {
				m[k] = rec[idx]
			} else {
				m[k] = ""
			}
		}
		id := randomID()
		lead := Lead{
			ID: id,
			Name: m["title"],
			Category: m["category"],
			Address: m["address"],
			Phone: m["phone"],
			Website: m["website"],
			Rating: m["review_rating"],
			ReviewCount: m["review_count"],
			GoogleMapsURL: m["link"],
			Raw: m,
		}
		leads = append(leads, lead)
		if req.Limit > 0 && len(leads) >= req.Limit { break }
	}
	return leads, nil
}
