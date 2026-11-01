package geo

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Where the databases come from. DB-IP publishes a new Lite file each month
// (dbip-city-lite-2026-11.mmdb.gz). When its site can't be reached, the same
// data comes from the ip-location-db packages on npm's registry, which most
// networks allow; they're published a little less often.
var (
	dbipBase = envOr("NETMON_DBIP_URL", "https://download.db-ip.com/free")
	npmBase  = envOr("NETMON_NPM_URL", "https://registry.npmjs.org")
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return strings.TrimRight(v, "/")
	}
	return def
}

var client = &http.Client{Timeout: 15 * time.Minute}

type kind struct {
	label    string // shown while downloading
	dbip     string // "city" or "asn"
	npmPkg   string
	npmFiles map[string]string // file in the package -> local name
	prefix   string            // local file names start with this
}

var (
	cityKind = kind{
		label:    "Location database",
		dbip:     "city",
		npmPkg:   "@ip-location-db/dbip-city-mmdb",
		npmFiles: map[string]string{"dbip-city-ipv4.mmdb": "city-v4.mmdb", "dbip-city-ipv6.mmdb": "city-v6.mmdb"},
		prefix:   "city",
	}
	asnKind = kind{
		label:    "Network database",
		dbip:     "asn",
		npmPkg:   "@ip-location-db/dbip-asn-mmdb",
		npmFiles: map[string]string{"dbip-asn.mmdb": "asn.mmdb"},
		prefix:   "asn",
	}
)

// Update downloads the newest databases when they're newer than the ones
// there, and swaps them in. It reports its progress in Status. force
// downloads even when the version is the same.
func (d *DB) Update(ctx context.Context, force bool) error {
	d.smu.Lock()
	if d.status.Downloading {
		d.smu.Unlock()
		return errBusy
	}
	d.status.Downloading = true
	d.status.Error = ""
	d.smu.Unlock()

	err := d.update(ctx, force)
	d.setStatus(func(s *Status) {
		s.Downloading = false
		s.Stage = ""
		s.Done, s.Total = 0, 0
		if err != nil {
			s.Error = err.Error()
		}
	})
	d.refreshStatus()
	return err
}

func (d *DB) update(ctx context.Context, force bool) error {
	tmp, err := os.MkdirTemp(d.dir, ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	d.mu.RLock()
	old := d.meta
	d.mu.RUnlock()

	city, cityErr := d.fetch(ctx, cityKind, tmp, old.City, force)
	asn, asnErr := d.fetch(ctx, asnKind, tmp, old.ASN, force)
	if cityErr != nil && asnErr != nil && (city == nil && asn == nil) {
		return cityErr
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if city != nil {
		closeAll(d.city)
		d.city = nil
		if err := swapIn(d.dir, tmp, old.City.Files, city.Files); err != nil {
			d.city = openAll(d.dir, old.City.Files)
			return err
		}
		d.meta.City = *city
		d.city = openAll(d.dir, city.Files)
	} else if cityErr == nil {
		d.meta.City.Checked = now
	}
	if asn != nil {
		closeAll(d.asn)
		d.asn = nil
		if err := swapIn(d.dir, tmp, old.ASN.Files, asn.Files); err != nil {
			d.asn = openAll(d.dir, old.ASN.Files)
			return err
		}
		d.meta.ASN = *asn
		d.asn = openAll(d.dir, asn.Files)
	} else if asnErr == nil {
		d.meta.ASN.Checked = now
	}
	d.cache = map[netip.Addr]Location{}
	data, _ := json.MarshalIndent(d.meta, "", "  ")
	if err := os.WriteFile(filepath.Join(d.dir, "meta.json"), data, 0o644); err != nil {
		return err
	}
	if cityErr != nil {
		return cityErr
	}
	return asnErr
}

// swapIn moves the new files from tmp into dir and removes old ones that
// aren't replaced. The readers of the old files must be closed (Windows
// doesn't replace a file that is open).
func swapIn(dir, tmp string, oldFiles, newFiles []string) error {
	keep := map[string]bool{}
	for _, f := range newFiles {
		if err := os.Rename(filepath.Join(tmp, f), filepath.Join(dir, f)); err != nil {
			return err
		}
		keep[f] = true
	}
	for _, f := range oldFiles {
		if !keep[f] && f == filepath.Base(f) {
			_ = os.Remove(filepath.Join(dir, f))
		}
	}
	return nil
}

// fetch downloads one database into tmp. It returns nil (and no error)
// when the one there is already the newest.
func (d *DB) fetch(ctx context.Context, k kind, tmp string, have Source, force bool) (*Source, error) {
	src, errDBIP := d.fetchDBIP(ctx, k, tmp, have, force)
	if errDBIP == nil {
		return src, nil
	}
	src, errNPM := d.fetchNPM(ctx, k, tmp, have, force)
	if errNPM == nil {
		return src, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("couldn't download the %s: %v; %v", strings.ToLower(k.label), errDBIP, errNPM)
}

func (d *DB) fetchDBIP(ctx context.Context, k kind, tmp string, have Source, force bool) (*Source, error) {
	now := time.Now().UTC()
	var lastErr error
	// This month's file appears a day or two into the month.
	for _, t := range []time.Time{now, now.AddDate(0, -1, 0), now.AddDate(0, -2, 0)} {
		version := t.Format("2006-01")
		if !force && have.From == "db-ip.com" && have.Version == version {
			return nil, nil
		}
		if !force && have.From == "db-ip.com" && have.Version > version {
			return nil, nil
		}
		url := fmt.Sprintf("%s/dbip-%s-lite-%s.mmdb.gz", dbipBase, k.dbip, version)
		name := k.prefix + ".mmdb"
		err := d.download(ctx, k.label, url, func(body io.Reader) error {
			zr, err := gzip.NewReader(body)
			if err != nil {
				return err
			}
			return writeFile(filepath.Join(tmp, name), zr)
		}, nil)
		if err == nil {
			if err := validate(filepath.Join(tmp, name)); err != nil {
				return nil, err
			}
			return &Source{From: "db-ip.com", Version: version, Files: []string{name}, Downloaded: time.Now(), Checked: time.Now()}, nil
		}
		lastErr = err
		var he httpError
		if !errors.As(err, &he) || he.code != http.StatusNotFound {
			break // the site can't be reached: no use trying older months
		}
	}
	return nil, fmt.Errorf("db-ip.com: %w", lastErr)
}

func (d *DB) fetchNPM(ctx context.Context, k kind, tmp string, have Source, force bool) (*Source, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, npmBase+"/"+k.npmPkg+"/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("npm: %w", err)
	}
	var info struct {
		Version string `json:"version"`
		Dist    struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm: %s", resp.Status)
	}
	if err != nil || info.Version == "" || info.Dist.Tarball == "" {
		return nil, fmt.Errorf("npm: unexpected answer for %s", k.npmPkg)
	}
	if !force && have.From == "npm" && have.Version == info.Version {
		return nil, nil
	}
	// A DB-IP file of the same month or newer is at least as new.
	if !force && have.From == "db-ip.com" && len(info.Version) >= 12 {
		// 2.3.2026060513 -> 2026-06
		v := info.Version[strings.LastIndex(info.Version, ".")+1:]
		if len(v) >= 6 && have.Version >= v[:4]+"-"+v[4:6] {
			return nil, nil
		}
	}
	var sum hash.Hash
	want := ""
	if alg, b64, ok := strings.Cut(info.Dist.Integrity, "-"); ok && alg == "sha512" {
		sum, want = sha512.New(), b64
	}
	got := 0
	err = d.download(ctx, k.label, info.Dist.Tarball, func(body io.Reader) error {
		zr, err := gzip.NewReader(body)
		if err != nil {
			return err
		}
		tr := tar.NewReader(zr)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			local, ok := k.npmFiles[path.Base(h.Name)]
			if !ok {
				continue
			}
			if err := writeFile(filepath.Join(tmp, local), tr); err != nil {
				return err
			}
			got++
		}
		// Read the rest, so the checksum covers the whole file.
		_, err = io.Copy(io.Discard, zr)
		return err
	}, sum)
	if err != nil {
		return nil, fmt.Errorf("npm: %w", err)
	}
	if sum != nil && base64.StdEncoding.EncodeToString(sum.Sum(nil)) != want {
		return nil, errors.New("npm: the download is damaged (checksum)")
	}
	if got != len(k.npmFiles) {
		return nil, fmt.Errorf("npm: %s is missing files", k.npmPkg)
	}
	var files []string
	for _, local := range sortedValues(k.npmFiles) {
		if err := validate(filepath.Join(tmp, local)); err != nil {
			return nil, err
		}
		files = append(files, local)
	}
	return &Source{From: "npm", Version: info.Version, Files: files, Downloaded: time.Now(), Checked: time.Now()}, nil
}

func sortedValues(m map[string]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	// city-v4 before city-v6: most addresses are IPv4.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

type httpError struct {
	code   int
	status string
}

func (e httpError) Error() string { return e.status }

// download GETs url and hands the body to read, counting progress (and
// feeding sum, when given).
func (d *DB) download(ctx context.Context, label, url string, read func(io.Reader) error, sum hash.Hash) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "NetworkMonitor")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpError{resp.StatusCode, resp.Status}
	}
	d.setStatus(func(s *Status) {
		s.Stage = label
		s.Done, s.Total = 0, resp.ContentLength
	})
	var body io.Reader = &progressReader{r: resp.Body, d: d}
	if sum != nil {
		body = io.TeeReader(body, sum)
	}
	return read(body)
}

type progressReader struct {
	r    io.Reader
	d    *DB
	done int64
	last time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if time.Since(p.last) > 200*time.Millisecond || err != nil {
		p.last = time.Now()
		done := p.done
		p.d.setStatus(func(s *Status) { s.Done = done })
	}
	return n, err
}

// writeFile writes r to a new file (at most 1 GB: a database is far smaller).
func writeFile(name string, r io.Reader) error {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(r, 1<<30))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// validate opens a downloaded database and looks a well-known address up.
func validate(name string) error {
	r, err := maxminddb.Open(name)
	if err != nil {
		return fmt.Errorf("the downloaded database doesn't open: %w", err)
	}
	defer r.Close()
	if r.Metadata.NodeCount == 0 {
		return errors.New("the downloaded database is empty")
	}
	return nil
}
