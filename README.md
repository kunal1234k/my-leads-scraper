My Leads Scraper — Self-hosted AI Lead Generation (Codespaces)

Overview

My Leads Scraper is a self-hosted lead-generation platform designed to run inside GitHub Codespaces. It uses the open-source gosom/google-maps-scraper project as the core Google Maps scraping engine and builds a lightweight Go-based backend and simple frontend around it to: launch scrapes, parse results, normalize and deduplicate leads, analyze websites, and (later) run AI qualification and service-specific scoring.

This repository contains two top-level components:

- google-maps-scraper/ — upstream gosom/google-maps-scraper (DO NOT MODIFY).
- lead-gen-ui/ — custom application layer (Go backend + static frontend) that invokes the upstream scraper and provides a dashboard.

Goals

- Run completely inside GitHub Codespaces (no paid hosting required).
- Keep the upstream scraper code untouched and use it as a subprocess.
- Provide an easy-to-use UI accessible via the Codespaces forwarded port.
- Store data locally in JSON/CSV files (data/ folder) for V1, with clear migration paths to a database later.
- Ensure API keys (e.g., Google Gemini) remain server-side only and are loaded from environment variables / Codespaces secrets.

Quick start (Codespaces)

1. Open this repository in GitHub Codespaces.
2. Ensure the Codespace has Go installed (preinstalled in most Codespace dev containers).
3. From the Codespace terminal, install missing Playwright / Chromium runtime libraries if needed (the upstream scraper may require them):

   sudo apt-get update && sudo apt-get install -y libgbm1 libxkbcommon0 libasound2-data libasound2t64

4. Build the upstream scraper (if not already built):

   cd google-maps-scraper
   go build

   The binary will be: ./google-maps-scraper

5. Build and run the custom backend (lead-gen-ui):

   cd ../lead-gen-ui
   go build -o lead-gen-ui .
   ./lead-gen-ui

   The backend listens on port 3000 by default. In Codespaces the port will be forwarded automatically.

6. Open the forwarded port in the Codespaces browser preview or external browser. The simple frontend is served at http://localhost:3000/

What the UI does (V1)

- Accepts niche, location, number of leads, scraping options, and a short "What are you trying to sell?" description to provide AI context.
- Starts a background scraping job that runs the upstream scraper as a subprocess and writes logs and results to data/jobs/<jobId>/.
- Parses results.csv into a normalized JSON leads file data/leads/<jobId>.json.
- Shows job status and a simple list of parsed leads.
- Saves the scrape configuration (including the service description) to data/jobs/<jobId>/config.json so downstream AI qualification can use it.

Project structure

/workspaces/my-leads-scraper/

- google-maps-scraper/         # Upstream scraper (do not modify)
- lead-gen-ui/
  - main.go                    # Go backend (serves UI, starts scrapes)
  - go.mod
  - static/
    - index.html               # Simple frontend
    - app.js
    - style.css
  - data/
    - jobs/                    # Per-job data (queries, logs, results, config.json)
    - leads/                   # Normalized leads per job (jobId.json)
    - exports/                 # Generated CSV/JSON exports
  - internal/                  # Placeholder for future packages (scraper, leads, scoring, gemini, website)

Security & secrets

- GEMINI_API_KEY and other secrets must never be embedded in frontend code or committed to Git.
- Use Codespaces environment variables or secrets to set GEMINI_API_KEY before enabling AI features.
- .env.example is provided in lead-gen-ui/ and .env must be added to .gitignore (already configured).

Development notes & roadmap

PHASE 0: Environment validation
- Ensure Playwright/Chromium runtime dependencies are installed in the Codespace so the upstream scraper can run headless browsers.

PHASE 1 (current): Basic backend + UI
- Serve static frontend, accept jobs, run scraper in background, parse CSV into JSON leads, show simple dashboard.

PHASE 2: Polished dashboard
- Filters, sorting, lead table, detail panel, job progress updates.

PHASE 3: Website analysis
- Basic site checks (reachable, HTTPS, viewport meta, response time), cache results.

PHASE 4: Gemini AI qualification
- Server-side Gemini integration for structured AI qualification; results are stored and cached per lead.

PHASE 5: Service-specific scoring
- Deterministic scoring engine configurable per service (website dev, AI automation, etc.) and transparent score breakdown.

PHASE 6: Exports
- CSV/JSON exports include lead fields, scores, AI reasoning, and sales angle.

PHASE 7: Polish & hardening
- UI improvements, error handling, job cancellation, rate limiting, concurrency controls, and optional DB migration.

Contributing

- Keep changes to lead-gen-ui/ and avoid editing google-maps-scraper/ unless absolutely necessary.
- Follow the incremental development process: small changes, build, test, and verify before pushing.

License

This repository follows the licenses of upstream components. The custom application code is provided under MIT license (unless otherwise specified). Confirm license compatibility before distribution.

Support

If you need help setting up the Codespace runtime libraries or getting the scraper to produce results, open an issue with the relevant logs from data/jobs/<jobId>/scraper.stderr.log and scraper.stdout.log.

Contact

For questions about the implementation or to request features, open an issue in this repository.
