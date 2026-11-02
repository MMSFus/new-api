package sessionrecorder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	backfillCheckpointName = "index-backfill-checkpoint.json"
	archiveObjectSuffix    = ".tar.gz"
	// maxArchiveManifestBytes bounds the manifest entry read from an archive.
	maxArchiveManifestBytes = 64 << 20
)

// ArchiveObjectStore is what historical backfill needs from R2.
// *R2Store implements it.
type ArchiveObjectStore interface {
	ArchiveGetter
	Head(ctx context.Context, key string) (ObjectMetadata, error)
	ListObjects(ctx context.Context, prefix, continuation string) (ObjectPage, error)
}

// ListArchiveObjects lists every batch archive under object_prefix, following
// continuation tokens. Connectivity probes under <prefix>/_checks/ and any
// other non-archive objects are left out.
func ListArchiveObjects(ctx context.Context, store ArchiveObjectStore, objectPrefix string) ([]string, error) {
	prefix := strings.Trim(strings.TrimSpace(objectPrefix), "/")
	if prefix == "" {
		return nil, errors.New("r2.object_prefix: required")
	}
	prefix += "/"
	checks := prefix + "_checks/"
	var keys []string
	continuation := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := store.ListObjects(ctx, prefix, continuation)
		if err != nil {
			return nil, err
		}
		for _, key := range page.Keys {
			if strings.HasPrefix(key, prefix) && !strings.HasPrefix(key, checks) && strings.HasSuffix(key, archiveObjectSuffix) {
				keys = append(keys, key)
			}
		}
		if page.NextContinuation == "" {
			return keys, nil
		}
		continuation = page.NextContinuation
	}
}

// BatchIDFromObjectKey is the batch id encoded in an archive object key
// (<prefix>/<site>/<host>/<YYYY/MM/DD/HH>/<batch_id>.tar.gz).
func BatchIDFromObjectKey(key string) string {
	return strings.TrimSuffix(path.Base(key), archiveObjectSuffix)
}

// IndexArchiveObject downloads one archive and returns its index rows. The
// archive size is the object size reported by HEAD, so archive bytes of the
// rows sum to exactly that size. The batch id comes from the manifest inside
// the archive, falling back to the object key. When the object carries the
// sha256 written at upload, the download is checked against it.
func IndexArchiveObject(ctx context.Context, store ArchiveObjectStore, key string) (string, []IndexRow, error) {
	meta, err := store.Head(ctx, key)
	if err != nil {
		return "", nil, err
	}
	if !meta.Exists {
		return "", nil, ErrObjectNotFound
	}
	body, _, err := store.Get(ctx, key)
	if err != nil {
		return "", nil, err
	}
	defer body.Close()
	hash := sha256.New()
	counted := &countingWriter{}
	tee := io.TeeReader(io.LimitReader(body, maxArchiveDownloadBytes+1), io.MultiWriter(hash, counted))
	index := newBatchIndex()
	var inner archiveManifest
	err = walkArchive(tee, func(rootID, rel string, size int64, entry io.Reader) error {
		if rootID == "" {
			raw, err := io.ReadAll(io.LimitReader(entry, maxArchiveManifestBytes+1))
			if err != nil {
				return err
			}
			if len(raw) > maxArchiveManifestBytes {
				return errUnsafeArchive
			}
			// A malformed manifest only loses the batch id and timestamp.
			_ = common.Unmarshal(raw, &inner)
			return nil
		}
		index.setSize(rootID, rel, size)
		if !isRecordFile(rel) {
			return nil
		}
		return index.addRecordFile(rootID, rel, entry)
	})
	if err != nil {
		return "", nil, err
	}
	if _, err := io.Copy(io.Discard, tee); err != nil {
		return "", nil, err
	}
	if counted.written > maxArchiveDownloadBytes {
		return "", nil, errArchiveTooBig
	}
	if counted.written != meta.Size {
		return "", nil, errors.New("archive size does not match the object")
	}
	if sum := meta.Metadata[metaSHA256]; sum != "" && !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), sum) {
		return "", nil, errors.New("archive checksum mismatch")
	}
	batchID := strings.TrimSpace(inner.BatchID)
	if batchID == "" || !safeKeySegment(batchID) {
		batchID = BatchIDFromObjectKey(key)
	}
	rows := index.rows(BatchManifest{BatchID: batchID, ObjectKey: key, ArchiveSize: meta.Size, CreatedAt: inner.CreatedAt})
	return batchID, rows, nil
}

// BackfillCheckpoint records the archive object keys historical backfill has
// finished, so a cancelled run resumes where it stopped.
type BackfillCheckpoint struct {
	Done map[string]bool `json:"done_objects"`
}

func LoadBackfillCheckpoint(workDir string) BackfillCheckpoint {
	checkpoint := BackfillCheckpoint{Done: map[string]bool{}}
	raw, err := os.ReadFile(filepath.Join(workDir, backfillCheckpointName))
	if err == nil {
		_ = common.Unmarshal(raw, &checkpoint)
	}
	if checkpoint.Done == nil {
		checkpoint.Done = map[string]bool{}
	}
	return checkpoint
}

func SaveBackfillCheckpoint(workDir string, checkpoint BackfillCheckpoint) error {
	raw, err := common.Marshal(checkpoint)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(workDir, backfillCheckpointName), raw, 0o600)
}
