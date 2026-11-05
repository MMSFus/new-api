package sessionrecorder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// TerminalBatch lists the spool sources of one cleaned or retained batch.
type TerminalBatch struct {
	// Scope separates consumers that share one database.
	Scope   string
	BatchID string
	// SourceKeys are hex encoded source identities.
	SourceKeys []string
	// ExpiresAt is Unix seconds; 0 keeps the sources until the batch is deleted.
	ExpiresAt int64
}

// SourceLedger persists the sources of terminal batches, so a scan skips
// them without reading every historical manifest. Like IndexSink, the
// package never touches the database; the host installs a ledger with
// SetSourceLedger.
//
// AddTerminalBatch must be idempotent. ContainsSource ignores sources whose
// batch expired at or before now.
type SourceLedger interface {
	AddTerminalBatch(ctx context.Context, batch TerminalBatch) error
	ContainsSource(ctx context.Context, scope, sourceKey string, now int64) (bool, error)
	ExpiredBatches(ctx context.Context, scope string, now int64, limit int) ([]string, error)
	DeleteBatch(ctx context.Context, scope, batchID string) error
}

type sourceLedgerHolder struct{ ledger SourceLedger }

var sourceLedger atomic.Pointer[sourceLedgerHolder]

// SetSourceLedger installs the terminal source store; nil removes it, and the
// consumer then stops before planning new batches.
func SetSourceLedger(ledger SourceLedger) {
	if ledger == nil {
		sourceLedger.Store(nil)
		return
	}
	sourceLedger.Store(&sourceLedgerHolder{ledger: ledger})
}

func currentSourceLedger() SourceLedger {
	if h := sourceLedger.Load(); h != nil {
		return h.ledger
	}
	return nil
}

var (
	errSourceLedger            = errors.New("session recorder: source ledger")
	errSourceLedgerUnavailable = fmt.Errorf("%w is not installed", errSourceLedger)
)

// cycleErrorState names a failed cycle step for the status report.
func cycleErrorState(err error) string {
	if errors.Is(err, errSourceLedger) {
		return "source_ledger_error"
	}
	return "filesystem_error"
}

func ledgerError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errSourceLedger, err)
}

// sourceIndex binds the ledger to one consumer's scope for a cycle.
type sourceIndex struct {
	ctx    context.Context
	ledger SourceLedger
	scope  string
}

// sourceLedgerScope is the site and host pair that also names the batch
// fingerprint and R2 keys, hashed to a fixed width.
func sourceLedgerScope(cfg Config) string {
	sum := sha256.Sum256([]byte(cfg.SiteID + "\x00" + cfg.HostID))
	return hex.EncodeToString(sum[:16])
}

func (i *sourceIndex) contains(identity [16]byte, now time.Time) (bool, error) {
	known, err := i.ledger.ContainsSource(i.ctx, i.scope, hex.EncodeToString(identity[:]), now.Unix())
	return known, ledgerError(err)
}

func (i *sourceIndex) add(m BatchManifest) error {
	batch := TerminalBatch{Scope: i.scope, BatchID: m.BatchID, SourceKeys: make([]string, 0, len(m.Sources))}
	if updated, err := time.Parse(time.RFC3339Nano, m.UpdatedAt); err == nil && m.State == StateCleaned {
		batch.ExpiresAt = updated.Add(cleanedManifestRetention).Unix()
	}
	for _, source := range m.Sources {
		identity := sourceIdentity(source)
		batch.SourceKeys = append(batch.SourceKeys, hex.EncodeToString(identity[:]))
	}
	return ledgerError(i.ledger.AddTerminalBatch(i.ctx, batch))
}

func (i *sourceIndex) moveManifest(workDir string, m BatchManifest) error {
	source := manifestPath(workDir, m.BatchID)
	target := terminalManifestPath(workDir, m.BatchID)
	if err := os.Rename(source, target); err != nil {
		if os.IsNotExist(err) {
			if _, statErr := os.Stat(target); statErr == nil {
				return nil
			}
		}
		return err
	}
	return nil
}

// expireCleaned removes each expired manifest before its ledger rows, so a
// crash between the two retries the delete on the next cycle.
func (i *sourceIndex) expireCleaned(workDir string, now time.Time) error {
	for {
		expired, err := i.ledger.ExpiredBatches(i.ctx, i.scope, now.Unix(), 256)
		if err != nil {
			return ledgerError(err)
		}
		if len(expired) == 0 {
			return nil
		}
		for _, id := range expired {
			if err := os.Remove(terminalManifestPath(workDir, id)); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := i.ledger.DeleteBatch(i.ctx, i.scope, id); err != nil {
				return ledgerError(err)
			}
		}
		if len(expired) < 256 {
			return nil
		}
	}
}
