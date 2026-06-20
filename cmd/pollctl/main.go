// pollctl is a CLI tool for admin operations on pollmd surveys.
// It wraps the admin HTTP API with five subcommands: list, create,
// reset, delete, and result. Standard library only — no external deps.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// --- Response types matching the server's JSON output ---

type surveySummary struct {
	SurveyID  string    `json:"survey_id"`
	Answers   []string  `json:"answers"`
	Mode      string    `json:"mode"`
	VoteCount int       `json:"vote_count"`
	CreatedAt time.Time `json:"created_at"`
}

type surveyDetail struct {
	SurveyID  string       `json:"survey_id"`
	Answers   []string     `json:"answers"`
	Mode      string       `json:"mode"`
	VoteCount int          `json:"vote_count"`
	CreatedAt time.Time    `json:"created_at"`
	Tallies   []tallyEntry `json:"tallies"`
}

type tallyEntry struct {
	Answer string `json:"answer"`
	Clicks int    `json:"clicks"`
}

type createResponse struct {
	SurveyID  string    `json:"survey_id"`
	Answers   string    `json:"answers"`
	Mode      string    `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	apiURL := os.Getenv("SURVEY_ADMIN_URL")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080"
	}
	apiToken := os.Getenv("SURVEY_ADMIN_TOKEN")

	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin, apiURL, apiToken))
}

// Run is the core entry point, extracted for testability. It parses args,
// dispatches to the appropriate subcommand, and returns an exit code.
func Run(args []string, out, errOut io.Writer, in io.Reader, apiURL, apiToken string) int {
	if len(args) == 0 {
		printUsage(errOut)
		return 1
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "list":
		return runList(subArgs, out, errOut, apiURL, apiToken)
	case "create":
		return runCreate(subArgs, out, errOut, apiURL, apiToken)
	case "reset":
		return runReset(subArgs, out, errOut, in, apiURL, apiToken)
	case "delete":
		return runDelete(subArgs, out, errOut, in, apiURL, apiToken)
	case "result":
		return runResult(subArgs, out, errOut, apiURL, apiToken)
	case "help", "-h", "--help":
		printUsage(out)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown subcommand: %s\n\n", subcommand)
		printUsage(errOut)
		return 1
	}
}

// --- HTTP helper ---

func doRequest(method, url, token string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// --- Subcommand: list ---

func runList(args []string, out, errOut io.Writer, apiURL, apiToken string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "output as JSON")
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	data, status, err := doRequest("GET", apiURL+"/admin/surveys", apiToken, nil)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	if status != 200 {
		fmt.Fprintln(errOut, "error: HTTP", status, string(data))
		return 1
	}

	var surveys []surveySummary
	if err := json.Unmarshal(data, &surveys); err != nil {
		fmt.Fprintln(errOut, "error: invalid response:", err)
		return 1
	}

	if *jsonFlag {
		return printJSON(out, surveys)
	}

	if len(surveys) == 0 {
		fmt.Fprintln(out, "No surveys found.")
		return 0
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tANSWERS\tMODE\tVOTES\tCREATED")
	for _, s := range surveys {
		answers := strings.Join(s.Answers, ",")
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", s.SurveyID, answers, s.Mode, s.VoteCount, s.CreatedAt.Format(time.DateOnly))
	}
	w.Flush()
	return 0
}

// --- Subcommand: create ---

func runCreate(args []string, out, errOut io.Writer, apiURL, apiToken string) int {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	answers := fs.String("answers", "", "comma-separated answer slugs (required)")
	mode := fs.String("mode", "anonymous", "survey mode: anonymous|named")
	jsonFlag := fs.Bool("json", false, "output as JSON")
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(errOut, "usage: pollctl create <survey-id> --answers a,b,c [--mode named]")
		return 1
	}
	surveyID := fs.Arg(0)
	if *answers == "" {
		fmt.Fprintln(errOut, "--answers is required")
		return 1
	}

	body := url.Values{
		"survey_id": {surveyID},
		"answers":   {*answers},
		"mode":      {*mode},
	}.Encode()

	data, status, err := doRequest("POST", apiURL+"/admin/surveys", apiToken, strings.NewReader(body))
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	if status != 201 {
		fmt.Fprintln(errOut, "error: HTTP", status, string(data))
		return 1
	}

	var resp createResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Fprintln(errOut, "error: invalid response:", err)
		return 1
	}

	if *jsonFlag {
		return printJSON(out, resp)
	}

	fmt.Fprintf(out, "Survey %q created (mode: %s)\n", resp.SurveyID, resp.Mode)
	return 0
}

// --- Subcommand: reset ---

func runReset(args []string, out, errOut io.Writer, in io.Reader, apiURL, apiToken string) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "output as JSON")
	yesFlag := fs.Bool("yes", false, "skip confirmation prompt")
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(errOut, "usage: pollctl reset <survey-id>")
		return 1
	}
	surveyID := fs.Arg(0)

	if !*yesFlag {
		fmt.Fprintf(out, "Reset all votes for survey %q? [y/N] ", surveyID)
		var response string
		fmt.Fscanln(in, &response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Fprintln(out, "Canceled.")
			return 0
		}
	}

	data, status, err := doRequest("POST", apiURL+"/admin/surveys/"+surveyID+"/reset", apiToken, nil)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	if status != 200 {
		fmt.Fprintln(errOut, "error: HTTP", status, string(data))
		return 1
	}

	if *jsonFlag {
		fmt.Fprint(out, string(data))
	} else {
		fmt.Fprintf(out, "Survey %q reset.\n", surveyID)
	}
	return 0
}

// --- Subcommand: delete ---

func runDelete(args []string, out, errOut io.Writer, in io.Reader, apiURL, apiToken string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "output as JSON")
	yesFlag := fs.Bool("yes", false, "skip confirmation prompt")
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(errOut, "usage: pollctl delete <survey-id>")
		return 1
	}
	surveyID := fs.Arg(0)

	if !*yesFlag {
		fmt.Fprintf(out, "Delete survey %q permanently? [y/N] ", surveyID)
		var response string
		fmt.Fscanln(in, &response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Fprintln(out, "Canceled.")
			return 0
		}
	}

	data, status, err := doRequest("DELETE", apiURL+"/admin/surveys/"+surveyID, apiToken, nil)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	if status != 200 {
		fmt.Fprintln(errOut, "error: HTTP", status, string(data))
		return 1
	}

	if *jsonFlag {
		fmt.Fprint(out, string(data))
	} else {
		fmt.Fprintf(out, "Survey %q deleted.\n", surveyID)
	}
	return 0
}

// --- Subcommand: result ---

func runResult(args []string, out, errOut io.Writer, apiURL, apiToken string) int {
	fs := flag.NewFlagSet("result", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "output as JSON")
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(errOut, "usage: pollctl result <survey-id>")
		return 1
	}
	surveyID := fs.Arg(0)

	data, status, err := doRequest("GET", apiURL+"/admin/surveys/"+surveyID, apiToken, nil)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	if status == 404 {
		fmt.Fprintf(errOut, "error: survey %q not found\n", surveyID)
		return 1
	}
	if status != 200 {
		fmt.Fprintln(errOut, "error: HTTP", status, string(data))
		return 1
	}

	var detail surveyDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		fmt.Fprintln(errOut, "error: invalid response:", err)
		return 1
	}

	if *jsonFlag {
		return printJSON(out, detail)
	}

	fmt.Fprintf(out, "Survey: %s (mode: %s)\n", detail.SurveyID, detail.Mode)
	fmt.Fprintf(out, "Total votes: %d\n\n", detail.VoteCount)

	if len(detail.Tallies) == 0 {
		fmt.Fprintln(out, "No votes recorded yet.")
		return 0
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ANSWER\tCLICKS")
	for _, t := range detail.Tallies {
		fmt.Fprintf(w, "%s\t%d\n", t.Answer, t.Clicks)
	}
	w.Flush()
	return 0
}

// --- Helpers ---

func printJSON(out io.Writer, v any) int {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: pollctl <subcommand> [options]

Subcommands:
  list                  List all surveys
  create <id>           Create a survey (--answers required, --mode optional)
  reset <id>            Reset votes for a survey (with confirmation)
  delete <id>           Delete a survey (with confirmation)
  result <id>           Show tally results for a survey

Environment:
  SURVEY_ADMIN_URL     Admin API URL (default: http://127.0.0.1:8080)
  SURVEY_ADMIN_TOKEN   Admin API auth token

Global flags:
  --json               Output as JSON (available on all subcommands)

Run 'pollctl <subcommand> -h' for subcommand-specific help.`)
}
