package sessionrecorder

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ArchiveManifestName is the last entry of every archive.
const ArchiveManifestName = "SESSION-RECORDER-MANIFEST.json"

type archiveManifest struct {
	FormatVersion int                  `json:"format_version"`
	BatchID       string               `json:"batch_id"`
	CreatedAt     string               `json:"created_at"`
	Roots         map[string]string    `json:"roots"`
	Sources       []archiveSourceEntry `json:"sources"`
}

type archiveSourceEntry struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	ModTimeNanos int64  `json:"mod_time_unix_nano"`
	Mode         uint32 `json:"mode"`
}

var errSourceChanged = errors.New("source changed")

type countingWriter struct {
	written int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	return len(p), nil
}

func archiveEntryName(rootID, rel string) string {
	return path.Join("roots", rootID, filepath.ToSlash(rel))
}

func rootPaths(cfg Config) map[string]string {
	roots := make(map[string]string, len(cfg.SpoolRoots))
	for _, root := range cfg.SpoolRoots {
		roots[root.RootID] = root.Path
	}
	return roots
}

// hasSymlinkComponent reports whether any path element between root
// (exclusive) and target (inclusive) is a symlink.
func hasSymlinkComponent(root, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, err
	}
	current := root
	for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}

// openVerifiedSource opens a spool file and checks it still matches the
// planned entry.
func openVerifiedSource(entry SourceEntry) (*os.File, fs.FileInfo, error) {
	file, err := openNoFollow(entry.AbsolutePath)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, errors.New("not a regular file")
	}
	if info.Size() != entry.Size || info.ModTime().UnixNano() != entry.ModTimeNanos {
		file.Close()
		return nil, nil, errSourceChanged
	}
	return file, info, nil
}

// writeSourceEntry streams one source file into the tar writer.
func writeSourceEntry(tw *tar.Writer, entry SourceEntry, rootPath string) error {
	symlink, err := hasSymlinkComponent(rootPath, entry.AbsolutePath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", entry.AbsolutePath, err)
	}
	if symlink {
		return fmt.Errorf("source path contains symlink: %s", entry.AbsolutePath)
	}
	file, info, err := openVerifiedSource(entry)
	if err != nil {
		if errors.Is(err, errSourceChanged) {
			return fmt.Errorf("source changed while archiving: %s", entry.AbsolutePath)
		}
		return fmt.Errorf("open source %s: %w", entry.AbsolutePath, err)
	}
	defer file.Close()
	header := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     archiveEntryName(entry.RootID, entry.RelativePath),
		Size:     info.Size(),
		Mode:     int64(info.Mode() & 0o777),
		ModTime:  info.ModTime(),
		Format:   tar.FormatPAX,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	copied, err := io.Copy(tw, file)
	if err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if copied != entry.Size || after.Size() != entry.Size || after.ModTime().UnixNano() != entry.ModTimeNanos {
		return fmt.Errorf("source changed while archiving: %s", entry.AbsolutePath)
	}
	return nil
}

func archivePath(workDir, batchID string) string {
	return filepath.Join(workDir, "archives", batchID+".tar.gz")
}

// CreateArchive builds work_dir/archives/<batch_id>.tar.gz for a planned
// batch: every source as roots/<root_id>/<rel> (PAX headers) followed by
// SESSION-RECORDER-MANIFEST.json, gzip compressed at the configured level.
func CreateArchive(m *BatchManifest, cfg Config, now time.Time) error {
	if m.State != StatePlanned {
		return errors.New("archive requires planned state")
	}
	if err := os.MkdirAll(filepath.Join(cfg.WorkDir, "archives"), 0o700); err != nil {
		return err
	}
	final := archivePath(cfg.WorkDir, m.BatchID)
	tmp := final + ".tmp"
	for _, stale := range []string{tmp, final} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			out.Close()
			os.Remove(tmp)
		}
	}()
	gz, err := gzip.NewWriterLevel(out, cfg.GzipLevel)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(gz)
	lookup := rootPaths(cfg)
	roots := make(map[string]string)
	sources := make([]archiveSourceEntry, 0, len(m.Sources))
	for _, entry := range m.Sources {
		rootPath, ok := lookup[entry.RootID]
		if !ok {
			return fmt.Errorf("open source %s: unknown root %s", entry.AbsolutePath, entry.RootID)
		}
		if err := writeSourceEntry(tw, entry, rootPath); err != nil {
			return err
		}
		roots[entry.RootID] = "roots/" + entry.RootID
		sources = append(sources, archiveSourceEntry{
			RootID:       entry.RootID,
			RelativePath: filepath.ToSlash(entry.RelativePath),
			Size:         entry.Size,
			ModTimeNanos: entry.ModTimeNanos,
			Mode:         entry.Mode,
		})
	}
	now = now.UTC()
	raw, err := common.Marshal(archiveManifest{
		FormatVersion: 1,
		BatchID:       m.BatchID,
		CreatedAt:     now.Format(timeLayout),
		Roots:         roots,
		Sources:       sources,
	})
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     ArchiveManifestName,
		Size:     int64(len(raw)),
		Mode:     0o600,
		ModTime:  now,
		Format:   tar.FormatPAX,
	}); err != nil {
		return err
	}
	if _, err := tw.Write(raw); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	committed = true
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	size, sum, err := hashFile(final)
	if err != nil {
		return err
	}
	m.ArchivePath = final
	m.ArchiveSize = size
	m.ArchiveSHA256 = sum
	m.Transition(StateArchived, now)
	return nil
}

func hashFile(name string) (int64, string, error) {
	file, err := os.Open(name)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	h := sha256.New()
	size, err := io.Copy(h, file)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

// SelectUnitsByCompressedTarget picks units in order until the estimated
// compressed size reaches target, the file limit or the input byte limit.
// The unit that crosses the target is included, and at least one unit is
// always selected so an oversized unit cannot block the queue.
func SelectUnitsByCompressedTarget(units []SelectionUnit, cfg Config) ([]SelectionUnit, error) {
	counter := &countingWriter{}
	gz, err := gzip.NewWriterLevel(counter, cfg.GzipLevel)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(gz)
	lookup := rootPaths(cfg)
	seen := make(map[string]struct{})
	var selected []SelectionUnit
	files := 0
	var input int64
	for _, unit := range units {
		unitFiles := 0
		var unitBytes int64
		for _, entry := range unit.Entries {
			if _, ok := seen[entry.AbsolutePath]; !ok {
				unitFiles++
				unitBytes += entry.Size
			}
		}
		if len(selected) > 0 && (files+unitFiles > cfg.MaxBatchFiles || input+unitBytes > cfg.MaxBatchInputBytes) {
			break
		}
		for _, entry := range unit.Entries {
			if _, ok := seen[entry.AbsolutePath]; ok {
				continue
			}
			seen[entry.AbsolutePath] = struct{}{}
			if err := writeSourceEntry(tw, entry, lookup[entry.RootID]); err != nil {
				if strings.HasPrefix(err.Error(), "source changed while archiving") {
					return nil, fmt.Errorf("source changed while sizing: %s", entry.AbsolutePath)
				}
				return nil, err
			}
		}
		if err := tw.Flush(); err != nil {
			return nil, err
		}
		if err := gz.Flush(); err != nil {
			return nil, err
		}
		selected = append(selected, unit)
		files += unitFiles
		input += unitBytes
		if counter.written >= cfg.TargetArchiveBytes {
			break
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return selected, nil
}
