package sessionrecorder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// The R2 client deliberately implements only HEAD, single PUT and the three
// multipart calls (initiate/upload part/complete). It has no code path that
// can delete, copy or overwrite a remote object.

const (
	metaSHA256       = "session-recorder-sha256"
	metaInstallation = "session-recorder-installation"
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// multipartPart is the multipart part size (a variable only for tests).
var multipartPart int64 = 16 << 20

// ObjectMetadata is the subset of HEAD we rely on.
type ObjectMetadata struct {
	Exists   bool
	Size     int64
	Metadata map[string]string
}

// ErrPreconditionFailed means the object already exists.
var ErrPreconditionFailed = errors.New("PreconditionFailed")

// httpStatusError is returned for non-success R2 responses.
type httpStatusError struct {
	Op     string
	Status int
	Code   string
}

func (e *httpStatusError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("r2 %s: status %d (%s)", e.Op, e.Status, e.Code)
	}
	return fmt.Sprintf("r2 %s: status %d", e.Op, e.Status)
}

func (e *httpStatusError) retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500 || e.Status == http.StatusRequestTimeout
}

type R2Store struct {
	endpoint *url.URL
	bucket   string
	region   string
	creds    aws.Credentials
	client   *http.Client
	signer   *v4.Signer

	maxConcurrency int
	adaptive       bool
	maxUploadMbps  int

	mu             sync.Mutex
	concurrency    int
	lastUploadMbps float64
}

// NewR2Store builds a path-style S3 client for the R2 endpoint.
func NewR2Store(cfg Config, creds Credentials) (*R2Store, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.R2.Endpoint), "/")
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errors.New("r2.endpoint: invalid URL")
	}
	if creds.State() != CredentialConfigured {
		return nil, fmt.Errorf("credentials %s", creds.State())
	}
	region := strings.TrimSpace(cfg.R2.Region)
	if region == "" {
		region = "auto"
	}
	start := min(2, cfg.MultipartConcurrency)
	if !cfg.AdaptiveUpload {
		start = cfg.MultipartConcurrency
	}
	return &R2Store{
		endpoint:       u,
		bucket:         cfg.R2.Bucket,
		region:         region,
		creds:          aws.Credentials{AccessKeyID: creds.AccessKeyID, SecretAccessKey: creds.SecretAccessKey, Source: "session-recorder"},
		client:         &http.Client{Timeout: 30 * time.Minute},
		signer:         v4.NewSigner(func(o *v4.SignerOptions) { o.DisableURIPathEscaping = true }),
		maxConcurrency: cfg.MultipartConcurrency,
		adaptive:       cfg.AdaptiveUpload,
		maxUploadMbps:  cfg.MaxUploadMbps,
		concurrency:    max(1, start),
	}, nil
}

// escapeKeyPath percent-encodes everything except RFC 3986 unreserved
// characters and '/', as S3 canonical URIs require.
func escapeKeyPath(p string) string {
	var b strings.Builder
	for i := range len(p) {
		c := p[i]
		if c == '/' || c == '-' || c == '_' || c == '.' || c == '~' ||
			('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func (s *R2Store) objectURL(key string, query url.Values) *url.URL {
	u := *s.endpoint
	plain := strings.TrimRight(u.Path, "/") + "/" + s.bucket + "/" + key
	u.Path = plain
	u.RawPath = escapeKeyPath(plain)
	u.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return &u
}

func (s *R2Store) do(ctx context.Context, method, key string, query url.Values, header http.Header, body io.Reader, size int64, payloadHash string) (*http.Response, error) {
	switch method {
	case http.MethodHead, http.MethodPut, http.MethodPost:
	default:
		return nil, fmt.Errorf("r2: method %s is not allowed", method)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.objectURL(key, query).String(), body)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if body != nil {
		req.ContentLength = size
	}
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if err := s.signer.SignHTTP(ctx, s.creds, req, payloadHash, "s3", s.region, time.Now().UTC()); err != nil {
		return nil, err
	}
	return s.client.Do(req)
}

type s3Error struct {
	Code string `xml:"Code"`
}

func statusError(op string, resp *http.Response) error {
	var parsed s3Error
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = xml.Unmarshal(raw, &parsed)
	if resp.StatusCode == http.StatusPreconditionFailed || parsed.Code == "PreconditionFailed" {
		return ErrPreconditionFailed
	}
	return &httpStatusError{Op: op, Status: resp.StatusCode, Code: parsed.Code}
}

// Head returns object metadata; a 404 is reported as Exists=false.
func (s *R2Store) Head(ctx context.Context, key string) (ObjectMetadata, error) {
	resp, err := s.do(ctx, http.MethodHead, key, nil, nil, nil, 0, emptyPayloadHash)
	if err != nil {
		return ObjectMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ObjectMetadata{}, nil
	}
	if resp.StatusCode/100 != 2 {
		return ObjectMetadata{}, &httpStatusError{Op: "head", Status: resp.StatusCode}
	}
	meta := make(map[string]string)
	for k, v := range resp.Header {
		lower := strings.ToLower(k)
		if name, ok := strings.CutPrefix(lower, "x-amz-meta-"); ok && len(v) > 0 {
			meta[name] = v[0]
		}
	}
	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if resp.ContentLength >= 0 {
		size = resp.ContentLength
	}
	return ObjectMetadata{Exists: true, Size: size, Metadata: meta}, nil
}

// PutBytes performs a conditional single PUT of a small in-memory body.
func (s *R2Store) PutBytes(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	sum := sha256.Sum256(data)
	header := metadataHeader(metadata)
	header.Set("If-None-Match", "*")
	resp, err := s.do(ctx, http.MethodPut, key, nil, header, bytes.NewReader(data), int64(len(data)), hex.EncodeToString(sum[:]))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return statusError("put", resp)
	}
	return nil
}

func metadataHeader(metadata map[string]string) http.Header {
	header := http.Header{}
	for k, v := range metadata {
		header.Set("X-Amz-Meta-"+k, v)
	}
	return header
}

// PutFile uploads an archive with If-None-Match: *. Archives smaller than
// one part use a single PUT, larger ones use multipart upload.
func (s *R2Store) PutFile(ctx context.Context, key, file string, size int64, sha string) error {
	metadata := map[string]string{metaSHA256: sha}
	started := time.Now()
	var err error
	if size < multipartPart {
		err = s.putSingle(ctx, key, file, size, sha, metadata)
	} else {
		err = s.putMultipart(ctx, key, file, size, metadata)
	}
	s.observe(err, size, time.Since(started))
	return err
}

func (s *R2Store) putSingle(ctx context.Context, key, file string, size int64, sha string, metadata map[string]string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	header := metadataHeader(metadata)
	header.Set("If-None-Match", "*")
	resp, err := s.do(ctx, http.MethodPut, key, nil, header, s.limit(f), size, sha)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return statusError("put", resp)
	}
	return nil
}

type initiateResult struct {
	UploadID string `xml:"UploadId"`
}

type completedPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type completeRequest struct {
	XMLName xml.Name        `xml:"CompleteMultipartUpload"`
	Parts   []completedPart `xml:"Part"`
}

func (s *R2Store) putMultipart(ctx context.Context, key, file string, size int64, metadata map[string]string) error {
	query := url.Values{"uploads": {""}}
	resp, err := s.do(ctx, http.MethodPost, key, query, metadataHeader(metadata), nil, 0, emptyPayloadHash)
	if err != nil {
		return err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return &httpStatusError{Op: "create multipart", Status: resp.StatusCode}
	}
	var initiated initiateResult
	if err := xml.Unmarshal(raw, &initiated); err != nil || initiated.UploadID == "" {
		return errors.New("r2 create multipart: missing upload id")
	}
	partCount := int((size + multipartPart - 1) / multipartPart)
	parts := make([]completedPart, partCount)
	errs := make([]error, partCount)
	partCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, max(1, s.currentConcurrency()))
	var wg sync.WaitGroup
	for i := range partCount {
		select {
		case sem <- struct{}{}:
		case <-partCtx.Done():
		}
		if partCtx.Err() != nil {
			break
		}
		wg.Go(func() {
			defer func() { <-sem }()
			offset := int64(i) * multipartPart
			length := min(multipartPart, size-offset)
			etag, err := s.uploadPart(partCtx, key, initiated.UploadID, file, i+1, offset, length)
			if err != nil {
				errs[i] = err
				cancel()
				return
			}
			parts[i] = completedPart{PartNumber: i + 1, ETag: etag}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := xml.Marshal(completeRequest{Parts: parts})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	header := http.Header{}
	header.Set("If-None-Match", "*")
	header.Set("Content-Type", "application/xml")
	resp, err = s.do(ctx, http.MethodPost, key, url.Values{"uploadId": {initiated.UploadID}}, header, bytes.NewReader(body), int64(len(body)), hex.EncodeToString(sum[:]))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return statusError("complete multipart", resp)
	}
	// S3 may report a failed completion with 200 and an <Error> body.
	raw, _ = io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var parsed s3Error
	if xml.Unmarshal(raw, &parsed) == nil && parsed.Code != "" {
		if parsed.Code == "PreconditionFailed" {
			return ErrPreconditionFailed
		}
		return &httpStatusError{Op: "complete multipart", Status: resp.StatusCode, Code: parsed.Code}
	}
	return nil
}

func (s *R2Store) uploadPart(ctx context.Context, key, uploadID, file string, number int, offset, length int64) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(f, offset, length)); err != nil {
		return "", err
	}
	query := url.Values{"partNumber": {strconv.Itoa(number)}, "uploadId": {uploadID}}
	resp, err := s.do(ctx, http.MethodPut, key, query, nil, s.limit(io.NewSectionReader(f, offset, length)), length, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", statusError("upload part", resp)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", errors.New("r2 upload part: missing etag")
	}
	return etag, nil
}

func (s *R2Store) currentConcurrency() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.concurrency
}

// SetBusy forces a single upload stream while the producer queue is busy.
func (s *R2Store) SetBusy(busy bool) {
	if !busy || !s.adaptive {
		return
	}
	s.mu.Lock()
	s.concurrency = 1
	s.mu.Unlock()
}

// observe adapts multipart concurrency: grow slowly on success, halve on
// throttling or failure.
func (s *R2Store) observe(err error, size int64, elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && elapsed > 0 {
		s.lastUploadMbps = float64(size*8) / elapsed.Seconds() / 1e6
	}
	if !s.adaptive {
		return
	}
	if err != nil && !errors.Is(err, ErrPreconditionFailed) {
		s.concurrency = max(1, s.concurrency/2)
		return
	}
	if s.maxUploadMbps > 0 && s.lastUploadMbps >= float64(s.maxUploadMbps)*0.9 {
		return
	}
	s.concurrency = min(s.maxConcurrency, s.concurrency+1)
}

// LastUploadMbps reports the throughput of the last successful upload.
func (s *R2Store) LastUploadMbps() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUploadMbps
}

// bandwidthReader throttles reads to bytesPerSecond.
type bandwidthReader struct {
	reader         io.Reader
	bytesPerSecond int64
	mu             *sync.Mutex
	started        *time.Time
	read           *int64
}

func (r bandwidthReader) Read(p []byte) (int, error) {
	if len(p) > 256<<10 {
		p = p[:256<<10]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.mu.Lock()
		*r.read += int64(n)
		expected := time.Duration(float64(*r.read) / float64(r.bytesPerSecond) * float64(time.Second))
		wait := expected - time.Since(*r.started)
		r.mu.Unlock()
		if wait > 0 {
			time.Sleep(wait)
		}
	}
	return n, err
}

var (
	limiterMu      sync.Mutex
	limiterStarted = time.Now()
	limiterRead    int64
)

func (s *R2Store) limit(r io.Reader) io.Reader {
	if s.maxUploadMbps <= 0 {
		return r
	}
	limiterMu.Lock()
	// Restart the shared window when it has been idle, so bursts are not
	// "saved up" across long pauses.
	rate := int64(s.maxUploadMbps) * 1_000_000 / 8
	if time.Since(limiterStarted) > time.Duration(float64(limiterRead)/float64(rate)*float64(time.Second))+5*time.Second {
		limiterStarted = time.Now()
		limiterRead = 0
	}
	limiterMu.Unlock()
	return bandwidthReader{reader: r, bytesPerSecond: rate, mu: &limiterMu, started: &limiterStarted, read: &limiterRead}
}
