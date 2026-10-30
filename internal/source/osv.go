package source

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"github.com/Arnav717/oryn/internal/detect"
)

var osvAPI = "https://api.osv.dev/v1"

type Vuln struct {
	ID       string
	Aliases  []string
	Summary  string
	Severity string // CRITICAL, HIGH, MEDIUM, LOW, UNKNOWN
	Fixed    string // first fixed version above the affected one, if known
	URL      string

	Ecosystem string
	Package   string
	Version   string
}

// CVE returns the first CVE alias, falling back to the OSV id.
func (v Vuln) CVE() string {
	for _, a := range v.Aliases {
		if strings.HasPrefix(a, "CVE-") {
			return a
		}
	}
	return v.ID
}

func SeverityRank(s string) int {
	switch s {
	case "CRITICAL":
		return 4
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	}
	return 0
}

type osvPkg struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvQuery struct {
	Package osvPkg `json:"package"`
	Version string `json:"version"`
}

type osvRecord struct {
	ID               string         `json:"id"`
	Summary          string         `json:"summary"`
	Details          string         `json:"details"`
	Aliases          []string       `json:"aliases"`
	DatabaseSpecific map[string]any `json:"database_specific"`
	Affected         []struct {
		Package osvPkg `json:"package"`
		Ranges  []struct {
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

const osvBatchMax = 1000

var (
	osvRecords = &diskCache[osvRecord]{name: "cache-osv.json", ttl: 7 * 24 * time.Hour}
	osvMatches = &diskCache[map[string][]int]{name: "cache-osv-matches.json", ttl: 10 * time.Minute}
)

// matchOSV asks which vulnerabilities affect these exact versions, returning
// vuln id -> query indexes. Sending every package is slow, so an answer is reused
// briefly. Any lockfile change alters the key and asks again.
func matchOSV(ctx context.Context, queries []osvQuery) (map[string][]int, error) {
	h := sha256.New()
	for _, q := range queries {
		fmt.Fprintln(h, q.Package.Ecosystem, q.Package.Name, q.Version)
	}
	key := hex.EncodeToString(h.Sum(nil))
	now := time.Now()
	cached, found, fresh := osvMatches.get(key, now)
	if fresh {
		return cached, nil
	}

	ids := map[string][]int{}
	for start := 0; start < len(queries); start += osvBatchMax {
		end := min(start+osvBatchMax, len(queries))
		var resp struct {
			Results []struct {
				Vulns []struct {
					ID string `json:"id"`
				} `json:"vulns"`
			} `json:"results"`
		}
		body := map[string]any{"queries": queries[start:end]}
		if err := doJSON(ctx, "POST", osvAPI+"/querybatch", body, &resp); err != nil {
			if found {
				return cached, nil
			}
			return nil, err
		}
		for i, r := range resp.Results {
			for _, v := range r.Vulns {
				ids[v.ID] = append(ids[v.ID], start+i)
			}
		}
	}
	osvMatches.replace(key, ids, now)
	osvMatches.save()
	return ids, nil
}

// trimDetails keeps what the summary falls back on, not whole advisories.
func trimDetails(s string) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

// QueryOSV returns known vulnerabilities for each dep, keyed by DepKey.
func QueryOSV(ctx context.Context, deps []detect.Dep) (map[string][]Vuln, error) {
	var queries []osvQuery
	var keys []string
	seen := map[string]bool{}
	for _, d := range deps {
		k := DepKey(d)
		if d.Version == "" || seen[k] {
			continue
		}
		seen[k] = true
		queries = append(queries, osvQuery{osvPkg{d.Name, d.Ecosystem}, osvVersion(d)})
		keys = append(keys, k)
	}

	ids, err := matchOSV(ctx, queries)
	if err != nil {
		return nil, err
	}

	idList := make([]string, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	// A record's details barely change, and fetching them is the slow part.
	now := time.Now()
	records := map[string]osvRecord{}
	var stale []string
	var missing []string
	for _, id := range idList {
		rec, found, fresh := osvRecords.get(id, now)
		if found {
			records[id] = rec
		}
		if !fresh {
			stale = append(stale, id)
		}
		if !found {
			missing = append(missing, id)
		}
	}
	var mu sync.Mutex
	var firstErr error
	forEach(stale, 16, func(id string) {
		var rec osvRecord
		err := getJSON(ctx, osvAPI+"/vulns/"+id, &rec)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			// An old copy is better than none, so only a gap is an error.
			if firstErr == nil && slices.Contains(missing, id) {
				firstErr = err
			}
			return
		}
		rec.Details = trimDetails(rec.Details)
		records[id] = rec
		osvRecords.put(id, rec, now)
	})
	if len(stale) > 0 {
		osvRecords.save() // only a cache: a failed write just means fetching again
	}

	out := map[string][]Vuln{}
	for id, idxs := range ids {
		rec, ok := records[id]
		if !ok {
			continue
		}
		for _, i := range idxs {
			q := queries[i]
			out[keys[i]] = append(out[keys[i]], Vuln{
				ID:        rec.ID,
				Aliases:   rec.Aliases,
				Summary:   summary(rec),
				Severity:  severity(rec),
				Fixed:     fixedVersion(rec, q),
				URL:       "https://osv.dev/vulnerability/" + rec.ID,
				Ecosystem: q.Package.Ecosystem,
				Package:   q.Package.Name,
				Version:   q.Version,
			})
		}
	}
	for k, vs := range out {
		out[k] = dedupe(vs)
	}
	return out, firstErr
}

// dedupe collapses records describing the same CVE (e.g. a GO- and a GHSA-
// advisory), keeping the one with the most specific severity.
func dedupe(vs []Vuln) []Vuln {
	best := map[string]int{}
	var out []Vuln
	for _, v := range vs {
		i, ok := best[v.CVE()]
		if !ok {
			best[v.CVE()] = len(out)
			out = append(out, v)
			continue
		}
		if SeverityRank(v.Severity) > SeverityRank(out[i].Severity) {
			out[i] = v
		}
	}
	return out
}

func DepKey(d detect.Dep) string { return d.Ecosystem + "/" + d.Name + "@" + osvVersion(d) }

func PkgKey(ecosystem, name string) string { return ecosystem + "/" + name }

// osvVersion normalizes versions to what OSV expects (Go versions without "v").
func osvVersion(d detect.Dep) string {
	if d.Ecosystem == detect.Go {
		return strings.TrimPrefix(d.Version, "v")
	}
	return d.Version
}

func summary(r osvRecord) string {
	if r.Summary != "" {
		return r.Summary
	}
	s := strings.TrimSpace(r.Details)
	if i := strings.IndexAny(s, "\n."); i > 0 {
		s = s[:i]
	}
	return s
}

func severity(r osvRecord) string {
	s, _ := r.DatabaseSpecific["severity"].(string)
	switch s = strings.ToUpper(s); s {
	case "MODERATE":
		return "MEDIUM"
	case "CRITICAL", "HIGH", "MEDIUM", "LOW":
		return s
	}
	return "UNKNOWN"
}

// fixedVersion picks the lowest fixed version greater than the queried one.
func fixedVersion(r osvRecord, q osvQuery) string {
	cur := "v" + q.Version
	var best string
	for _, a := range r.Affected {
		if a.Package.Name != q.Package.Name {
			continue
		}
		for _, rg := range a.Ranges {
			for _, ev := range rg.Events {
				f, ok := ev["fixed"]
				if !ok {
					continue
				}
				fv := "v" + strings.TrimPrefix(f, "v")
				if semver.IsValid(cur) && semver.IsValid(fv) && semver.Compare(fv, cur) <= 0 {
					continue
				}
				if best == "" || (semver.IsValid(fv) && semver.Compare(fv, "v"+best) < 0) {
					best = strings.TrimPrefix(f, "v")
				}
			}
		}
	}
	return best
}
