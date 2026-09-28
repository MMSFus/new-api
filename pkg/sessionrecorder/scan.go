package sessionrecorder

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const cleanupDirName = ".session-recorder-cleanup"

// SelectionUnit is one record file together with every attachment it
// references. Units are archived atomically.
type SelectionUnit struct {
	Record  SourceEntry
	Entries []SourceEntry
}

// ScanIssue is a problem found while scanning; it never stops the scan.
type ScanIssue struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}

// ScanResult is the output of one spool scan.
type ScanResult struct {
	Units        []SelectionUnit
	Issues       []ScanIssue
	ScannedFiles int
	OrphanMedia  int
	BacklogFiles int
	BacklogBytes int64
}

// SkipFunc reports whether a stable source was already handled by an earlier
// batch and must not be uploaded again.
type SkipFunc func(SourceEntry) bool

type recordRefs struct {
	Attachments []struct {
		File string `json:"file"`
	} `json:"attachments"`
	BodyFiles []struct {
		File string `json:"file"`
	} `json:"body_files"`
}

func isRecordFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".json" || ext == ".jsonl"
}

// stableSourceEntry validates a spool file and describes it. The returned
// category is empty on success.
func stableSourceEntry(rootID, rootPath, abs string, cutoff time.Time) (SourceEntry, string) {
	if !pathInside(rootPath, abs) {
		return SourceEntry{}, "attachment_outside_root"
	}
	if symlink, err := hasSymlinkComponent(rootPath, abs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SourceEntry{}, "missing_attachment"
		}
		return SourceEntry{}, "stat_error"
	} else if symlink {
		return SourceEntry{}, "unsupported_file_type"
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SourceEntry{}, "missing_attachment"
		}
		return SourceEntry{}, "stat_error"
	}
	if info.Mode()&fs.ModeType != 0 {
		return SourceEntry{}, "unsupported_file_type"
	}
	if info.ModTime().After(cutoff) {
		return SourceEntry{}, "unstable_file"
	}
	rel, err := filepath.Rel(rootPath, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return SourceEntry{}, "outside_root"
	}
	return SourceEntry{
		RootID:       rootID,
		AbsolutePath: abs,
		RelativePath: filepath.ToSlash(rel),
		Size:         info.Size(),
		ModTimeNanos: info.ModTime().UnixNano(),
		Mode:         uint32(info.Mode()),
		Cleanup:      CleanupPending,
	}, ""
}

// readRecordRefs returns every attachment/body file referenced by a record
// file. JSONL files must end with a newline, otherwise the writer may still
// be appending to them.
func readRecordRefs(file string) ([]string, string) {
	f, err := openNoFollow(file)
	if err != nil {
		return nil, "read_error"
	}
	defer f.Close()
	var refs []string
	collect := func(line []byte) bool {
		var r recordRefs
		if err := common.Unmarshal(line, &r); err != nil {
			return false
		}
		for _, a := range r.Attachments {
			if a.File != "" {
				refs = append(refs, a.File)
			}
		}
		for _, b := range r.BodyFiles {
			if b.File != "" {
				refs = append(refs, b.File)
			}
		}
		return true
	}
	if strings.ToLower(filepath.Ext(file)) == ".json" {
		raw, err := io.ReadAll(f)
		if err != nil {
			return nil, "read_error"
		}
		if !collect(raw) {
			return nil, "malformed_record"
		}
		return refs, ""
	}
	reader := bufio.NewReaderSize(f, 256<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] != '\n' {
			return nil, "incomplete_jsonl"
		}
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 && !collect(trimmed) {
			return nil, "malformed_record"
		}
		if errors.Is(err, io.EOF) {
			return refs, ""
		}
		if err != nil {
			return nil, "read_error"
		}
	}
}

// resolveAttachmentReference maps attachments[].file (relative to
// <root>/<model>) to an absolute path.
func resolveAttachmentReference(rootPath, recordRel, ref string) (string, bool) {
	if ref == "" || strings.Contains(ref, "\\") || path.IsAbs(ref) {
		return "", false
	}
	model, _, _ := strings.Cut(filepath.ToSlash(recordRel), "/")
	joined := path.Clean(path.Join(model, ref))
	if joined == "." || joined == ".." || strings.HasPrefix(joined, "../") {
		return "", false
	}
	abs := filepath.Join(rootPath, filepath.FromSlash(joined))
	return abs, pathInside(rootPath, abs)
}

// ScanSpools walks every spool root and returns stable selection units.
func ScanSpools(cfg Config, now time.Time, skip SkipFunc, extraSkipPaths ...string) ScanResult {
	var result ScanResult
	cutoff := now.Add(-time.Duration(cfg.StabilityWindowSeconds) * time.Second)
	skipPaths := make(map[string]struct{})
	for _, p := range extraSkipPaths {
		if p != "" {
			skipPaths[filepath.Clean(p)] = struct{}{}
		}
	}
	for _, root := range cfg.SpoolRoots {
		scanRoot(root, cfg, cutoff, skip, skipPaths, &result)
	}
	result.Units = mergeOverlappingUnits(result.Units)
	for _, unit := range result.Units {
		result.BacklogFiles += len(unit.Entries)
		for _, e := range unit.Entries {
			result.BacklogBytes += e.Size
		}
	}
	return result
}

func scanRoot(root SpoolRoot, cfg Config, cutoff time.Time, skip SkipFunc, skipPaths map[string]struct{}, result *ScanResult) {
	rootPath := filepath.Clean(root.Path)
	if _, err := os.Stat(rootPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			result.Issues = append(result.Issues, ScanIssue{Path: rootPath, Category: "filesystem_error"})
		}
		return
	}
	quarantine := filepath.Join(rootPath, cleanupDirName)
	var records, others []string
	_ = filepath.WalkDir(rootPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			result.Issues = append(result.Issues, ScanIssue{Path: p, Category: "walk_error"})
			if d != nil && d.IsDir() && p != rootPath {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p == quarantine {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := skipPaths[filepath.Clean(p)]; ok {
			return nil
		}
		result.ScannedFiles++
		if isRecordFile(d.Name()) {
			records = append(records, p)
		} else {
			others = append(others, p)
		}
		return nil
	})
	referenced := make(map[string]struct{})
	for _, recordPath := range records {
		entry, category := stableSourceEntry(root.RootID, rootPath, recordPath, cutoff)
		if category == "unstable_file" {
			// Still being written; keep its references out of the orphan count.
			if refs, cat := readRecordRefs(recordPath); cat == "" {
				rel, _ := filepath.Rel(rootPath, recordPath)
				for _, ref := range refs {
					if abs, ok := resolveAttachmentReference(rootPath, rel, ref); ok {
						referenced[abs] = struct{}{}
					}
				}
			}
			continue
		}
		if category != "" {
			result.Issues = append(result.Issues, ScanIssue{Path: recordPath, Category: category})
			continue
		}
		refs, category := readRecordRefs(recordPath)
		if category != "" {
			result.Issues = append(result.Issues, ScanIssue{Path: recordPath, Category: category})
			continue
		}
		unit, deferred := scanRecordUnit(root, rootPath, entry, refs, cutoff, skip, referenced, result)
		if deferred || (skip != nil && skip(entry)) {
			continue
		}
		result.Units = append(result.Units, unit)
	}
	for _, other := range others {
		if _, ok := referenced[other]; ok {
			continue
		}
		info, err := os.Lstat(other)
		if err == nil && !info.ModTime().After(cutoff) {
			result.OrphanMedia++
		}
	}
}

func scanRecordUnit(root SpoolRoot, rootPath string, record SourceEntry, refs []string, cutoff time.Time, skip SkipFunc, referenced map[string]struct{}, result *ScanResult) (SelectionUnit, bool) {
	unit := SelectionUnit{Record: record, Entries: []SourceEntry{record}}
	seen := map[string]struct{}{record.AbsolutePath: {}}
	deferred := false
	for _, ref := range refs {
		abs, ok := resolveAttachmentReference(rootPath, record.RelativePath, ref)
		if !ok {
			result.Issues = append(result.Issues, ScanIssue{Path: record.AbsolutePath, Category: "attachment_outside_root"})
			continue
		}
		referenced[abs] = struct{}{}
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		entry, category := stableSourceEntry(root.RootID, rootPath, abs, cutoff)
		switch category {
		case "":
			if skip != nil && skip(entry) {
				continue
			}
			unit.Entries = append(unit.Entries, entry)
		case "unstable_file":
			deferred = true
		default:
			result.Issues = append(result.Issues, ScanIssue{Path: abs, Category: category})
		}
	}
	return unit, deferred
}

// mergeOverlappingUnits joins units that share an attachment so a shared
// file is archived exactly once, and orders units by record path.
func mergeOverlappingUnits(units []SelectionUnit) []SelectionUnit {
	parent := make([]int, len(units))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	owner := make(map[string]int)
	for i, unit := range units {
		for _, e := range unit.Entries {
			if j, ok := owner[e.AbsolutePath]; ok {
				a, b := find(i), find(j)
				if a != b {
					parent[max(a, b)] = min(a, b)
				}
				continue
			}
			owner[e.AbsolutePath] = i
		}
	}
	groups := make(map[int]*SelectionUnit)
	var order []int
	for i, unit := range units {
		r := find(i)
		g, ok := groups[r]
		if !ok {
			copyUnit := SelectionUnit{Record: unit.Record}
			groups[r] = &copyUnit
			g = &copyUnit
			order = append(order, r)
		}
		for _, e := range unit.Entries {
			if !containsEntry(g.Entries, e.AbsolutePath) {
				g.Entries = append(g.Entries, e)
			}
		}
	}
	merged := make([]SelectionUnit, 0, len(order))
	for _, r := range order {
		merged = append(merged, *groups[r])
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Record.RootID != merged[j].Record.RootID {
			return merged[i].Record.RootID < merged[j].Record.RootID
		}
		return merged[i].Record.RelativePath < merged[j].Record.RelativePath
	})
	return merged
}

func containsEntry(entries []SourceEntry, abs string) bool {
	for _, e := range entries {
		if e.AbsolutePath == abs {
			return true
		}
	}
	return false
}

// UnitSources flattens selected units into de-duplicated batch sources.
func UnitSources(units []SelectionUnit) []SourceEntry {
	seen := make(map[string]struct{})
	var out []SourceEntry
	for _, unit := range units {
		for _, e := range unit.Entries {
			if _, ok := seen[e.AbsolutePath]; ok {
				continue
			}
			seen[e.AbsolutePath] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}
