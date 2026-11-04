package sessionrecorder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fake, non-functional credentials used only against the in-process fake R2.
const (
	testAccessKey = "TESTACCESSKEYEXAMPLE"
	testSecretKey = "test-secret-not-real"
)

func TestBatchIDAndObjectKeyGolden(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SiteID, cfg.HostID = "site-a", "host-1"
	cfg.R2.ObjectPrefix = "standalone-v1"
	src := []SourceEntry{
		{RootID: "primary", AbsolutePath: "/spool/m/jsonl/b.jsonl", RelativePath: "m/jsonl/b.jsonl", Size: 20, ModTimeNanos: 1700000000000000001, Mode: 0o644, Cleanup: CleanupDeleted},
		{RootID: "primary", AbsolutePath: "/spool/m/jsonl/a.jsonl", RelativePath: "m/jsonl/a.jsonl", Size: 10, ModTimeNanos: 1700000000000000000, Mode: 0o644},
	}
	now := time.Date(2026, 9, 28, 13, 4, 5, 6, time.FixedZone("x", 8*3600))
	m, err := NewPlannedManifest(src, now, cfg)
	require.NoError(t, err)

	// Golden fingerprint bytes: sorted by root/relative path, cleanup reset
	// to "pending", omitempty fields dropped.
	golden := `{"site":"site-a","host":"host-1","sources":[` +
		`{"root_id":"primary","absolute_path":"/spool/m/jsonl/a.jsonl","relative_path":"m/jsonl/a.jsonl","size":10,"mod_time_unix_nano":1700000000000000000,"mode":420,"cleanup":"pending"},` +
		`{"root_id":"primary","absolute_path":"/spool/m/jsonl/b.jsonl","relative_path":"m/jsonl/b.jsonl","size":20,"mod_time_unix_nano":1700000000000000001,"mode":420,"cleanup":"pending"}]}`
	raw, err := common.Marshal(batchFingerprint{Site: "site-a", Host: "host-1", Sources: m.Sources})
	require.NoError(t, err)
	require.Equal(t, golden, string(raw))
	sum := sha256.Sum256([]byte(golden))
	require.Equal(t, hex.EncodeToString(sum[:16]), m.BatchID)
	require.Len(t, m.BatchID, 32)

	require.Equal(t, "standalone-v1/site-a/host-1/2026/09/28/05/"+m.BatchID+".tar.gz", m.ObjectKey)
	require.Equal(t, StatePlanned, m.State)
	require.Equal(t, "2026-09-28T05:04:05.000000006Z", m.CreatedAt)
	require.Equal(t, "m/jsonl/a.jsonl", m.Sources[0].RelativePath)
	require.Equal(t, CleanupPending, m.Sources[1].Cleanup)

	assert.Equal(t, "p/s/h/2026/01/02/03/id.tar.gz", ObjectKey("p/", "s", "h", time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC), "id"))
	assert.Equal(t, "standalone-v1/_checks/abc", CheckObjectKey("standalone-v1", "abc"))

	_, err = NewPlannedManifest(nil, now, cfg)
	require.EqualError(t, err, "sources: empty")
}

func TestManifestGoldenJSON(t *testing.T) {
	m := BatchManifest{FormatVersion: 1, BatchID: "b", State: StateArchived, SiteID: "s", HostID: "h",
		CreatedAt: "c", UpdatedAt: "u", Sources: []SourceEntry{{RootID: "r", Cleanup: CleanupPending}}, ObjectKey: "k"}
	raw, err := common.Marshal(m)
	require.NoError(t, err)
	require.Equal(t, `{"format_version":1,"batch_id":"b","state":"archived","site_id":"s","host_id":"h","created_at":"c","updated_at":"u",`+
		`"sources":[{"root_id":"r","absolute_path":"","relative_path":"","size":0,"mod_time_unix_nano":0,"mode":0,"cleanup":"pending"}],`+
		`"object_key":"k","deleted_count":0,"skipped_count":0,"missing_count":0,"failed_count":0,"retry_count":0}`, string(raw))
}

// writeSpoolFile creates a file and backdates it past the stability window.
func writeSpoolFile(t *testing.T, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
	require.NoError(t, os.WriteFile(name, []byte(content), 0o644))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(name, old, old))
}

// withStorageUsage pins the disk/inode usage seen by the recorder.
func withStorageUsage(t *testing.T, disk, inode int) {
	t.Helper()
	old := storageUsageFn
	storageUsageFn = func(string) (int, int, error) { return disk, inode, nil }
	t.Cleanup(func() { storageUsageFn = old })
}

func testConfig(t *testing.T) Config {
	t.Helper()
	withStorageUsage(t, 10, 10)
	base := t.TempDir()
	cfg := DefaultConfig()
	cfg.InstallationID = "0123456789abcdef0123456789abcdef"
	cfg.SiteID, cfg.HostID = "site-a", "host-1"
	cfg.SpoolRoots = []SpoolRoot{{RootID: "primary", Path: filepath.Join(base, "spool")}}
	cfg.WorkDir = filepath.Join(base, "work")
	cfg.R2.Bucket = "bucket"
	cfg.R2.Endpoint = "http://127.0.0.1"
	cfg.StabilityWindowSeconds = 60
	cfg.RetryInitialBackoffSeconds = 1
	cfg.RetryMaxBackoffSeconds = 1
	return cfg
}

func makeRecord(t *testing.T, root, model, name string, bodyRefs ...string) string {
	t.Helper()
	var refs []string
	for _, r := range bodyRefs {
		refs = append(refs, `{"kind":"request","file":"`+r+`"}`)
		writeSpoolFile(t, filepath.Join(root, model, filepath.FromSlash(r)), "body:"+r)
	}
	line := `{"schema_version":1,"attachments":[],"body_files":[` + strings.Join(refs, ",") + `]}` + "\n"
	file := filepath.Join(root, model, "jsonl", "2026", "09", "28", "05", name)
	writeSpoolFile(t, file, line)
	return file
}

func TestScanAndArchiveFormat(t *testing.T) {
	cfg := testConfig(t)
	root := cfg.SpoolRoots[0].Path
	makeRecord(t, root, "gpt-4o", "00-aaaa.jsonl", "body/2026/09/28/05/ab/cd/s1/r1_request.body")
	makeRecord(t, root, "gpt-4o", "10-aaaa.jsonl", "body/2026/09/28/05/ab/cd/s1/r2_request.body")
	writeSpoolFile(t, filepath.Join(root, "gpt-4o", "media", "orphan.bin"), "orphan")
	// Incomplete JSONL (no trailing newline) is reported, not archived.
	writeSpoolFile(t, filepath.Join(root, "gpt-4o", "jsonl", "x.jsonl"), `{"a":1}`)

	scan := ScanSpools(cfg, time.Now(), nil)
	require.Len(t, scan.Units, 2)
	require.Equal(t, 1, scan.OrphanMedia)
	require.Equal(t, 4, scan.BacklogFiles)
	require.Contains(t, scan.Issues, ScanIssue{Path: filepath.Join(root, "gpt-4o", "jsonl", "x.jsonl"), Category: "incomplete_jsonl"})

	units, err := SelectUnitsByCompressedTarget(scan.Units, cfg)
	require.NoError(t, err)
	require.Len(t, units, 2)
	m, err := NewPlannedManifest(UnitSources(units), time.Now(), cfg)
	require.NoError(t, err)
	require.NoError(t, CreateArchive(&m, cfg, time.Now()))
	require.Equal(t, StateArchived, m.State)
	require.Equal(t, filepath.Join(cfg.WorkDir, "archives", m.BatchID+".tar.gz"), m.ArchivePath)

	f, err := os.Open(m.ArchivePath)
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	var names []string
	var manifestRaw []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		names = append(names, hdr.Name)
		require.Equal(t, byte(tar.TypeReg), hdr.Typeflag)
		if hdr.Name == ArchiveManifestName {
			require.Equal(t, int64(0o600), hdr.Mode)
			manifestRaw, err = io.ReadAll(tr)
			require.NoError(t, err)
		}
	}
	require.Equal(t, ArchiveManifestName, names[len(names)-1], "manifest must be the last entry")
	require.Equal(t, "roots/primary/gpt-4o/body/2026/09/28/05/ab/cd/s1/r1_request.body", names[0])
	require.Len(t, names, 5)
	var am archiveManifest
	require.NoError(t, common.Unmarshal(manifestRaw, &am))
	require.Equal(t, 1, am.FormatVersion)
	require.Equal(t, m.BatchID, am.BatchID)
	require.Equal(t, map[string]string{"primary": "roots/primary"}, am.Roots)
	require.Len(t, am.Sources, 4)
	require.True(t, strings.HasPrefix(string(manifestRaw), `{"format_version":1,"batch_id":"`))
}

func TestFilterMatching(t *testing.T) {
	f, err := CompileFilter(FilterConfig{})
	require.NoError(t, err)
	assert.True(t, f.Match(RequestInfo{ChannelID: 1, Group: "default", Model: "gpt-4o"}), "empty include records everything")

	f, err = CompileFilter(FilterConfig{
		Include: []FilterRule{
			{ChannelID: 3},
			{ChannelID: 5, Model: "claude-*"},
			{Group: "vip"},
			{Group: "default", Model: "gpt-4o"},
			{Model: "gemini-*-flash"},
		},
		Exclude: []FilterRule{{ChannelID: 3, Model: "gpt-4o-mini"}, {Group: "vip", Model: "o1?"}},
	})
	require.NoError(t, err)
	cases := []struct {
		info RequestInfo
		want bool
	}{
		{RequestInfo{3, "x", "anything"}, true},
		{RequestInfo{3, "x", "gpt-4o-mini"}, false},
		{RequestInfo{5, "x", "claude-sonnet"}, true},
		{RequestInfo{5, "x", "gpt-4o"}, false},
		{RequestInfo{9, "vip", "gpt-4o"}, true},
		{RequestInfo{9, "vip", "o1p"}, false},
		{RequestInfo{9, "default", "gpt-4o"}, true},
		{RequestInfo{9, "default", "gpt-4o-2024"}, false},
		{RequestInfo{9, "x", "gemini-2.5-flash"}, true},
		{RequestInfo{9, "x", "gemini-2.5-pro"}, false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, f.Match(tc.info), "%+v", tc.info)
	}
	_, err = CompileFilter(FilterConfig{Include: []FilterRule{{Model: "bad[x"}}})
	require.Error(t, err)
	var nilFilter *Filter
	assert.False(t, nilFilter.Match(RequestInfo{}))
}

func TestProducerQueueFullDropsWithoutBlocking(t *testing.T) {
	s := DefaultSettings()
	s.SpoolRoot = t.TempDir()
	s.QueueSize = 1
	p := NewProducer(s) // writer not started: the queue cannot drain
	require.True(t, p.Reserve(100))
	start := time.Now()
	require.True(t, p.Enqueue(&Capture{}, 100))
	require.True(t, p.Reserve(50))
	require.False(t, p.Enqueue(&Capture{}, 50))
	require.False(t, p.Enqueue(&Capture{}, 0))
	require.Less(t, time.Since(start), time.Second)
	c := p.Counters()
	require.EqualValues(t, 1, c.Enqueued)
	require.EqualValues(t, 2, c.DroppedFull)
	require.EqualValues(t, 100, c.InflightBytes, "dropped capture releases its reservation")
	require.Equal(t, 100, p.UsagePercent())

	p.maxBytes = 10
	require.False(t, p.Reserve(1000), "memory budget is enforced")
}

func TestProducerWritesRequestReaderToSpool(t *testing.T) {
	s := DefaultSettings()
	s.SpoolRoot = filepath.Join(t.TempDir(), "spool")
	p := NewProducer(s)
	body := `{"model":"gpt-4o"}`
	finished := time.Date(2026, 9, 28, 5, 17, 0, 0, time.UTC)
	capture := &Capture{
		Record:            Record{RecordID: "reader-req", SessionID: "session", Model: "gpt-4o"},
		RequestBodyReader: io.NopCloser(strings.NewReader(body)),
		RequestBodyLimit:  int64(len(body)),
		RequestFull:       int64(len(body)),
		IncludeRequest:    true,
		FinishedAt:        finished,
	}
	require.True(t, p.Reserve(int64(len(body))))
	p.write(capture)
	require.EqualValues(t, 1, p.Counters().Written)

	shard := filepath.Join(s.SpoolRoot, "gpt-4o", "jsonl", "2026", "09", "28", "05", "10-"+p.instance+".jsonl")
	raw, err := os.ReadFile(shard)
	require.NoError(t, err)
	var record Record
	require.NoError(t, common.Unmarshal(bytes.TrimSpace(raw), &record))
	require.Len(t, record.BodyFiles, 1)
	require.Equal(t, "request", record.BodyFiles[0].Kind)
	bodyPath := filepath.Join(s.SpoolRoot, "gpt-4o", filepath.FromSlash(record.BodyFiles[0].File))
	actualBody, err := os.ReadFile(bodyPath)
	require.NoError(t, err)
	require.Equal(t, body, string(actualBody))
}

func TestProducerWritesSpoolProtocol(t *testing.T) {
	withStorageUsage(t, 10, 10)
	sink := installFakeSink(t)
	s := DefaultSettings()
	s.SpoolRoot = filepath.Join(t.TempDir(), "spool")
	s.StatsPath = filepath.Join(filepath.Dir(s.SpoolRoot), "stats.json")
	p := NewProducer(s)
	p.Start()
	finished := time.Date(2026, 9, 28, 5, 17, 0, 0, time.UTC)
	ok := p.Enqueue(&Capture{
		Record:            Record{RecordID: "req-1", SessionID: "sess/1", Model: "gpt-4o", Stream: true},
		RequestBodyReader: io.NopCloser(strings.NewReader(`{"model":"gpt-4o"}`)),
		RequestBodyLimit:  int64(len(`{"model":"gpt-4o"}`)),
		ResponseBody:      []byte("data: hi\n\n"),
		IncludeRequest:    true,
		IncludeResponse:   true,
		FinishedAt:        finished,
	}, 0)
	require.True(t, ok)
	p.Stop(5 * time.Second)
	require.EqualValues(t, 1, p.Counters().Written)
	require.Len(t, sink.upserts, 1, "the index row is flushed on stop")
	indexed := sink.upserts[0]
	assert.Equal(t, "req-1", indexed.RecordID)
	assert.Equal(t, "gpt-4o/jsonl/2026/09/28/05/10-"+p.instance+".jsonl", indexed.RecordPath)
	assert.Equal(t, "primary", indexed.RootID)
	assert.Zero(t, indexed.ArchiveBytes)

	shard := filepath.Join(s.SpoolRoot, "gpt-4o", "jsonl", "2026", "09", "28", "05", "10-"+p.instance+".jsonl")
	raw, err := os.ReadFile(shard)
	require.NoError(t, err)
	require.True(t, bytes.HasSuffix(raw, []byte("\n")))
	assert.EqualValues(t, len(raw)+len(`{"model":"gpt-4o"}`)+len("data: hi\n\n"), indexed.RawBytes)
	var rec Record
	require.NoError(t, common.Unmarshal(bytes.TrimSpace(raw), &rec))
	require.Len(t, rec.BodyFiles, 2)
	require.True(t, strings.HasPrefix(rec.BodyFiles[0].File, "body/2026/09/28/05/"))
	require.True(t, strings.HasSuffix(rec.BodyFiles[1].File, "_response.sse"))
	for _, b := range rec.BodyFiles {
		bodyPath := filepath.Join(s.SpoolRoot, "gpt-4o", filepath.FromSlash(b.File))
		_, err := os.Stat(bodyPath)
		require.NoError(t, err)
		require.False(t, strings.HasSuffix(b.File, ".json"), "body files must not look like records")
		if b.Kind == "request" {
			body, readErr := os.ReadFile(bodyPath)
			require.NoError(t, readErr)
			require.Equal(t, `{"model":"gpt-4o"}`, string(body))
		}
	}
	var stats ProducerStats
	statsRaw, err := os.ReadFile(s.StatsPath)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(statsRaw, &stats))
	_, err = time.Parse(time.RFC3339Nano, stats.Timestamp)
	require.NoError(t, err)

	// The scanner picks the record and both bodies up as one unit.
	cfg := DefaultConfig()
	cfg.SpoolRoots = []SpoolRoot{{RootID: "primary", Path: s.SpoolRoot}}
	cfg.StabilityWindowSeconds = 1
	scan := ScanSpools(cfg, time.Now().Add(time.Hour), nil)
	require.Len(t, scan.Units, 1)
	require.Len(t, scan.Units[0].Entries, 3)
	require.Zero(t, scan.OrphanMedia)
}

// fakeR2 is a minimal in-memory S3 endpoint that only accepts the methods
// the recorder is allowed to use.
type fakeR2 struct {
	mu        sync.Mutex
	objects   map[string][]byte
	meta      map[string]map[string]string
	uploads   map[string]map[int][]byte
	upMeta    map[string]map[string]string
	failPuts  int
	pageSize  int
	lists     int
	methods   map[string]int
	forbidden []string
}

func newFakeR2() *fakeR2 {
	return &fakeR2{objects: map[string][]byte{}, meta: map[string]map[string]string{},
		uploads: map[string]map[int][]byte{}, upMeta: map[string]map[string]string{}, methods: map[string]int{}}
}

func amzMeta(h http.Header) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		if name, ok := strings.CutPrefix(strings.ToLower(k), "x-amz-meta-"); ok {
			out[name] = v[0]
		}
	}
	return out
}

func (f *fakeR2) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.methods[r.Method]++
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") || r.Header.Get("X-Amz-Content-Sha256") == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodGet && q.Get("list-type") == "2":
		f.lists++
		prefix, after := q.Get("prefix"), q.Get("continuation-token")
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, prefix) && k > after {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		size := f.pageSize
		if size <= 0 {
			size = 1000
		}
		truncated := len(keys) > size
		if truncated {
			keys = keys[:size]
		}
		var b strings.Builder
		b.WriteString(`<ListBucketResult>`)
		for _, k := range keys {
			b.WriteString(`<Contents><Key>` + k + `</Key></Contents>`)
		}
		if truncated {
			b.WriteString(`<IsTruncated>true</IsTruncated><NextContinuationToken>` + keys[len(keys)-1] + `</NextContinuationToken>`)
		}
		b.WriteString(`</ListBucketResult>`)
		_, _ = io.WriteString(w, b.String())
	case r.Method == http.MethodGet:
		data, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	case r.Method == http.MethodHead:
		data, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for k, v := range f.meta[key] {
			w.Header().Set("X-Amz-Meta-"+k, v)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	case r.Method == http.MethodPut && q.Has("partNumber"):
		n, _ := strconv.Atoi(q.Get("partNumber"))
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != r.Header.Get("X-Amz-Content-Sha256") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.uploads[q.Get("uploadId")][n] = body
		w.Header().Set("ETag", `"etag-`+strconv.Itoa(n)+`"`)
	case r.Method == http.MethodPut:
		if f.failPuts > 0 {
			f.failPuts--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if _, exists := f.objects[key]; exists && r.Header.Get("If-None-Match") == "*" {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body
		f.meta[key] = amzMeta(r.Header)
	case r.Method == http.MethodPost && q.Has("uploads"):
		id := "upload-" + strconv.Itoa(len(f.uploads)+1)
		f.uploads[id] = map[int][]byte{}
		f.upMeta[id] = amzMeta(r.Header)
		_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>`+id+`</UploadId></InitiateMultipartUploadResult>`)
	case r.Method == http.MethodPost && q.Has("uploadId"):
		id := q.Get("uploadId")
		var req completeRequest
		raw, _ := io.ReadAll(r.Body)
		if err := xml.Unmarshal(raw, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var buf bytes.Buffer
		for i, p := range req.Parts {
			if p.PartNumber != i+1 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			buf.Write(f.uploads[id][p.PartNumber])
		}
		f.objects[key] = buf.Bytes()
		f.meta[key] = f.upMeta[id]
		_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><Key>`+key+`</Key></CompleteMultipartUploadResult>`)
	default:
		f.forbidden = append(f.forbidden, r.Method+" "+r.URL.String())
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newTestStore(t *testing.T, cfg Config) *R2Store {
	t.Helper()
	store, err := NewR2Store(cfg, Credentials{AccessKeyID: testAccessKey, SecretAccessKey: testSecretKey})
	require.NoError(t, err)
	return store
}

func TestConsumerRecoversAcrossRestartWithFakeR2(t *testing.T) {
	fake := newFakeR2()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	cfg := testConfig(t)
	cfg.R2.Endpoint = srv.URL
	root := cfg.SpoolRoots[0].Path
	rec := makeRecord(t, root, "claude-sonnet", "00-bbbb.jsonl", "body/2026/09/28/05/aa/bb/s/r_request.body")
	require.NoError(t, cfg.Validate())

	// First run: the PUT fails, the batch stays archived on disk.
	fake.failPuts = 1
	first := NewConsumer(cfg, newTestStore(t, cfg))
	res := first.RunCycle(context.Background())
	require.Equal(t, "upload_failed", res.State)
	manifests, _ := filepath.Glob(filepath.Join(cfg.WorkDir, "manifests", "*.json"))
	require.Len(t, manifests, 1)
	m, err := LoadManifest(manifests[0])
	require.NoError(t, err)
	require.Equal(t, StateArchived, m.State)
	require.Equal(t, 1, m.RetryCount)

	// "Restart": a fresh consumer resumes the persisted batch.
	second := NewConsumer(cfg, newTestStore(t, cfg))
	res = second.RunCycle(context.Background())
	require.Equal(t, string(StateCleaned), res.State)
	m, err = LoadManifest(manifests[0])
	require.NoError(t, err)
	require.Equal(t, StateCleaned, m.State)
	require.Equal(t, 2, m.DeletedCount)
	_, err = os.Stat(rec)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(m.ArchivePath)
	require.ErrorIs(t, err, os.ErrNotExist, "local archive removed after verify")

	obj, ok := fake.objects[m.ObjectKey]
	require.True(t, ok)
	sum := sha256.Sum256(obj)
	require.Equal(t, m.ArchiveSHA256, hex.EncodeToString(sum[:]))
	require.Equal(t, m.ArchiveSHA256, fake.meta[m.ObjectKey][metaSHA256])
	require.True(t, strings.HasPrefix(m.ObjectKey, "standalone-v1/site-a/host-1/"))

	// A re-run with nothing new is idle; nothing is uploaded twice.
	res = second.RunCycle(context.Background())
	require.Equal(t, "idle", res.State)
	require.Empty(t, fake.forbidden)
	require.Zero(t, fake.methods[http.MethodDelete])
}

func TestConsumerRetainModeDoesNotReupload(t *testing.T) {
	fake := newFakeR2()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	cfg := testConfig(t)
	cfg.R2.Endpoint = srv.URL
	cfg.LocalCleanupEnabled = false
	root := cfg.SpoolRoots[0].Path
	rec := makeRecord(t, root, "m", "00-cccc.jsonl")
	c := NewConsumer(cfg, newTestStore(t, cfg))
	require.Equal(t, string(StateRetained), c.RunCycle(context.Background()).State)
	_, err := os.Stat(rec)
	require.NoError(t, err, "retain mode keeps local files")
	puts := fake.methods[http.MethodPut]
	require.Equal(t, "idle", c.RunCycle(context.Background()).State)
	require.Equal(t, puts, fake.methods[http.MethodPut])
	require.Len(t, fake.objects, 1)
}

func TestMultipartUploadAndCheck(t *testing.T) {
	fake := newFakeR2()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	cfg := testConfig(t)
	cfg.R2.Endpoint = srv.URL
	old := multipartPart
	multipartPart = 7
	defer func() { multipartPart = old }()

	file := filepath.Join(t.TempDir(), "a.tar.gz")
	payload := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	require.NoError(t, os.WriteFile(file, payload, 0o600))
	sum := sha256.Sum256(payload)
	store := newTestStore(t, cfg)
	key := "standalone-v1/s/h/2026/09/28/05/x y+z.tar.gz"
	require.NoError(t, store.PutFile(context.Background(), key, file, int64(len(payload)), hex.EncodeToString(sum[:])))
	require.Equal(t, payload, fake.objects[key])
	require.Equal(t, hex.EncodeToString(sum[:]), fake.meta[key][metaSHA256])
	meta, err := store.Head(context.Background(), key)
	require.NoError(t, err)
	require.True(t, meta.Exists)
	require.EqualValues(t, len(payload), meta.Size)

	require.NoError(t, Check(context.Background(), cfg, store))
	probe := CheckObjectKey(cfg.R2.ObjectPrefix, cfg.InstallationID)
	require.Equal(t, cfg.InstallationID, fake.meta[probe][metaInstallation])
	require.NoError(t, Check(context.Background(), cfg, store), "second check reuses the probe")
	other := cfg
	other.InstallationID = "ffffffffffffffffffffffffffffffff"
	fake.objects[CheckObjectKey(cfg.R2.ObjectPrefix, other.InstallationID)] = nil
	require.Error(t, Check(context.Background(), other, store), "foreign probe is never overwritten")
	require.Empty(t, fake.forbidden)
}

func TestCredentialsAndQueueGuard(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	c, err := LoadCredentials("")
	require.NoError(t, err)
	require.Equal(t, CredentialMissing, c.State())

	t.Setenv("AWS_ACCESS_KEY_ID", testAccessKey)
	c, _ = LoadCredentials("")
	require.Equal(t, CredentialInvalid, c.State())

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, credentialAccessKeyFile), []byte(testAccessKey+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, credentialSecretKeyFile), []byte(testSecretKey), 0o600))
	c, err = LoadCredentials(dir)
	require.NoError(t, err)
	require.Equal(t, CredentialConfigured, c.State())
	require.NotContains(t, c.String(), testSecretKey)
	raw, err := common.Marshal(c)
	require.NoError(t, err)
	require.Equal(t, "{}", string(raw))

	now := time.Now()
	qc := QueueConfig{BusyThreshold: 25, FreshnessSeconds: 120}
	state, _ := evaluateStats(ProducerStats{Timestamp: now.UTC().Format(time.RFC3339Nano), ExportQueueUsagePercent: 10}, qc, now)
	require.Equal(t, QueueHealthy, state)
	state, _ = evaluateStats(ProducerStats{Timestamp: now.UTC().Format(time.RFC3339Nano), ExportQueueUsagePercent: 30}, qc, now)
	require.Equal(t, QueueBusy, state)
	state, err = evaluateStats(ProducerStats{Timestamp: now.Add(-time.Hour).UTC().Format(time.RFC3339Nano)}, qc, now)
	require.Equal(t, QueueUnknown, state)
	require.EqualError(t, err, "queue stats stale")
	state, _ = EvaluateQueueGuard(QueueConfig{}, now)
	require.Equal(t, QueueDisabled, state)
}

func TestSettingsDefaultsAreOff(t *testing.T) {
	s, err := ParseSettings("")
	require.NoError(t, err)
	require.False(t, s.Enabled)
	require.False(t, s.ProducerEnabled)
	require.False(t, s.ConsumerEnabled)
	require.NoError(t, s.Validate())
	s.Enabled, s.ConsumerEnabled = true, true
	s.SpoolRoot, s.WorkDir = filepath.Join(t.TempDir(), "spool"), filepath.Join(t.TempDir(), "work")
	s.R2Endpoint, s.R2Bucket = "https://example.invalid", "b"
	require.NoError(t, s.Validate())
	s.WorkDir = filepath.Join(s.SpoolRoot, "work")
	require.Error(t, s.Validate(), "work_dir must not overlap the spool")
	keys := []string{}
	raw, _ := common.Marshal(DefaultSettings())
	var m map[string]any
	require.NoError(t, common.Unmarshal(raw, &m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		require.NotContains(t, k, "secret")
		require.NotContains(t, k, "access_key")
	}
}

func TestStoragePauseAtNinetyPercent(t *testing.T) {
	cfg := testConfig(t)
	makeRecord(t, cfg.SpoolRoots[0].Path, "m", "00-dddd.jsonl")
	withStorageUsage(t, 50, StoragePauseThreshold)
	c := NewConsumer(cfg, nil)
	require.Equal(t, "storage_paused", c.RunCycle(context.Background()).State)

	s := DefaultSettings()
	s.SpoolRoot = filepath.Join(t.TempDir(), "spool")
	p := NewProducer(s)
	p.checkStorage()
	require.False(t, p.Accepting())
	require.False(t, p.Enqueue(&Capture{}, 0))
	require.EqualValues(t, 1, p.Counters().DroppedStorage)
}

type fakeIndexSink struct {
	mu      sync.Mutex
	upserts []IndexRow
	batches [][]IndexRow
}

func (f *fakeIndexSink) UpsertRecords(_ context.Context, rows []IndexRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts = append(f.upserts, rows...)
	return nil
}

func (f *fakeIndexSink) ApplyBatch(_ context.Context, rows []IndexRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, append([]IndexRow(nil), rows...))
	return nil
}

func installFakeSink(t *testing.T) *fakeIndexSink {
	t.Helper()
	sink := &fakeIndexSink{}
	SetIndexSink(sink)
	t.Cleanup(func() { SetIndexSink(nil) })
	return sink
}

func TestAllocateArchiveBytes(t *testing.T) {
	cases := []struct {
		name    string
		raw     []int64
		archive int64
		want    []int64
	}{
		{"single record takes everything", []int64{123}, 50, []int64{50}},
		{"proportional split", []int64{100, 300}, 40, []int64{10, 30}},
		{"remainder goes to the last record", []int64{1, 1, 1}, 10, []int64{3, 3, 4}},
		{"rounding down per record", []int64{2, 3, 5}, 7, []int64{1, 2, 4}},
		{"all raw bytes zero splits by count", []int64{0, 0, 0}, 8, []int64{2, 2, 4}},
		{"zero archive size", []int64{5, 5}, 0, []int64{0, 0}},
		{"no overflow on large values", []int64{1 << 40, 1 << 40}, 1 << 40, []int64{1 << 39, 1 << 39}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := make([]IndexRow, len(tc.raw))
			for i, r := range tc.raw {
				rows[i].RawBytes = r
			}
			AllocateArchiveBytes(rows, tc.archive)
			var got []int64
			var sum int64
			for _, r := range rows {
				got = append(got, r.ArchiveBytes)
				sum += r.ArchiveBytes
			}
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.archive, sum)
		})
	}
	AllocateArchiveBytes(nil, 10) // no rows: no panic
}

func TestRevenueRulesValidationAndFirstMatch(t *testing.T) {
	rules := []RevenueRule{
		{ID: "off", Name: "Disabled", MatchType: RevenueMatchKeyword, Pattern: "claude", PricePerTB: 9, Enabled: false},
		{ID: "r1", Name: "Opus", MatchType: RevenueMatchGlob, Pattern: "claude-opus-*", PricePerTB: 1200, Enabled: true},
		{ID: "r2", Name: "GPT", MatchType: RevenueMatchKeyword, Pattern: "GPT", PricePerTB: 600, Enabled: true},
		{ID: "r3", Name: "Gemini", MatchType: RevenueMatchRegex, Pattern: `^gemini-3(\.\d+)?-`, PricePerTB: 400, Enabled: true},
		{ID: "r4", Name: "Claude", MatchType: RevenueMatchKeyword, Pattern: "claude", PricePerTB: 100, Enabled: true},
	}
	m, err := CompileRevenueRules(rules)
	require.NoError(t, err)
	for model, want := range map[string]string{
		"claude-opus-4": "r1", "claude-sonnet-4": "r4", "gpt-4o": "r2", "openai/GPT-5": "r2",
		"gemini-3.1-pro": "r3", "gemini-2.5-pro": "", "": "",
	} {
		rule, ok := m.Match(model)
		assert.Equal(t, want != "", ok, model)
		assert.Equal(t, want, rule.ID, model)
	}
	assert.Len(t, m.Rules(), 4, "disabled rules are not reported")

	invalid := []RevenueRule{
		{ID: "", Name: "n", MatchType: RevenueMatchKeyword, Pattern: "p"},
		{ID: "a", Name: "", MatchType: RevenueMatchKeyword, Pattern: "p"},
		{ID: "a", Name: "n", MatchType: "contains", Pattern: "p"},
		{ID: "a", Name: "n", MatchType: RevenueMatchRegex, Pattern: "("},
		{ID: "a", Name: "n", MatchType: RevenueMatchRegex, Pattern: strings.Repeat("a", 513)},
		{ID: "a", Name: "n", MatchType: RevenueMatchGlob, Pattern: "bad[x"},
		{ID: "a", Name: "n", MatchType: RevenueMatchKeyword, Pattern: "p", PricePerTB: -1},
		{ID: "a", Name: "n", MatchType: RevenueMatchKeyword, Pattern: ""},
	}
	for _, rule := range invalid {
		_, err := CompileRevenueRules([]RevenueRule{rule})
		assert.Error(t, err, "%+v", rule)
	}
	dup := RevenueRule{ID: "a", Name: "n", MatchType: RevenueMatchKeyword, Pattern: "p"}
	_, err = CompileRevenueRules([]RevenueRule{dup, dup})
	require.Error(t, err)
	_, err = CompileRevenueRules(make([]RevenueRule, MaxRevenueRules+1))
	require.Error(t, err)
	require.Error(t, ValidateRevenueOption(RevenueRulesOptionKey, `{"not":"array"}`))
	require.NoError(t, ValidateRevenueOption(RevenueRulesOptionKey, ""))
	require.Error(t, ValidateRevenueOption(RevenueCurrencyLabelOptionKey, strings.Repeat("x", 17)))
}

// writeIndexedRecord writes a record line with real record metadata and its
// body files into a spool JSONL shard.
func writeIndexedRecord(t *testing.T, root, model, shard, recordID string, bodies map[string]string) {
	t.Helper()
	rec := Record{
		SchemaVersion: 1, RecordID: recordID, SessionID: "sess-1", StartedAt: "2026-11-02T05:10:00Z",
		Model: model, Path: "/v1/chat/completions", StatusCode: 200, UserID: 7, Attachments: []Attachment{},
	}
	for _, kind := range []string{"request", "response"} {
		content, ok := bodies[kind]
		if !ok {
			continue
		}
		ref := "body/2026/11/02/05/ab/cd/sess-1/" + recordID + "_" + kind + ".body"
		writeSpoolFile(t, filepath.Join(root, model, filepath.FromSlash(ref)), content)
		rec.BodyFiles = append(rec.BodyFiles, BodyFile{Kind: kind, File: ref, Size: int64(len(content)), ContentType: "application/json"})
	}
	line, err := common.Marshal(rec)
	require.NoError(t, err)
	file := filepath.Join(root, model, "jsonl", "2026", "11", "02", "05", shard)
	var existing []byte
	if raw, err := os.ReadFile(file); err == nil {
		existing = raw
	}
	writeSpoolFile(t, file, string(existing)+string(line)+"\n")
}

func TestVerifiedBatchIndexAndContentReadback(t *testing.T) {
	sink := installFakeSink(t)
	fake := newFakeR2()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	cfg := testConfig(t)
	cfg.R2.Endpoint = srv.URL
	root := cfg.SpoolRoots[0].Path
	large := strings.Repeat("a", MaxContentBodyBytes+10)
	writeIndexedRecord(t, root, "gpt-4o", "10-ab46d8fc.jsonl", "req-1", map[string]string{"request": `{"q":"hi"}`, "response": large})
	writeIndexedRecord(t, root, "gpt-4o", "10-ab46d8fc.jsonl", "req-2", map[string]string{"request": `{"q":"again"}`})
	writeIndexedRecord(t, root, "claude-opus-4", "10-ab46d8fc.jsonl", "req-3", nil)

	settings := DefaultSettings()
	settings.SpoolRoot, settings.RootID, settings.WorkDir = root, "primary", cfg.WorkDir
	store := newTestStore(t, cfg)
	loc := ContentLocation{RecordID: "req-1", RootID: "primary", RecordPath: "gpt-4o/jsonl/2026/11/02/05/10-ab46d8fc.jsonl"}

	// Before upload the spool answers.
	local, err := ReadRecordContent(context.Background(), settings, loc, store)
	require.NoError(t, err)
	assert.Equal(t, "local", local.Source)
	require.NotNil(t, local.Request)
	assert.Equal(t, `{"q":"hi"}`, local.Request.Text)
	require.NotNil(t, local.Response)
	assert.True(t, local.Response.Truncated)
	assert.Len(t, local.Response.Text, MaxContentBodyBytes)
	assert.EqualValues(t, len(large), local.Response.Size)

	consumer := NewConsumer(cfg, store)
	require.Equal(t, string(StateCleaned), consumer.RunCycle(context.Background()).State)
	manifests, _ := filepath.Glob(filepath.Join(cfg.WorkDir, "manifests", "*.json"))
	require.Len(t, manifests, 1)
	m, err := LoadManifest(manifests[0])
	require.NoError(t, err)

	require.Len(t, sink.batches, 1)
	rows := sink.batches[0]
	require.Len(t, rows, 3)
	var allocated int64
	byID := map[string]IndexRow{}
	for _, row := range rows {
		allocated += row.ArchiveBytes
		byID[row.RecordID] = row
		assert.Equal(t, m.BatchID, row.BatchID)
		assert.Equal(t, m.ObjectKey, row.ObjectKey)
		assert.Equal(t, "sess-1", row.SessionID)
		assert.Positive(t, row.ArchivedAt)
	}
	assert.Equal(t, m.ArchiveSize, allocated, "allocation sums to the archive size")
	assert.Greater(t, byID["req-1"].RawBytes, int64(MaxContentBodyBytes), "raw bytes include body files")
	assert.Equal(t, "gpt-4o/jsonl/2026/11/02/05/10-ab46d8fc.jsonl", byID["req-1"].RecordPath)
	assert.Equal(t, int64(1793596200), byID["req-1"].CreatedAt)

	// After cleanup the archive answers, and a second read hits the cache.
	loc.ObjectKey = m.ObjectKey
	archived, err := ReadRecordContent(context.Background(), settings, loc, store)
	require.NoError(t, err)
	assert.Equal(t, "archive", archived.Source)
	assert.Equal(t, local.Request, archived.Request)
	assert.Equal(t, local.Response, archived.Response)
	assert.JSONEq(t, string(local.Record), string(archived.Record))
	gets := fake.methods[http.MethodGet]
	_, err = ReadRecordContent(context.Background(), settings, loc, store)
	require.NoError(t, err)
	assert.Equal(t, gets, fake.methods[http.MethodGet], "cached archive is reused")
	missing := loc
	missing.RecordID = "nope"
	_, err = ReadRecordContent(context.Background(), settings, missing, store)
	require.ErrorIs(t, err, ErrRecordContentUnavailable)

	// Backfill works from the R2 listing alone: drop the local manifest and
	// add objects that must be listed across pages or left out.
	require.NoError(t, os.Remove(manifests[0]))
	fake.mu.Lock()
	for _, extra := range []string{"standalone-v1/_checks/probe.tar.gz", "standalone-v1/site-a/notes.txt", "other-prefix/x/y.tar.gz",
		"standalone-v1/site-a/host-1/2026/11/02/05/zz.tar.gz.tmp"} {
		fake.objects[extra] = []byte("x")
	}
	fake.pageSize = 2
	fake.mu.Unlock()
	keys, err := ListArchiveObjects(context.Background(), store, "/standalone-v1/")
	require.NoError(t, err)
	assert.Equal(t, []string{m.ObjectKey}, keys)
	assert.GreaterOrEqual(t, fake.lists, 2, "the listing follows continuation tokens")
	batchID, backfilled, err := IndexArchiveObject(context.Background(), store, m.ObjectKey)
	require.NoError(t, err)
	assert.Equal(t, m.BatchID, batchID, "batch id comes from the manifest inside the archive")
	assert.Equal(t, m.BatchID, BatchIDFromObjectKey(m.ObjectKey))
	require.Len(t, backfilled, len(rows))
	var backfilledBytes int64
	for _, row := range backfilled {
		backfilledBytes += row.ArchiveBytes
		live := byID[row.RecordID]
		assert.Positive(t, row.ArchivedAt)
		row.ArchivedAt = live.ArchivedAt // archive time vs plan time
		assert.Equal(t, live, row)
	}
	assert.EqualValues(t, len(fake.objects[m.ObjectKey]), backfilledBytes, "archive bytes sum to the object size")
	checkpoint := LoadBackfillCheckpoint(cfg.WorkDir)
	checkpoint.Done[m.ObjectKey] = true
	require.NoError(t, SaveBackfillCheckpoint(cfg.WorkDir, checkpoint))
	assert.True(t, LoadBackfillCheckpoint(cfg.WorkDir).Done[m.ObjectKey])
	fake.mu.Lock()
	fake.meta[m.ObjectKey][metaSHA256] = strings.Repeat("0", 64)
	fake.mu.Unlock()
	_, _, err = IndexArchiveObject(context.Background(), store, m.ObjectKey)
	require.Error(t, err, "a corrupted download is rejected")
	_, _, err = IndexArchiveObject(context.Background(), store, "standalone-v1/missing.tar.gz")
	require.ErrorIs(t, err, ErrObjectNotFound)
	require.Empty(t, fake.forbidden)
}

func tarGz(t *testing.T, entries ...*tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, hdr := range entries {
		require.NoError(t, tw.WriteHeader(hdr))
		if hdr.Typeflag == tar.TypeReg {
			_, err := tw.Write(make([]byte, hdr.Size))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestWalkArchiveRejectsUnsafeArchives(t *testing.T) {
	visit := func(string, string, int64, io.Reader) error { return nil }
	for _, hdr := range []*tar.Header{
		{Name: "roots/primary/../../etc/passwd", Typeflag: tar.TypeReg, Size: 1},
		{Name: "/etc/passwd", Typeflag: tar.TypeReg, Size: 1},
		{Name: "roots/primary/m/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "roots/../x", Typeflag: tar.TypeReg, Size: 1},
		{Name: "other/file", Typeflag: tar.TypeReg, Size: 1},
	} {
		err := walkArchive(bytes.NewReader(tarGz(t, hdr)), visit)
		require.ErrorIs(t, err, errUnsafeArchive, hdr.Name)
	}
	ok := tarGz(t, &tar.Header{Name: "roots/primary/m/jsonl/a.jsonl", Typeflag: tar.TypeReg, Size: 3},
		&tar.Header{Name: ArchiveManifestName, Typeflag: tar.TypeReg, Size: 2})
	var names []string
	require.NoError(t, walkArchive(bytes.NewReader(ok), func(root, rel string, _ int64, _ io.Reader) error {
		names = append(names, root+":"+rel)
		return nil
	}))
	assert.Equal(t, []string{"primary:m/jsonl/a.jsonl", ":" + ArchiveManifestName}, names, "the manifest is passed with an empty root id")

	// A body larger than any capture is refused before it is read.
	_, err := readBodyEntry(BodyFile{}, maxArchiveEntryBytes+1, strings.NewReader("x"))
	require.ErrorIs(t, err, errUnsafeArchive)
	// Decompressed output is bounded (gzip bomb).
	_, err = io.ReadAll(&expandLimitReader{r: strings.NewReader("0123456789"), left: 4})
	require.ErrorIs(t, err, errArchiveTooBig)
}
