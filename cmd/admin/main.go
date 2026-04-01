// asterisk-admin is a CLI for managing Asterisk bot users via the HTTP admin API.
//
// Usage:
//
//	asterisk-admin [--url <url>] [--token <token>] <command>
//
// Commands:
//
//	health
//	users list
//	users grant <telegram_id>
//	users revoke <telegram_id>
//
// Config precedence:
//  1. --url / --token flags
//  2. ASTERISK_URL / ASTERISK_TOKEN env vars
//  3. Default URL: http://localhost:8080
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	// Top-level flags
	fs := flag.NewFlagSet("asterisk-admin", flag.ExitOnError)
	flagURL := fs.String("url", "", "Admin API base URL (env: ASTERISK_URL)")
	flagToken := fs.String("token", "", "Admin API token (env: ASTERISK_TOKEN)")

	// We parse flags manually to allow them before or after the subcommand.
	// Collect remaining args after flag parsing.
	_ = fs.Parse(os.Args[1:])
	args := fs.Args()

	// Resolve config with precedence: flag > env > default
	baseURL := resolveConfig(*flagURL, "ASTERISK_URL", "http://localhost:8080")
	token := resolveConfig(*flagToken, "ASTERISK_TOKEN", "")

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "health":
		runHealth(baseURL, token)

	case "users":
		if len(args) < 2 {
			printUsage()
			os.Exit(1)
		}
		switch args[1] {
		case "list":
			runUsersList(baseURL, token)

		case "grant":
			if len(args) < 3 {
				fmt.Fprintln(os.Stderr, "Error: missing <telegram_id>")
				os.Exit(1)
			}
			id := mustParseID(args[2])
			runUserAction(baseURL, token, id, "grant")

		case "revoke":
			if len(args) < 3 {
				fmt.Fprintln(os.Stderr, "Error: missing <telegram_id>")
				os.Exit(1)
			}
			id := mustParseID(args[2])
			runUserAction(baseURL, token, id, "revoke")

		default:
			fmt.Fprintf(os.Stderr, "Error: unknown users sub-command %q\n", args[1])
			printUsage()
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func resolveConfig(flagVal, envKey, defaultVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return defaultVal
}

func mustParseID(s string) int64 {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid telegram_id %q: %v\n", s, err)
		os.Exit(1)
	}
	return id
}

// --- HTTP helpers ---

type apiClient struct {
	base  string
	token string
	http  *http.Client
}

func newClient(base, token string) *apiClient {
	return &apiClient{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *apiClient) do(method, path string) (*http.Response, error) {
	u, err := url.JoinPath(c.base, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func handleHTTPError(resp *http.Response, action string) {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		fmt.Fprintln(os.Stderr, "Error: Invalid admin token. Set ASTERISK_TOKEN or use --token.")
	default:
		fmt.Fprintf(os.Stderr, "Error: HTTP %d for %s\n", resp.StatusCode, action)
	}
	os.Exit(1)
}

func isConnectionRefused(err error) bool {
	return err != nil && (errors.Is(err, errors.New("connection refused")) ||
		strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connect: "))
}

// --- Commands ---

func runHealth(base, token string) {
	c := newClient(base, token)
	resp, err := c.do(http.MethodGet, "/health")
	if err != nil {
		if isConnectionRefused(err) {
			fmt.Fprintf(os.Stderr, "Error: Could not connect to %s\nMake sure the admin API is reachable (try kubectl port-forward if running in-cluster).\n", base)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		handleHTTPError(resp, "health")
	}

	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	fmt.Printf("Status: %s\n", body["status"])
}

type apiUser struct {
	TelegramID int64     `json:"telegram_id"`
	Username   string    `json:"username"`
	FirstName  string    `json:"first_name"`
	FullAccess bool      `json:"full_access"`
	FirstSeen  time.Time `json:"first_seen"`
	DailyCount int       `json:"daily_count"`
}

const defaultDailyLimit = 15

func runUsersList(base, token string) {
	c := newClient(base, token)

	resp, err := c.do(http.MethodGet, "/users")
	if err != nil {
		if isConnectionRefused(err) {
			fmt.Fprintf(os.Stderr, "Error: Could not connect to %s\nMake sure the admin API is reachable (try kubectl port-forward if running in-cluster).\n", base)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		handleHTTPError(resp, "GET /users")
	}

	var body struct {
		Users []apiUser `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to decode response: %v\n", err)
		os.Exit(1)
	}

	if len(body.Users) == 0 {
		fmt.Println("No users found.")
		return
	}

	// Header
	fmt.Printf("%-16s %-16s %-16s %-10s %s\n",
		"ID", "USERNAME", "NAME", "ACCESS", "TODAY")
	fmt.Println(strings.Repeat("\u2500", 70))

	for _, u := range body.Users {
		access := "limited"
		if u.FullAccess {
			access = "full"
		}

		today := fmt.Sprintf("%d/%d", u.DailyCount, defaultDailyLimit)
		if u.FullAccess {
			today = "-"
		}

		fmt.Printf("%-16d %-16s %-16s %-10s %s\n",
			u.TelegramID,
			truncate(u.Username, 16),
			truncate(u.FirstName, 16),
			access,
			today,
		)
	}
}

func runUserAction(base, token string, telegramID int64, action string) {
	c := newClient(base, token)

	path := fmt.Sprintf("/users/%d/%s", telegramID, action)
	resp, err := c.do(http.MethodPost, path)
	if err != nil {
		if isConnectionRefused(err) {
			fmt.Fprintf(os.Stderr, "Error: Could not connect to %s\nMake sure the admin API is reachable (try kubectl port-forward if running in-cluster).\n", base)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		handleHTTPError(resp, fmt.Sprintf("POST %s", path))
	}

	switch action {
	case "grant":
		fmt.Printf("\u2713 Full access granted to user %d.\n", telegramID)
	case "revoke":
		fmt.Printf("\u2713 Full access revoked for user %d. Default limit (%d/day) applies.\n", telegramID, defaultDailyLimit)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "\u2026"
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  asterisk-admin [--url <url>] [--token <token>] <command>

Commands:
  health
  users list
  users grant <telegram_id>
  users revoke <telegram_id>

Config (precedence: flag > env > default):
  --url    / ASTERISK_URL    Admin API base URL (default: http://localhost:8080)
  --token  / ASTERISK_TOKEN  Admin API bearer token`)
}
