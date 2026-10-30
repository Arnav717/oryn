package source

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Arnav717/oryn/internal/gitrepo"
)

func repoAPI(ref gitrepo.Ref) string {
	return fmt.Sprintf("%s/repos/%s/%s", githubAPI, ref.Owner, ref.Name)
}

type job struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Steps      []struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
	} `json:"steps"`
}

func (j job) failing() bool { return j.Conclusion == "failure" || j.Conclusion == "timed_out" }

func fetchJobs(ctx context.Context, h map[string]string, base string, runID int64) ([]job, error) {
	var r struct {
		Jobs []job `json:"jobs"`
	}
	err := doJSONH(ctx, "GET", fmt.Sprintf("%s/actions/runs/%d/jobs?filter=latest&per_page=100", base, runID), h, nil, &r)
	return r.Jobs, err
}

// findFailure explains a red default branch. Only a run of that exact commit
// counts: a fork's pull request can name its head branch main and write its
// own job names, and those mustn't pass for the project's. The job lookup is
// best-effort, since the run alone still says where to read the log.
func findFailure(ctx context.Context, h map[string]string, base string, d *RepoData) *CIFailure {
	if d.DefaultCI != "FAILURE" && d.DefaultCI != "ERROR" || d.HeadSHA == "" {
		return nil
	}
	for _, r := range d.Runs {
		if !r.Failing() || r.HeadSHA != d.HeadSHA || r.Branch != d.DefaultBranch {
			continue
		}
		f := &CIFailure{RunID: r.ID, Workflow: r.Workflow}
		jobs, err := fetchJobs(ctx, h, base, r.ID)
		if err != nil {
			return f
		}
		for _, j := range jobs {
			if !j.failing() {
				continue
			}
			f.Job = j.Name
			for _, s := range j.Steps {
				if s.Conclusion == "failure" || s.Conclusion == "timed_out" {
					f.Step = s.Name
					break
				}
			}
			break
		}
		return f
	}
	return nil
}

const logTail = 1000 // lines of a job's log kept, counted from the end

var logStamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T[0-9:.]+Z `)

// RunLog is the tail of a run's first failed job, or its first finished one.
// GitHub only serves a job's log once the job ends, so live means ask again.
func RunLog(ctx context.Context, ref gitrepo.Ref, runID int64) (lines []string, live bool, err error) {
	token := GitHubToken()
	if token == "" {
		return nil, false, ErrNoToken
	}
	h := authHeaders(token)
	jobs, err := fetchJobs(ctx, h, repoAPI(ref), runID)
	if err != nil {
		return nil, false, err
	}
	var pick *job
	for i, j := range jobs {
		if j.Status != "completed" {
			live = true
			continue
		}
		if pick == nil || j.failing() && !pick.failing() {
			pick = &jobs[i]
		}
	}
	if pick == nil {
		return nil, live, nil
	}
	lines, err = jobLog(ctx, token, fmt.Sprintf("%s/actions/jobs/%d/logs", repoAPI(ref), pick.ID))
	return append([]string{"job: " + pick.Name}, lines...), live, err
}

// jobLog reads only the end of a log, which can run to many megabytes. The
// download redirects to a signed URL, so errors leave the URL out.
func jobLog(ctx context.Context, token, u string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("reading the log: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reading the log: %s", resp.Status)
	}
	var out []string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		out = append(out, logStamp.ReplaceAllString(strings.TrimPrefix(sc.Text(), "\ufeff"), ""))
		if len(out) > 2*logTail {
			out = append(out[:0], out[len(out)-logTail:]...)
		}
	}
	return out[max(0, len(out)-logTail):], nil
}
