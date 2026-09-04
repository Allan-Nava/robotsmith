// Command backlog-sync projects BACKLOG.md onto GitHub issues, idempotently.
//
//	backlog-sync --repo Allan-Nava/robotsmith            # dry run: prints the plan, changes nothing
//	backlog-sync --repo Allan-Nava/robotsmith --apply    # applies it (needs GITHUB_TOKEN)
//
// Why the file is the source and the issues the projection: the backlog lives next to the code, in
// the same review and the same commit. Issues can be recreated from the file at any time; the file
// cannot be reconstructed from the issues.
//
// ⚠️ Dry run is the DEFAULT. A tool that writes to a tracker by accident gets run once and never
// again.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Allan-Nava/robotsmith/internal/backlog"
)

func main() {
	fs := flag.NewFlagSet("backlog-sync", flag.ContinueOnError)
	file := fs.String("file", "BACKLOG.md", "the backlog to project")
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name")
	apply := fs.Bool("apply", false, "actually write to GitHub (default: print the plan only)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *repo == "" {
		fmt.Fprintln(os.Stderr, "--repo owner/name is required (or set GITHUB_REPOSITORY)")
		os.Exit(2)
	}

	md, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	items, err := backlog.Parse(string(md))
	if err != nil {
		fmt.Fprintln(os.Stderr, *file+":", err)
		os.Exit(1)
	}
	if probs := backlog.Lint(items, string(md)); len(probs) > 0 {
		// ⚠️ Never sync a file that does not lint: a typo in a milestone title would create a
		// duplicate milestone, and a duplicate id would fight over one issue forever.
		for _, p := range probs {
			fmt.Fprintln(os.Stderr, *file+":", p)
		}
		os.Exit(1)
	}

	gh := &client{repo: *repo, token: os.Getenv("GITHUB_TOKEN"), dry: !*apply}
	existing, err := gh.ownedIssues()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	plan := backlog.Plan(items, existing)
	var acted int
	for _, a := range plan {
		if a.Kind == backlog.Skip {
			continue
		}
		acted++
		fmt.Printf("%-6s %-34s #%-4d %s\n", a.Kind, a.IssueID, a.Number, a.Why)
		if !*apply {
			continue
		}
		if err := gh.do(a); err != nil {
			fmt.Fprintf(os.Stderr, "%s %s: %v\n", a.Kind, a.IssueID, err)
			os.Exit(1)
		}
	}
	fmt.Printf("\n%d items · %d issues owned · %d changes%s\n",
		len(items), len(existing), acted, map[bool]string{true: " (dry run: nothing written)"}[!*apply])
}

// client is a minimal GitHub REST client: the repo has no dependencies outside the stdlib, and this
// needs six calls.
type client struct {
	repo  string
	token string
	dry   bool
	mile  map[string]int
}

func (c *client) call(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "https://api.github.com"+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

type apiIssue struct {
	Number    int                     `json:"number"`
	Title     string                  `json:"title"`
	Body      string                  `json:"body"`
	State     string                  `json:"state"`
	Labels    []struct{ Name string } `json:"labels"`
	Milestone *struct{ Title string } `json:"milestone"`
}

// ownedIssues lists only the issues this projection owns — the label is the filter, the fingerprint
// in the body is the proof.
func (c *client) ownedIssues() ([]backlog.Issue, error) {
	var out []backlog.Issue
	for page := 1; ; page++ {
		var batch []apiIssue
		path := fmt.Sprintf("/repos/%s/issues?state=all&labels=%s&per_page=100&page=%d",
			c.repo, backlog.SyncLabel, page)
		if err := c.call("GET", path, nil, &batch); err != nil {
			return nil, err
		}
		for _, is := range batch {
			conv := backlog.Issue{Number: is.Number, Title: is.Title, Body: is.Body, State: is.State,
				ID: backlog.IDFromBody(is.Body)}
			for _, l := range is.Labels {
				conv.Labels = append(conv.Labels, l.Name)
			}
			if is.Milestone != nil {
				conv.Milestone = is.Milestone.Title
			}
			out = append(out, conv)
		}
		if len(batch) < 100 {
			return out, nil
		}
	}
}

// milestoneNumber resolves a milestone by EXACT title, creating it when missing. Exact matching is
// why the backlog lint refuses a milestone title that has no section heading: a typo here would
// quietly create a second milestone.
func (c *client) milestoneNumber(title string) (int, error) {
	if title == "" {
		return 0, nil
	}
	if c.mile == nil {
		c.mile = map[string]int{}
		var ms []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
		}
		if err := c.call("GET", "/repos/"+c.repo+"/milestones?state=all&per_page=100", nil, &ms); err != nil {
			return 0, err
		}
		for _, m := range ms {
			c.mile[m.Title] = m.Number
		}
	}
	if n, ok := c.mile[title]; ok {
		return n, nil
	}
	var created struct {
		Number int `json:"number"`
	}
	if err := c.call("POST", "/repos/"+c.repo+"/milestones",
		map[string]any{"title": title}, &created); err != nil {
		return 0, err
	}
	c.mile[title] = created.Number
	return created.Number, nil
}

func (c *client) do(a backlog.Action) error {
	switch a.Kind {
	case backlog.Create, backlog.Update, backlog.Reopen:
		mn, err := c.milestoneNumber(a.Item.Milestone)
		if err != nil {
			return err
		}
		payload := map[string]any{
			"title":  a.Item.Title,
			"body":   backlog.RenderBody(a.Item),
			"labels": backlog.Labels(a.Item),
			"state":  "open",
		}
		if mn > 0 {
			payload["milestone"] = mn
		}
		if a.Kind == backlog.Create {
			return c.call("POST", "/repos/"+c.repo+"/issues", payload, nil)
		}
		return c.call("PATCH", fmt.Sprintf("/repos/%s/issues/%d", c.repo, a.Number), payload, nil)
	case backlog.Close:
		// The comment is the point: a closed issue with no explanation looks like someone tidying up.
		if err := c.call("POST", fmt.Sprintf("/repos/%s/issues/%d/comments", c.repo, a.Number),
			map[string]any{"body": "Closed by the backlog sync: " + a.Why + " (`" + a.IssueID + "`)."},
			nil); err != nil {
			return err
		}
		return c.call("PATCH", fmt.Sprintf("/repos/%s/issues/%d", c.repo, a.Number),
			map[string]any{"state": "closed", "state_reason": "completed"}, nil)
	}
	return nil
}
