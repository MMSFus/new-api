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

func TestProducerWritesSpoolProtocol(t *testing.T) {
	withStorageUsage(t, 10, 10)
	s := DefaultSettings()
	s.SpoolRoot = filepath.Join(t.TempDir(), "spool")
	s.StatsPath = filepath.Join(filepath.Dir(s.SpoolRoot), "stats.json")
	p := NewProducer(s)
	p.Start()
	finished := time.Date(2026, 9, 28, 5, 17, 0, 0, time.UTC)
	ok := p.Enqueue(&Capture{
		Record:          Record{RecordID: "req-1", SessionID: "sess/1", Model: "gpt-4o", Stream: true},
		RequestBody:     []byte(`{"model":"gpt-4o"}`),
		ResponseBody:    []byte("data: hi\n\n"),
		IncludeRequest:  true,
		IncludeResponse: true,
		FinishedAt:      finished,
	}, 0)
	require.True(t, ok)
	p.Stop(5 * time.Second)
	require.EqualValues(t, 1, p.Counters().Written)

	shard := filepath.Join(s.SpoolRoot, "gpt-4o", "jsonl", "2026", "09", "28", "05", "10-"+p.instance+".jsonl")
	raw, err := os.ReadFile(shard)
	require.NoError(t, err)
	require.True(t, bytes.HasSuffix(raw, []byte("\n")))
	var rec Record
	require.NoError(t, common.Unmarshal(bytes.TrimSpace(raw), &rec))
	require.Len(t, rec.BodyFiles, 2)
	require.True(t, strings.HasPrefix(rec.BodyFiles[0].File, "body/2026/09/28/05/"))
	require.True(t, strings.HasSuffix(rec.BodyFiles[1].File, "_response.sse"))
	for _, b := range rec.BodyFiles {
		_, err := os.Stat(filepath.Join(s.SpoolRoot, "gpt-4o", filepath.FromSlash(b.File)))
		require.NoError(t, err)
		require.False(t, strings.HasSuffix(b.File, ".json"), "body files must not look like records")
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
