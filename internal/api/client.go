// Package api is a minimal client for the TickTick Open API v1.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.ticktick.com/open/v1"

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(token string) *Client {
	return &Client{BaseURL: DefaultBaseURL, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Error is a non-2xx response. Body is kept for diagnostics; it never contains the token.
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string { return fmt.Sprintf("ticktick api: %d %s", e.Status, e.Body) }

// Backoff is the wait before each retry of a rate-limited or 5xx-overloaded request.
// TickTick allows 100 requests/minute and answers 500 "exceed_query_limit" beyond that.
var Backoff = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}

// Retryable reports whether err is rate limiting or a transient server error.
func Retryable(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	switch e.Status {
	case 429, 502, 503, 504:
		return true
	case 500:
		return strings.Contains(e.Body, "exceed_query_limit")
	}
	return false
}

// Do sends body (if non-nil) as JSON and decodes a JSON response into out (if non-nil).
// Rate-limited requests are retried with Backoff.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var b []byte
	if body != nil {
		var err error
		if b, err = json.Marshal(body); err != nil {
			return err
		}
	}
	for i := 0; ; i++ {
		err := c.do(ctx, method, path, b, out)
		if i >= len(Backoff) || !Retryable(err) {
			return err
		}
		select {
		case <-time.After(Backoff[i]):
		case <-ctx.Done():
			return err
		}
	}
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return &Error{Status: resp.StatusCode, Body: string(data)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	GroupID   string `json:"groupId"`
	Closed    bool   `json:"closed"`
	Kind      string `json:"kind"`
	ViewMode  string `json:"viewMode"`
	SortOrder int64  `json:"sortOrder"`
}

// Group is a folder of lists (GET /project/group).
type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int64  `json:"sortOrder"`
	ShowAll   bool   `json:"showAll"`
}

func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	var out []Group
	return out, c.Do(ctx, http.MethodGet, "/project/group", nil, &out)
}

type Task struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"projectId"`
	ParentID      string   `json:"parentId,omitempty"`
	Title         string   `json:"title"`
	Content       string   `json:"content,omitempty"`
	IsAllDay      bool     `json:"isAllDay,omitempty"`
	StartDate     string   `json:"startDate,omitempty"`
	DueDate       string   `json:"dueDate,omitempty"`
	TimeZone      string   `json:"timeZone,omitempty"`
	RepeatFlag    string   `json:"repeatFlag,omitempty"`
	Priority      int      `json:"priority"`
	Status        int      `json:"status"`
	CompletedTime string   `json:"completedTime,omitempty"`
	CreatedTime   string   `json:"createdTime,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Items         []Item   `json:"items,omitempty"`
}

type Item struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status int    `json:"status"`
}

type ProjectData struct {
	Project Project `json:"project"`
	Tasks   []Task  `json:"tasks"`
}

func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var out []Project
	return out, c.Do(ctx, http.MethodGet, "/project", nil, &out)
}

// OpenTasks returns every open task in every list (Inbox included) in one request.
// Cheaper than ProjectData per list under the 100 requests/minute limit.
func (c *Client) OpenTasks(ctx context.Context) ([]Task, error) {
	var out []Task
	return out, c.Do(ctx, http.MethodPost, "/task/filter", map[string]any{"status": []int{0}}, &out)
}

func (c *Client) ProjectData(ctx context.Context, projectID string) (*ProjectData, error) {
	var out ProjectData
	return &out, c.Do(ctx, http.MethodGet, "/project/"+projectID+"/data", nil, &out)
}

// Email returns the signed-in account's email: the "self" member of the Inbox.
// There is no /user endpoint; see docs/api-notes.md.
func (c *Client) Email(ctx context.Context) (string, error) {
	var ms []struct {
		Username string `json:"username"`
		Self     bool   `json:"self"`
	}
	if err := c.Do(ctx, http.MethodGet, "/project/inbox/members", nil, &ms); err != nil {
		return "", err
	}
	for _, m := range ms {
		if m.Self {
			return m.Username, nil
		}
	}
	return "", nil
}

// CompletedTasks returns tasks completed since since (all lists).
func (c *Client) CompletedTasks(ctx context.Context, since time.Time) ([]Task, error) {
	var out []Task
	body := map[string]any{"startDate": since.UTC().Format(DateLayout)}
	return out, c.Do(ctx, http.MethodPost, "/task/completed", body, &out)
}

// DateLayout is the API date format. Responses add milliseconds, which time.Parse accepts
// after the seconds field even though the layout doesn't mention them.
const DateLayout = "2006-01-02T15:04:05-0700"

// CreateTask creates t (ProjectID empty = Inbox) and returns the server's copy.
func (c *Client) CreateTask(ctx context.Context, t map[string]any) (*Task, error) {
	var out Task
	return &out, c.Do(ctx, http.MethodPost, "/task", t, &out)
}

// UpdateTask sends a partial update; fields not in f are kept by the server.
// dueDate: null clears it (and the repeat rule); "" is ignored. repeatFlag: "" clears it.
func (c *Client) UpdateTask(ctx context.Context, id, projectID string, f map[string]any) (*Task, error) {
	body := map[string]any{"id": id, "projectId": projectID}
	for k, v := range f {
		body[k] = v
	}
	var out Task
	return &out, c.Do(ctx, http.MethodPost, "/task/"+id, body, &out)
}

func (c *Client) CompleteTask(ctx context.Context, projectID, id string) error {
	return c.Do(ctx, http.MethodPost, "/project/"+projectID+"/task/"+id+"/complete", nil, nil)
}

func (c *Client) GetTask(ctx context.Context, projectID, id string) (*Task, error) {
	var out Task
	return &out, c.Do(ctx, http.MethodGet, "/project/"+projectID+"/task/"+id, nil, &out)
}
