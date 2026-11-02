package sessionrecorder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"golang.org/x/sync/singleflight"
)

const (
	// MaxContentBodyBytes is the most of one body returned to the console.
	MaxContentBodyBytes = 2 << 20
	// maxArchiveEntryBytes rejects a body entry larger than any capture the
	// producer can write; such an archive is not trusted.
	maxArchiveEntryBytes = 64 << 20
	// maxArchiveExpandedBytes bounds the decompressed bytes read from one
	// archive (gzip bomb protection). Batches are capped at 2 GiB of input
	// by default.
	maxArchiveExpandedBytes = 16 << 30
	// maxArchiveDownloadBytes bounds one archive download.
	maxArchiveDownloadBytes = 8 << 30

	contentCacheDirName = "content-cache"
	contentCacheTTL     = 10 * time.Minute
	contentCacheMaxSize = 4 << 30
)

var (
	// ErrRecordContentUnavailable means neither the spool nor R2 has the record.
	ErrRecordContentUnavailable = common.NewMessage("Recorded content is no longer available")
	// ErrArchiveStoreUnavailable means R2 cannot be read with the current settings.
	ErrArchiveStoreUnavailable = common.NewMessage("Session recorder archive storage is not configured")

	errUnsafeArchive  = errors.New("archive contains an unsafe entry")
	errArchiveTooBig  = errors.New("archive expands beyond the allowed size")
	errStopArchive    = errors.New("stop archive walk")
	errRecordNotFound = errors.New("record not found")
)

// ArchiveGetter reads archive objects from R2. *R2Store implements it.
type ArchiveGetter interface {
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
}

type expandLimitReader struct {
	r    io.Reader
	left int64
}

func (l *expandLimitReader) Read(p []byte) (int, error) {
	if l.left <= 0 {
		return 0, errArchiveTooBig
	}
	if int64(len(p)) > l.left {
		p = p[:l.left]
	}
	n, err := l.r.Read(p)
	l.left -= int64(n)
	return n, err
}

// archiveEntryPath validates a tar entry name. It returns the root id and the
// spool relative path of a source entry, or ok=false for the trailing
// manifest. Anything else is an unsafe entry.
func archiveEntryPath(name string) (rootID, rel string, ok bool, err error) {
	if name == ArchiveManifestName {
		return "", "", false, nil
	}
	if strings.Contains(name, "\\") || strings.Contains(name, "\x00") || path.IsAbs(name) || path.Clean(name) != name {
		return "", "", false, errUnsafeArchive
	}
	rest, found := strings.CutPrefix(name, "roots/")
	if !found {
		return "", "", false, errUnsafeArchive
	}
	rootID, rel, found = strings.Cut(rest, "/")
	if !found || !safeKeySegment(rootID) || rel == "" {
		return "", "", false, errUnsafeArchive
	}
	return rootID, rel, true, nil
}

// walkArchive streams a session-recorder tar.gz and calls fn for each entry:
// sources with their root id and spool relative path, the trailing manifest
// with an empty root id. Entry names are validated against path traversal,
// only regular files are accepted and the decompressed size is bounded. fn
// may return errStopArchive to end the walk early.
func walkArchive(r io.Reader, fn func(rootID, rel string, size int64, body io.Reader) error) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(&expandLimitReader{r: gz, left: maxArchiveExpandedBytes})
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size < 0 {
			return errUnsafeArchive
		}
		rootID, rel, isSource, err := archiveEntryPath(hdr.Name)
		if err != nil {
			return err
		}
		if !isSource {
			rel = ArchiveManifestName
		}
		if err := fn(rootID, rel, hdr.Size, tr); err != nil {
			if errors.Is(err, errStopArchive) {
				return nil
			}
			return err
		}
	}
}

// ContentBody is a request or response body returned to the console.
type ContentBody struct {
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Truncated   bool   `json:"truncated"`
	Encoding    string `json:"encoding"`
	Text        string `json:"text"`
}

// RecordContent is the response of the record content API.
type RecordContent struct {
	Record   json.RawMessage `json:"record"`
	Request  *ContentBody    `json:"request"`
	Response *ContentBody    `json:"response"`
	Source   string          `json:"source"`
}

// newContentBody builds a body from at most MaxContentBodyBytes of data;
// stored is the size of the body file. Text that is valid UTF-8 apart from
// a character cut at the limit is returned as utf8, anything else as base64.
func newContentBody(meta BodyFile, data []byte, stored int64) *ContentBody {
	body := &ContentBody{
		ContentType: meta.ContentType,
		Size:        max(meta.OriginalLen, stored),
		Truncated:   meta.Truncated || stored > int64(len(data)),
	}
	text := data
	if stored > int64(len(data)) {
		for cut := 0; cut < utf8.UTFMax && len(text) > 0 && !utf8.Valid(text); cut++ {
			text = text[:len(text)-1]
		}
	}
	if utf8.Valid(text) {
		body.Encoding, body.Text = "utf8", string(text)
		return body
	}
	body.Encoding, body.Text = "base64", base64.StdEncoding.EncodeToString(data)
	return body
}

// readBodyEntry reads at most MaxContentBodyBytes of a body of the given
// size, refusing entries larger than maxArchiveEntryBytes.
func readBodyEntry(meta BodyFile, size int64, r io.Reader) (*ContentBody, error) {
	if size > maxArchiveEntryBytes {
		return nil, errUnsafeArchive
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxContentBodyBytes))
	if err != nil {
		return nil, err
	}
	return newContentBody(meta, data, size), nil
}

// findRecordLine returns the JSONL line of recordID in a record file.
func findRecordLine(r io.Reader, name, recordID string) (json.RawMessage, Record, error) {
	needle := []byte(`"record_id":"` + recordID + `"`)
	var found json.RawMessage
	var rec Record
	err := scanRecordLines(r, name, func(line []byte, _ int64) error {
		if !bytes.Contains(line, needle) {
			return nil
		}
		var candidate Record
		if common.Unmarshal(line, &candidate) != nil || candidate.RecordID != recordID {
			return nil
		}
		found, rec = append(json.RawMessage(nil), line...), candidate
		return errStopArchive
	})
	if err != nil && !errors.Is(err, errStopArchive) {
		return nil, Record{}, err
	}
	if found == nil {
		return nil, Record{}, errRecordNotFound
	}
	return found, rec, nil
}

func (c *RecordContent) setBody(kind string, body *ContentBody) {
	switch kind {
	case "request":
		c.Request = body
	case "response":
		c.Response = body
	}
}

// openSpoolFile opens <root>/<rel> without following symlinks anywhere
// below the root.
func openSpoolFile(root, rel string) (*os.File, int64, error) {
	if rel == "" || strings.Contains(rel, "\\") || path.IsAbs(rel) || path.Clean(rel) != rel || strings.HasPrefix(rel, "../") || rel == ".." {
		return nil, 0, errUnsafeArchive
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if !pathInside(root, abs) {
		return nil, 0, errUnsafeArchive
	}
	if symlink, err := hasSymlinkComponent(root, abs); err != nil || symlink {
		if err == nil {
			err = errUnsafeArchive
		}
		return nil, 0, err
	}
	file, err := openNoFollow(abs)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		if err == nil {
			err = errUnsafeArchive
		}
		return nil, 0, err
	}
	return file, info.Size(), nil
}

// readLocalContent reads a record and its bodies from the spool. A missing
// record or body is reported as fs.ErrNotExist so the caller can fall back
// to the archive (cleanup removes bodies before their record file).
func readLocalContent(root, recordPath, recordID string) (*RecordContent, error) {
	file, _, err := openSpoolFile(root, recordPath)
	if err != nil {
		// Missing, replaced by a symlink or an unexpected path: the archive
		// copy is the trustworthy one.
		return nil, fs.ErrNotExist
	}
	line, rec, err := findRecordLine(file, recordPath, recordID)
	file.Close()
	if errors.Is(err, errRecordNotFound) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	content := &RecordContent{Record: line, Source: "local"}
	for _, meta := range rec.BodyFiles {
		rel, ok := resolveRecordRef(recordPath, meta.File)
		if !ok {
			continue
		}
		body, size, err := openSpoolFile(root, rel)
		if err != nil {
			return nil, fs.ErrNotExist
		}
		parsed, err := readBodyEntry(meta, size, body)
		body.Close()
		if err != nil {
			return nil, err
		}
		content.setBody(meta.Kind, parsed)
	}
	return content, nil
}

// readArchiveContent reads a record and its bodies from a cached archive
// file in two passes: bodies precede their record file in the archive.
func readArchiveContent(file *os.File, rootID, recordPath, recordID string) (*RecordContent, error) {
	var line json.RawMessage
	var rec Record
	err := walkArchive(file, func(entryRoot, rel string, _ int64, body io.Reader) error {
		if entryRoot != rootID || rel != recordPath {
			return nil
		}
		var findErr error
		line, rec, findErr = findRecordLine(body, rel, recordID)
		if findErr != nil {
			return findErr
		}
		return errStopArchive
	})
	if errors.Is(err, errRecordNotFound) || (err == nil && line == nil) {
		return nil, ErrRecordContentUnavailable
	}
	if err != nil {
		return nil, err
	}
	content := &RecordContent{Record: line, Source: "archive"}
	wanted := make(map[string]BodyFile)
	for _, meta := range rec.BodyFiles {
		if rel, ok := resolveRecordRef(recordPath, meta.File); ok {
			wanted[rel] = meta
		}
	}
	if len(wanted) == 0 {
		return content, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	err = walkArchive(file, func(entryRoot, rel string, size int64, body io.Reader) error {
		meta, ok := wanted[rel]
		if entryRoot != rootID || !ok {
			return nil
		}
		parsed, err := readBodyEntry(meta, size, body)
		if err != nil {
			return err
		}
		content.setBody(meta.Kind, parsed)
		delete(wanted, rel)
		if len(wanted) == 0 {
			return errStopArchive
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return content, nil
}

// ContentLocation is where the index says a record lives.
type ContentLocation struct {
	RecordID   string
	RootID     string
	RecordPath string
	ObjectKey  string
}

// ReadRecordContent returns a record and its bodies, from the local spool
// while it still has them, otherwise from the R2 archive through the local
// archive cache. store may be nil; it is then opened from settings only when
// the archive is needed.
func ReadRecordContent(ctx context.Context, settings Settings, loc ContentLocation, store ArchiveGetter) (*RecordContent, error) {
	root := filepath.Clean(settings.SpoolRoot)
	if loc.RootID != "" && loc.RootID == strings.TrimSpace(settings.RootID) && filepath.IsAbs(root) {
		content, err := readLocalContent(root, loc.RecordPath, loc.RecordID)
		if err == nil {
			return content, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	if loc.ObjectKey == "" {
		return nil, ErrRecordContentUnavailable
	}
	if !filepath.IsAbs(settings.WorkDir) {
		return nil, ErrArchiveStoreUnavailable
	}
	if store == nil {
		r2, err := OpenArchiveStore(settings)
		if err != nil {
			return nil, err
		}
		store = r2
	}
	file, err := contentCache.open(ctx, store, filepath.Join(filepath.Clean(settings.WorkDir), contentCacheDirName), loc.ObjectKey)
	if errors.Is(err, ErrObjectNotFound) {
		return nil, ErrRecordContentUnavailable
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readArchiveContent(file, loc.RootID, loc.RecordPath, loc.RecordID)
}

// OpenArchiveStore builds a read client for the configured R2 bucket.
func OpenArchiveStore(settings Settings) (*R2Store, error) {
	creds, err := LoadCredentials(settings.CredentialsDir)
	if err != nil || creds.State() != CredentialConfigured {
		return nil, ErrArchiveStoreUnavailable
	}
	store, err := NewR2Store(settings.ToConfig(), creds)
	if err != nil {
		return nil, ErrArchiveStoreUnavailable
	}
	return store, nil
}

// archiveCache keeps recently read archives under work_dir/content-cache.
// An entry read within contentCacheTTL is reused; the directory is bounded
// by contentCacheMaxSize and trimmed least-recently-used first.
type archiveCache struct {
	mu    sync.Mutex
	group singleflight.Group
}

var contentCache = &archiveCache{}

func (a *archiveCache) open(ctx context.Context, store ArchiveGetter, dir, key string) (*os.File, error) {
	sum := sha256.Sum256([]byte(key))
	name := filepath.Join(dir, hex.EncodeToString(sum[:])+".tar.gz")
	a.mu.Lock()
	info, err := os.Stat(name)
	fresh := err == nil && time.Since(info.ModTime()) < contentCacheTTL
	if fresh {
		now := time.Now()
		_ = os.Chtimes(name, now, now)
	}
	a.mu.Unlock()
	if !fresh {
		if _, err, _ := a.group.Do(name, func() (any, error) {
			return nil, a.download(ctx, store, dir, name, key)
		}); err != nil {
			return nil, err
		}
	}
	return os.Open(name)
}

func (a *archiveCache) download(ctx context.Context, store ArchiveGetter, dir, name, key string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body, size, err := store.Get(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()
	if size > maxArchiveDownloadBytes {
		return errArchiveTooBig
	}
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	written, err := io.Copy(tmp, io.LimitReader(body, maxArchiveDownloadBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if written > maxArchiveDownloadBytes {
		return errArchiveTooBig
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.Rename(tmp.Name(), name); err != nil {
		return err
	}
	a.evictLocked(dir, name)
	return nil
}

// evictLocked drops expired entries, then the least recently used ones
// until the cache fits its size limit. keep is never removed.
func (a *archiveCache) evictLocked(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type cached struct {
		name string
		size int64
		mod  time.Time
	}
	var live []cached
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		file := filepath.Join(dir, entry.Name())
		age := time.Since(info.ModTime())
		if strings.HasPrefix(entry.Name(), ".download-") {
			if age > time.Hour {
				_ = os.Remove(file)
			}
			continue
		}
		if file != keep && age >= contentCacheTTL {
			_ = os.Remove(file)
			continue
		}
		live = append(live, cached{name: file, size: info.Size(), mod: info.ModTime()})
		total += info.Size()
	}
	sort.Slice(live, func(i, j int) bool { return live[i].mod.Before(live[j].mod) })
	for _, entry := range live {
		if total <= contentCacheMaxSize {
			return
		}
		if entry.name == keep {
			continue
		}
		if os.Remove(entry.name) == nil {
			total -= entry.size
		}
	}
}
