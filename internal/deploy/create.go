package deploy

import (
	"context"
	"errors"
	"net/url"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Arnav717/oryn/internal/source"
)

// Creator can make an empty project on the platform, for a first deploy.
type Creator interface {
	Create(ctx context.Context, name string) (ProjectRef, error)
}

var nameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,50}[a-z0-9])?$`)

// ValidName reports whether name is acceptable on all three platforms:
// lowercase letters, digits and inner hyphens, up to 52 characters.
func ValidName(name string) bool { return nameRE.MatchString(name) }

// SuggestName turns a folder name into a valid project name.
func SuggestName(folder string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(folder) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 52 {
		s = strings.Trim(s[:52], "-")
	}
	return s
}

func (v *Vercel) Create(ctx context.Context, name string) (ProjectRef, error) {
	tok := vercelToken()
	if tok == "" {
		return ProjectRef{}, ErrNoCredentials
	}
	var r struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		AccountID string `json:"accountId"`
	}
	err := source.DoJSONH(ctx, "POST", v.base+"/v11/projects", bearer(tok), map[string]string{"name": name}, &r)
	if err != nil {
		return ProjectRef{}, err
	}
	return ProjectRef{Name: r.Name, Ref: r.ID, Team: r.AccountID}, nil
}

func (n *Netlify) Create(ctx context.Context, name string) (ProjectRef, error) {
	tok := netlifyToken()
	if tok == "" {
		return ProjectRef{}, ErrNoCredentials
	}
	var r struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := source.DoJSONH(ctx, "POST", n.base+"/api/v1/sites", bearer(tok), map[string]string{"name": name}, &r); err != nil {
		return ProjectRef{}, err
	}
	return ProjectRef{Name: r.Name, Ref: r.ID}, nil
}

func (c *Cloudflare) Create(ctx context.Context, name string) (ProjectRef, error) {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return ProjectRef{}, ErrNoCredentials
	}
	var r struct {
		Result struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	u := c.base + "/client/v4/accounts/" + url.PathEscape(acc) + "/pages/projects"
	body := map[string]string{"name": name, "production_branch": "main"}
	if err := source.DoJSONH(ctx, "POST", u, bearer(tok), body, &r); err != nil {
		return ProjectRef{}, err
	}
	return ProjectRef{Name: r.Result.Name, Ref: r.Result.Name}, nil
}

// CreateProject creates a project and survives a dropped connection: a
// transport error doesn't say whether the platform processed the request, so
// it checks for the project before retrying once. HTTP errors (name taken,
// bad token) are returned as they are.
func CreateProject(ctx context.Context, platform, name string) (ProjectRef, error) {
	cn := ConnectorFor(platform)
	cr, _ := cn.(Creator)
	if cr == nil {
		return ProjectRef{}, errors.New("cannot create projects on " + platform)
	}
	return createWith(ctx, cn, cr, name)
}

func createWith(ctx context.Context, cn Connector, cr Creator, name string) (ProjectRef, error) {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var ref ProjectRef
		if ref, err = cr.Create(ctx, name); err == nil {
			return ref, nil
		}
		var netErr *neturl.Error
		if !errors.As(err, &netErr) {
			return ProjectRef{}, err
		}
		// Did it get created anyway?
		if ps, lerr := cn.Projects(ctx); lerr == nil {
			for _, p := range ps {
				if p.Name == name {
					return p, nil
				}
			}
		}
	}
	return ProjectRef{}, err
}

// CreatorFor returns the platform's Creator, or nil.
func CreatorFor(platform string) Creator {
	c, _ := ConnectorFor(platform).(Creator)
	return c
}

// Command is a platform CLI invocation that publishes a project.
type Command struct {
	Bin     string
	Args    []string
	Env     []string // extra environment for the child only
	Dir     string
	Install string // how to get the CLI when it's missing
}

// Display is the command as shown in the confirm prompt. It never includes
// secrets: tokens travel in Env.
func (c Command) Display() string {
	return "cd " + c.Dir + " && " + c.Bin + " " + strings.Join(c.Args, " ")
}

// Installed reports whether the CLI is on PATH.
func (c Command) Installed() bool {
	_, err := exec.LookPath(c.Bin)
	return err == nil
}

// Cmd builds the runnable command. On Unix it is wrapped so the terminal
// stays on the CLI's output until Enter is pressed: oryn redraws its own
// screen the moment the process exits, which would wipe the build log.
func (c Command) Cmd() *exec.Cmd {
	cmd := exec.Command(c.Bin, c.Args...)
	if runtime.GOOS != "windows" {
		cmd = exec.Command("sh", "-c",
			`"$@"; code=$?; printf '\n[oryn] %s exited with status %s. Press Enter to return to oryn. ' "$1" "$code"; read _; exit $code`,
			"sh", c.Bin)
		cmd.Args = append(cmd.Args, c.Args...)
	}
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	return cmd
}

// outputDirs are the usual static build folders, tried in order for Cloudflare Pages.
var outputDirs = []string{"dist", "build", "out", "public", ".output/public"}

// DeployCommand returns how to publish the linked project to production with
// the platform's own CLI, which handles building and uploading.
func DeployCommand(l Link, dir string) (Command, error) {
	if !l.Linked() {
		return Command{}, errors.New("project is not linked to a " + l.Platform + " project yet")
	}
	switch l.Platform {
	case "vercel":
		if l.Team == "" {
			return Command{}, errors.New("this link has no account id; link the project again")
		}
		return Command{
			Bin: "vercel", Args: []string{"--prod", "--yes"}, Dir: dir, Install: "npm i -g vercel",
			Env: []string{"VERCEL_PROJECT_ID=" + l.Ref, "VERCEL_ORG_ID=" + l.Team, "VERCEL_TOKEN=" + vercelToken()},
		}, nil
	case "netlify":
		return Command{
			Bin: "netlify", Args: []string{"deploy", "--prod", "--build", "--site", l.Ref}, Dir: dir, Install: "npm i -g netlify-cli",
			Env: []string{"NETLIFY_AUTH_TOKEN=" + netlifyToken()},
		}, nil
	case "cloudflare":
		out := ""
		for _, d := range outputDirs {
			if fi, err := os.Stat(filepath.Join(dir, d)); err == nil && fi.IsDir() {
				out = d
				break
			}
		}
		if out == "" {
			return Command{}, errors.New("no build output folder (dist, build, out, public) found; build the site first")
		}
		tok, acc := cloudflareCreds()
		return Command{
			Bin: "wrangler", Args: []string{"pages", "deploy", out, "--project-name", l.Ref}, Dir: dir, Install: "npm i -g wrangler",
			Env: []string{"CLOUDFLARE_API_TOKEN=" + tok, "CLOUDFLARE_ACCOUNT_ID=" + acc},
		}, nil
	}
	return Command{}, errors.New("cannot deploy to " + l.Platform)
}
