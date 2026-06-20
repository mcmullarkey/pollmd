package server

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sspaeti/minimal-newsletter-survey/internal/store"
	"github.com/sspaeti/minimal-newsletter-survey/internal/voter"
)

type Config struct {
	DBPath     string
	HTTPAddr   string
	QuackAddr  string
	QuackToken string
	AdminToken string
	SiteURL    string
}

//go:embed thanks.html result.html landing.html home.html style.css ogimage.png ogimage-q.png
var staticFS embed.FS

// Route patterns. Edit here if the URL shape ever changes — the rest of the
// file uses these constants so any path change is a single edit. The vote
// patterns use Go 1.22 ServeMux wildcards; r.PathValue("id") and
// r.PathValue("answer") pull the segments inside the handler.
const (
	routeVote          = "/{id}/{answer}"        // primary, short form (e.g. q.ssp.sh/init/awesome)
	routeVoteLegacy    = "/survey/{id}/{answer}" // kept so old survey links keep working
	routeResult        = "/result/{id}"          // server-rendered tally page
	routeLanding       = "/{id}"                 // landing page with answer buttons for registered surveys
	routeLandingLegacy = "/survey/{id}"          // alias matching the explicit /survey/ form the user might type
	routeHome          = "/{$}"                  // root explainer page; {$} anchors to "/" only so /{id} stays distinct
	routeThanks        = "/thanks"
	routeHealth        = "/healthz"
	routeStyle         = "/style.css"      // shared CSS for thanks.html + result.html
	routeOGImage       = "/og-image.png"   // social-card image for landing.html + result.html
	routeOGImage2      = "/og-image-q.png" // social-card image for home.html (root /)
	routeAdminCreate   = "POST /admin/surveys"
	routeAdminReset    = "POST /admin/surveys/{id}/reset"
	routeAdminDelete   = "DELETE /admin/surveys/{id}"
	routeAdminList     = "GET /admin/surveys"
	routeAdminDetail   = "GET /admin/surveys/{id}"
)

// Public URLs the root home page links to. Hardcoded rather than wired
// through Config because there's exactly one canonical pollmd deployment;
// fork/reuse cases can promote these to env vars later.
const (
	homeDocsURL   = "https://pollmd.ssp.sh/"
	homeGitHubURL = "https://github.com/sspaeti/pollmd"
)

// slugRe gates both survey_id and answer. Lowercase alnum, dash, underscore,
// must start with alnum, max 64 chars. Keeps the URL space clean and the
// table free of arbitrary user-supplied data.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// botUASubstrings matches common link unfurlers, RSS readers, search
// crawlers, headless-browser link checkers, and security scanners that
// fetch the URL with GET but do not represent a human click. Matched
// case-insensitively as substrings against the User-Agent header.
//
// Refine this list when a new platform shows up in the vote tally with
// suspicious volume — re-deploy and re-test. Order does not matter.
var botUASubstrings = []string{
	// Social media link unfurlers
	"twitterbot", "facebookexternalhit", "linkedinbot", "slackbot",
	"slack-imgproxy", "discordbot", "telegrambot", "whatsapp",
	"skypeuripreview", "redditbot", "pinterestbot", "applebot", "tumblr",
	"cardyb", "bsky", "bluesky", "mastodon", "akkoma", "pleroma",
	"fediverse",
	// Search-engine + SEO crawlers
	"googlebot", "bingbot", "yandex", "duckduckbot", "baiduspider",
	"ahrefsbot", "semrushbot", "mj12bot", "petalbot",
	// Headless browser link checkers
	"headlesschrome", "phantomjs", "puppeteer", "selenium", "playwright",
	"lighthouse",
	// RSS / feed readers
	"feedfetcher", "rssbot", "inoreader", "feedly", "newsblur",
	// Generic HTTP clients (bots rarely customise these)
	"curl/", "wget/", "python-requests", "python-urllib",
	"go-http-client", "okhttp", "java/", "apache-httpclient", "httpx",
	"node-fetch", "axios/",
	// Security / Safe-Links / URL scanners
	"safelinks", "urlscan", "virustotal", "phishtank",
	// Generic bot markers
	"bot/", "crawler", "spider", "scraper", "preview",
}

// isBotUA returns true for User-Agent strings that look like automation
// rather than a human-driven browser. An empty UA also counts — every
// mainstream browser sends one.
func isBotUA(ua string) bool {
	if ua == "" {
		return true
	}
	ua = strings.ToLower(ua)
	for _, sub := range botUASubstrings {
		if strings.Contains(ua, sub) {
			return true
		}
	}
	return false
}

type Server struct {
	cfg      Config
	store    *store.Store
	salt     *voter.Salt
	thanks   *template.Template
	result   *template.Template
	landing  *template.Template
	home     *template.Template
	css      []byte // cached at startup, served from routeStyle
	ogImage  []byte // cached at startup, served from routeOGImage
	ogImage2 []byte // cached at startup, served from routeOGImage2 (home page card)
}

func New(cfg Config, st *store.Store) *Server {
	css, err := staticFS.ReadFile("style.css")
	if err != nil {
		panic("embedded style.css missing: " + err.Error())
	}
	ogImage, err := staticFS.ReadFile("ogimage.png")
	if err != nil {
		panic("embedded ogimage.png missing: " + err.Error())
	}
	ogImage2, err := staticFS.ReadFile("ogimage-q.png")
	if err != nil {
		panic("embedded ogimage-q.png missing: " + err.Error())
	}
	return &Server{
		cfg:      cfg,
		store:    st,
		salt:     voter.NewSalt(),
		thanks:   template.Must(template.ParseFS(staticFS, "thanks.html")),
		result:   template.Must(template.ParseFS(staticFS, "result.html")),
		landing:  template.Must(template.ParseFS(staticFS, "landing.html")),
		home:     template.Must(template.ParseFS(staticFS, "home.html")),
		css:      css,
		ogImage:  ogImage,
		ogImage2: ogImage2,
	}
}

// Handler returns the http.Handler with all routes registered.
// Separate from ListenAndServe so tests can pass the mux to httptest.Server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Order doesn't matter for Go 1.22 ServeMux — more specific patterns
	// always win. /result/{id} beats /{id}/{answer} for paths under /result/.
	mux.HandleFunc(routeResult, s.handleResult)
	mux.HandleFunc(routeVote, s.handleSurvey)
	mux.HandleFunc(routeVoteLegacy, s.handleSurvey)
	mux.HandleFunc(routeLanding, s.handleLanding)
	mux.HandleFunc(routeLandingLegacy, s.handleLanding)
	mux.HandleFunc(routeHome, s.handleHome)
	mux.HandleFunc(routeThanks, s.handleThanks)
	mux.HandleFunc(routeStyle, s.handleStyle)
	mux.HandleFunc(routeOGImage, s.handleOGImage)
	mux.HandleFunc(routeOGImage2, s.handleOGImage2)
	mux.HandleFunc(routeAdminList, s.handleAdminList)
	mux.HandleFunc(routeAdminDetail, s.handleAdminDetail)
	mux.HandleFunc(routeAdminCreate, s.handleAdminCreate)
	mux.HandleFunc(routeAdminReset, s.handleAdminReset)
	mux.HandleFunc(routeAdminDelete, s.handleAdminDelete)
	mux.HandleFunc(routeHealth, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}

func (s *Server) ListenAndServe() error {
	httpServer := &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return httpServer.ListenAndServe()
}

func (s *Server) handleSurvey(w http.ResponseWriter, r *http.Request) {
	// Email scanners (Microsoft Safe Links, Gmail prefetch) issue HEAD before
	// the user actually clicks. Reply 200 but do not record.
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	surveyID := r.PathValue("id")
	answer := r.PathValue("answer")
	if !slugRe.MatchString(surveyID) || !slugRe.MatchString(answer) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ua := r.Header.Get("User-Agent")
	// Social-media link unfurlers, RSS readers, security scanners, etc. fetch
	// the URL with GET (not HEAD), so this is needed in addition to the HEAD
	// guard above. Reply 200 but do not record.
	if isBotUA(ua) {
		log.Printf("bot-skip survey_id=%s answer=%s", surveyID, answer)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Optional per-survey answer allowlist. Populated by `make survey-create`
	// (writes a row into the `surveys` table via Quack). If the survey isn't
	// registered, GetAllowedAnswers returns nil and we stay in open mode —
	// any slug-valid answer counts. If it IS registered, only listed answers
	// are recorded; anything else returns 200 without writing, same shape as
	// the bot-skip path above.
	allowed, err := s.store.GetAllowedAnswers(surveyID)
	if err != nil {
		log.Printf("allowed-answers lookup: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if allowed != nil && !allowed[answer] {
		log.Printf("answer-reject survey_id=%s answer=%s", surveyID, answer)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check if the survey supports named voting. Unregistered surveys
	// default to "anonymous" mode.
	mode, err := s.store.GetSurveyMode(surveyID)
	if err != nil {
		log.Printf("survey mode lookup: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var voterName string
	if mode == "named" {
		voterName = r.URL.Query().Get("name")
		if len(voterName) > 200 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}

	ip := clientIP(r)
	vh := voter.Hash(ip, ua, surveyID, s.salt.Current())

	if err := s.store.RecordVote(surveyID, answer, vh, voterName); err != nil {
		log.Printf("record vote: survey=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	log.Printf("vote survey_id=%s answer=%s", surveyID, answer)
	http.Redirect(w, r, "/thanks?id="+surveyID, http.StatusFound)
}

func (s *Server) handleStyle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Write(s.css)
}

func (s *Server) handleOGImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	// Long cache: this is a static asset that only changes on redeploy. Social
	// scrapers (Twitter/FB/LinkedIn) cache aggressively anyway.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Write(s.ogImage)
}

func (s *Server) handleOGImage2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Write(s.ogImage2)
}

func (s *Server) handleThanks(w http.ResponseWriter, r *http.Request) {
	data := struct {
		SiteURL  string
		SurveyID string
	}{
		SiteURL:  s.cfg.SiteURL,
		SurveyID: r.URL.Query().Get("id"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.thanks.Execute(w, data); err != nil {
		log.Printf("thanks template: %v", err)
	}
}

// tallyRow is the view-model passed to result.html — Tally as it comes out
// of the store, plus a PercentOfMax we computed for the CSS bar width, and
// Voters (voter names for named-mode surveys).
type tallyRow struct {
	Answer       string
	Clicks       int
	PercentOfMax int
	Voters       []string // voter names, empty for anonymous-mode surveys
}

type resultPageData struct {
	SurveyID      string
	Tallies       []tallyRow
	TotalVotes    int
	Mode          string // "anonymous" or "named"
	MutedText     string // mode-aware disclaimer
	SiteURL       string
	PageURL       string // absolute URL of this result page, for og:url
	OGImageURL    string // absolute URL of the social-card image, for og:image
	OGTitle       string
	OGDescription string
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	surveyID := r.PathValue("id")
	if !slugRe.MatchString(surveyID) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	rows, err := s.store.TallyBySurvey(surveyID)
	if err != nil {
		log.Printf("tally survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Check mode and fetch voter names if named
	mode, err := s.store.GetSurveyMode(surveyID)
	if err != nil {
		log.Printf("survey mode: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var namesByAnswer map[string][]string
	if mode == "named" {
		namesByAnswer, err = s.store.VoteNamesByAnswer(surveyID)
		if err != nil {
			log.Printf("vote names: survey_id=%s err=%v", surveyID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	total, top := 0, 0
	for _, t := range rows {
		total += t.Clicks
		if t.Clicks > top {
			top = t.Clicks
		}
	}
	view := make([]tallyRow, len(rows))
	for i, t := range rows {
		pct := 0
		if top > 0 {
			pct = (t.Clicks * 100) / top
		}
		view[i] = tallyRow{
			Answer:       t.Answer,
			Clicks:       t.Clicks,
			PercentOfMax: pct,
			Voters:       namesByAnswer[t.Answer],
		}
	}

	mutedText := "Live tally from the poll database. Each vote was hashed anonymously " +
		"with a salt that rotates every day at midnight UTC, so individual " +
		"voters can't be tied to clicks."
	if mode == "named" {
		mutedText = "Live tally from the poll database. Voters chose to show " +
			"their name alongside their vote."
	}

	base := publicBaseURL(r)
	data := resultPageData{
		SurveyID:      surveyID,
		Tallies:       view,
		TotalVotes:    total,
		Mode:          mode,
		MutedText:     mutedText,
		SiteURL:       s.cfg.SiteURL,
		PageURL:       base + r.URL.Path,
		OGImageURL:    base + routeOGImage,
		OGTitle:       "Survey results: " + surveyID,
		OGDescription: "Live tally of poll responses for " + surveyID + ".",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.result.Execute(w, data); err != nil {
		log.Printf("result template: %v", err)
	}
}

// landingAnswer is one button on the landing page — the slug (URL segment
// part) and a human-friendly Label derived from the slug.
type landingAnswer struct {
	Slug  string
	Label string
}

type landingPageData struct {
	SurveyID      string
	Answers       []landingAnswer
	PageURL       string
	OGImageURL    string
	OGTitle       string
	OGDescription string
}

// handleLanding serves /{id} and /survey/{id} — the public vote landing page
// that lists the registered answer slugs as buttons. Only renders for
// surveys that have a row in the `surveys` table; unregistered (open-mode)
// surveys 404 so we don't accidentally expose a wildcard "anyone can guess
// a slug" landing.
func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	surveyID := r.PathValue("id")
	if !slugRe.MatchString(surveyID) {
		http.NotFound(w, r)
		return
	}

	answers, err := s.store.GetSurveyAnswers(surveyID)
	if err != nil {
		log.Printf("survey answers lookup: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if answers == nil {
		// Unregistered survey — no landing page to show.
		http.NotFound(w, r)
		return
	}

	links := make([]landingAnswer, len(answers))
	for i, a := range answers {
		links[i] = landingAnswer{Slug: a, Label: titleAnswer(a)}
	}

	base := publicBaseURL(r)
	data := landingPageData{
		SurveyID:      surveyID,
		Answers:       links,
		PageURL:       base + r.URL.Path,
		OGImageURL:    base + routeOGImage,
		OGTitle:       "Vote: " + surveyID,
		OGDescription: "One click to record your vote for poll " + surveyID + ".",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.landing.Execute(w, data); err != nil {
		log.Printf("landing template: %v", err)
	}
}

// titleAnswer turns a slug like "not-sure" or "could_be_better" into a display
// label "Not Sure" / "Could Be Better". Splits on both `-` and `_` so either
// slug style renders cleanly. Validation upstream guarantees slugs are
// lower-case ASCII so byte-indexing the first rune of each part is safe.
func titleAnswer(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// homePageData is the view-model for home.html — the root explainer page
// at /. Only externally-visible URLs need to be passed in; the marketing
// copy lives in the template.
type homePageData struct {
	PageURL       string
	OGImageURL    string
	OGTitle       string
	OGDescription string
	DocsURL       string
	GitHubURL     string
}

// handleHome serves the root explainer page at /. The {$} anchor on
// routeHome ensures this never shadows /{id} or /{id}/{answer}; the
// shadowing test in server_test.go pins that — don't drop the anchor.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := publicBaseURL(r)
	data := homePageData{
		PageURL:       base + "/",
		OGImageURL:    base + routeOGImage2,
		OGTitle:       "pollmd — minimal polls in Markdown",
		OGDescription: "A ~200-line Go service that records anonymous poll responses into a DuckDB file. No cookies, no JS.",
		DocsURL:       homeDocsURL,
		GitHubURL:     homeGitHubURL,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.home.Execute(w, data); err != nil {
		log.Printf("home template: %v", err)
	}
}

// validAdminToken checks the request's Authorization header against the
// configured admin token using constant-time comparison. Returns true if
// the token matches, false if auth is missing or wrong.
func (s *Server) validAdminToken(r *http.Request) bool {
	if s.cfg.AdminToken == "" {
		return false
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	given := strings.TrimPrefix(auth, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(given), []byte(s.cfg.AdminToken)) == 1
}

// adminCreateRequest is the JSON body accepted by handleAdminCreate.
// Form-urlencoded fields follow the same key names.
type adminCreateRequest struct {
	SurveyID string `json:"survey_id"`
	Answers  string `json:"answers"`
	Mode     string `json:"mode"`
}

// Admin JSON response types

type adminSurveyListItem struct {
	SurveyID  string    `json:"survey_id"`
	Answers   []string  `json:"answers"`
	Mode      string    `json:"mode"`
	VoteCount int       `json:"vote_count"`
	CreatedAt time.Time `json:"created_at"`
}

type adminSurveyDetail struct {
	SurveyID  string        `json:"survey_id"`
	Answers   []string      `json:"answers"`
	Mode      string        `json:"mode"`
	VoteCount int           `json:"vote_count"`
	CreatedAt time.Time     `json:"created_at"`
	Tallies   []store.Tally `json:"tallies"`
}

type adminCreateResponse struct {
	SurveyID  string    `json:"survey_id"`
	Answers   string    `json:"answers"`
	Mode      string    `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
}

// handleAdminCreate creates or updates a survey's allowed-answers
// registration. Accepts both JSON and form-urlencoded POST bodies.
// Protected by Bearer token auth.
func (s *Server) handleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.validAdminToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req adminCreateRequest
	switch ct := r.Header.Get("Content-Type"); {
	case strings.HasPrefix(ct, "application/json"):
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	default:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.SurveyID = r.FormValue("survey_id")
		req.Answers = r.FormValue("answers")
		req.Mode = r.FormValue("mode")
	}

	if !slugRe.MatchString(req.SurveyID) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Answers == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Validate mode if provided
	if req.Mode != "" && req.Mode != "anonymous" && req.Mode != "named" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Validate each answer slug
	for _, a := range strings.Split(req.Answers, ",") {
		a = strings.TrimSpace(a)
		if !slugRe.MatchString(a) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}

	if err := s.store.UpsertSurvey(req.SurveyID, req.Answers, req.Mode); err != nil {
		log.Printf("admin create: survey_id=%s err=%v", req.SurveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	respMode := req.Mode
	if respMode == "" {
		respMode = "anonymous"
	}

	createdAt, err := s.store.GetSurveyCreatedAt(req.SurveyID)
	if err != nil {
		log.Printf("get created_at: survey_id=%s err=%v", req.SurveyID, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(adminCreateResponse{
		SurveyID:  req.SurveyID,
		Answers:   req.Answers,
		Mode:      respMode,
		CreatedAt: createdAt,
	})
}

// handleAdminList returns all registered surveys as a JSON array.
func (s *Server) handleAdminList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.validAdminToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	summaries, err := s.store.ListSurveys()
	if err != nil {
		log.Printf("list surveys: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]adminSurveyListItem, 0, len(summaries))
	for _, sum := range summaries {
		items = append(items, adminSurveyListItem{
			SurveyID:  sum.SurveyID,
			Answers:   strings.Split(sum.AllowedAnswers, ","),
			Mode:      sum.Mode,
			VoteCount: sum.VoteCount,
			CreatedAt: sum.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// handleAdminDetail returns the full detail for a single survey as JSON.
func (s *Server) handleAdminDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.validAdminToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	surveyID := surveyIDFromPath(r)
	if !slugRe.MatchString(surveyID) {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	// Check existence
	answers, err := s.store.GetSurveyAnswers(surveyID)
	if err != nil {
		log.Printf("get survey answers: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if answers == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}

	mode, err := s.store.GetSurveyMode(surveyID)
	if err != nil {
		log.Printf("get survey mode: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	createdAt, err := s.store.GetSurveyCreatedAt(surveyID)
	if err != nil {
		log.Printf("get created_at: survey_id=%s err=%v", surveyID, err)
		// Non-fatal — default to zero time
	}

	tallies, err := s.store.TallyBySurvey(surveyID)
	if err != nil {
		log.Printf("tally: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	total := 0
	for _, t := range tallies {
		total += t.Clicks
	}

	data := adminSurveyDetail{
		SurveyID:  surveyID,
		Answers:   answers,
		Mode:      mode,
		VoteCount: total,
		CreatedAt: createdAt,
		Tallies:   tallies,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// surveyIDFromPath extracts the survey ID from the URL path for admin
// routes. Both /admin/surveys/{id} and /admin/surveys/{id}/reset have
// the ID at the 4th path segment (index 3 after splitting). We parse
// manually rather than using r.PathValue because the tests call handlers
// directly without going through the mux.
func surveyIDFromPath(r *http.Request) string {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

// handleAdminReset clears all votes for a survey while preserving the survey
// registration (allowed_answers). Protected by Bearer token auth.
func (s *Server) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.validAdminToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	surveyID := surveyIDFromPath(r)
	if !slugRe.MatchString(surveyID) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Check existence
	answers, err := s.store.GetSurveyAnswers(surveyID)
	if err != nil {
		log.Printf("get survey: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if answers == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "survey not found"})
		return
	}

	if err := s.store.ResetVotes(surveyID); err != nil {
		log.Printf("admin reset: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleAdminDelete fully removes a survey: all votes and the registration
// itself. Protected by Bearer token auth.
func (s *Server) handleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.validAdminToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	surveyID := surveyIDFromPath(r)
	if !slugRe.MatchString(surveyID) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Check existence
	answers, err := s.store.GetSurveyAnswers(surveyID)
	if err != nil {
		log.Printf("get survey: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if answers == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "survey not found"})
		return
	}

	if err := s.store.DeleteSurvey(surveyID); err != nil {
		log.Printf("admin delete: survey_id=%s err=%v", surveyID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// publicBaseURL returns the scheme+host that an external client used to reach
// us — needed for absolute URLs in og: tags (social scrapers reject relative
// URLs). Trusts X-Forwarded-Proto from the upstream reverse proxy; defaults to
// https since this app only ships behind TLS termination in real deployments.
func publicBaseURL(r *http.Request) string {
	scheme := "https"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	return scheme + "://" + r.Host
}

// clientIP picks the request's source IP, preferring proxy-set headers over
// the raw TCP RemoteAddr. The priority matches goatcounter's zhttp/mware.RealIP
// (and chi's middleware.RealIP): X-Real-IP first (Caddy/NPM set this as a
// single value), then the leftmost X-Forwarded-For hop (Railway and most cloud
// edges), then RemoteAddr. The IP is only used as one input to the voter hash;
// it is never persisted.
func clientIP(r *http.Request) string {
	if rip := strings.TrimSpace(r.Header.Get("X-Real-Ip")); rip != "" {
		return rip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
