package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"ttui/internal/api"
	"ttui/internal/auth"
	"ttui/internal/store"
	"ttui/internal/ui"
)

// runDev handles `ttui dev ...`, developer-only helpers that use TTUI_DEV_TOKEN.
//
//	ttui dev run                      the TUI, with TTUI_CLIENT_ID from .env
//	ttui dev demo                     the TUI on the design's mock data (no API)
//	ttui dev lists                    list names and open task counts
//	ttui dev raw METHOD PATH [JSON]   print status and raw response body
func runDev(args []string) error {
	loadDotEnv(".env")
	if len(args) > 0 && args[0] == "oauth" {
		return devOAuth()
	}
	if len(args) > 0 && args[0] == "run" {
		return ui.Run(version)
	}
	if len(args) > 0 && args[0] == "demo" {
		return ui.RunDemo(version, store.Demo(time.Now()))
	}
	token := os.Getenv("TTUI_DEV_TOKEN")
	if token == "" {
		return errors.New("TTUI_DEV_TOKEN not set (put it in .env)")
	}
	c := api.New(token)
	ctx := context.Background()

	if len(args) == 0 {
		return errors.New("usage: ttui dev lists | ttui dev raw METHOD PATH [JSON]")
	}
	switch args[0] {
	case "lists":
		projects, err := c.Projects(ctx)
		if err != nil {
			return err
		}
		tasks, err := c.OpenTasks(ctx)
		if err != nil {
			return err
		}
		// Inbox is not in GET /project; its tasks carry projectId "inbox<userId>".
		counts := map[string]int{}
		for _, t := range tasks {
			if strings.HasPrefix(t.ProjectID, "inbox") {
				t.ProjectID = "inbox"
			}
			counts[t.ProjectID]++
		}
		fmt.Printf("%-30s %d\n", "Inbox", counts["inbox"])
		for _, p := range projects {
			if !p.Closed {
				fmt.Printf("%-30s %d\n", p.Name, counts[p.ID])
			}
		}
		return nil
	case "raw":
		if len(args) < 3 {
			return errors.New("usage: ttui dev raw METHOD PATH [JSON]")
		}
		var body any
		if len(args) > 3 {
			body = json.RawMessage(args[3])
		}
		var out json.RawMessage
		err := c.Do(ctx, strings.ToUpper(args[1]), args[2], body, &out)
		var apiErr *api.Error
		if errors.As(err, &apiErr) {
			fmt.Printf("%d\n%s\n", apiErr.Status, apiErr.Body)
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("200\n%s\n", out)
		return nil
	}
	return fmt.Errorf("unknown dev command %q", args[0])
}

// devOAuth runs the browser sign-in once and reports what the token response contains
// (never the token values), then saves auth.toml.
func devOAuth() error {
	f, err := auth.Start()
	if err != nil {
		return err
	}
	fmt.Println("Open this URL if the browser does not start:\n" + f.URL)
	if err := exec.Command("xdg-open", f.URL).Start(); err != nil {
		fmt.Println("(could not open browser:", err, ")")
	}
	fmt.Println("Waiting for callback on localhost:8421 ...")
	tok, err := f.Wait(context.Background())
	if err != nil {
		return err
	}
	fmt.Println("token exchange OK")
	fmt.Println("  refresh_token present:", tok.RefreshToken != "")
	fmt.Println("  expires_in (s):", tok.ExpiresIn)
	fmt.Println("  token_type:", tok.TokenType, " scope:", tok.Scope)

	ps, err := api.New(tok.AccessToken).Projects(context.Background())
	if err != nil {
		return fmt.Errorf("token does not work against the API: %w", err)
	}
	fmt.Println("  API check: GET /project OK,", len(ps), "lists")

	if err := auth.Save(auth.FromToken(tok, "ticktick.com")); err != nil {
		return err
	}
	st, _ := os.Stat(auth.Path())
	fmt.Printf("saved %s (%v)\n", auth.Path(), st.Mode().Perm())
	return nil
}

// loadDotEnv sets KEY=VALUE pairs from path without overriding the real environment.
// Missing file is not an error.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}
