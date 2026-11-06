package cookiejar

import (
	"cmp"
	"net/url"
	"slices"
	"time"
)

// Entry is one stored cookie as the jar holds it. Domain and Path are the entry's own, which with
// Name form the id Delete takes.
type Entry struct {
	Name       string
	Value      string
	Domain     string
	Path       string
	SameSite   string
	Secure     bool
	HttpOnly   bool
	HostOnly   bool
	Persistent bool
	Expires    time.Time
}

// Entries lists what Cookies would send to u, in the same order, without touching LastAccess or
// deleting expired entries.
func (j *Jar) Entries(u *url.URL) []Entry {
	return j.entriesAt(u, time.Now())
}

func (j *Jar) entriesAt(u *url.URL, now time.Time) []Entry {
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}
	host, err := canonicalHost(u.Host)
	if err != nil {
		return nil
	}
	key := jarKey(host, j.psList)

	j.mu.Lock()
	defer j.mu.Unlock()

	https := u.Scheme == "https"
	path := u.Path
	if path == "" {
		path = "/"
	}

	var selected []entry
	for _, e := range j.entries[key] {
		if e.Persistent && !e.Expires.After(now) {
			continue
		}
		if e.shouldSend(https, host, path) {
			selected = append(selected, e)
		}
	}
	slices.SortFunc(selected, func(a, b entry) int {
		if r := cmp.Compare(b.Path, a.Path); r != 0 {
			return r
		}
		if r := a.Creation.Compare(b.Creation); r != 0 {
			return r
		}
		return cmp.Compare(a.seqNum, b.seqNum)
	})
	out := make([]Entry, 0, len(selected))
	for _, e := range selected {
		out = append(out, Entry{
			Name: e.Name, Value: e.Value, Domain: e.Domain, Path: e.Path, SameSite: e.SameSite,
			Secure: e.Secure, HttpOnly: e.HttpOnly, HostOnly: e.HostOnly,
			Persistent: e.Persistent, Expires: e.Expires,
		})
	}
	return out
}

// Delete removes the entry with exactly this domain, path and name (an Entry's own fields) and
// reports whether one existed.
func (j *Jar) Delete(domain, path, name string) bool {
	key := jarKey(domain, j.psList)
	id := domain + ";" + path + ";" + name

	j.mu.Lock()
	defer j.mu.Unlock()

	submap := j.entries[key]
	if _, ok := submap[id]; !ok {
		return false
	}
	delete(submap, id)
	if len(submap) == 0 {
		delete(j.entries, key)
	}
	return true
}
