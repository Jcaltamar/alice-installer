package installation

import (
	"context"
	"errors"
	"fmt"
)

type pm2InventorySnapshot interface {
	Snapshot(context.Context) ([]PM2Record, error)
}

type socketOwnershipSnapshot interface {
	Snapshot(context.Context) ([]SocketOwner, error)
}

type procIdentityReader interface {
	Read(context.Context, int) (ProcIdentity, error)
}

type LinuxPM2SnapshotProvider struct {
	Inventory pm2InventorySnapshot
	Sockets   socketOwnershipSnapshot
	Proc      procIdentityReader
}

func (p LinuxPM2SnapshotProvider) Snapshot(ctx context.Context) (PM2Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return PM2Snapshot{}, err
	}
	if p.Inventory == nil || p.Sockets == nil || p.Proc == nil {
		return PM2Snapshot{}, errors.New("pm2 snapshot provider is incomplete")
	}
	records, err := p.Inventory.Snapshot(ctx)
	if err != nil {
		return PM2Snapshot{}, fmt.Errorf("pm2 inventory is unavailable: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return PM2Snapshot{}, err
	}
	sockets, err := p.Sockets.Snapshot(ctx)
	if err != nil {
		return PM2Snapshot{}, fmt.Errorf("socket ownership is unavailable: %w", err)
	}
	proc := make(map[int]ProcIdentity, len(records))
	seenIDs := make(map[int64]bool, len(records))
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return PM2Snapshot{}, err
		}
		if record.ID < 0 || record.CWD == "" || record.ExecPath == "" || record.Name == "" || seenIDs[record.ID] {
			return PM2Snapshot{}, observationOutputError("snapshot-validation", "", "pm2-record-incomplete")
		}
		seenIDs[record.ID] = true
		if record.Status == "stopped" {
			if record.PID != 0 {
				return PM2Snapshot{}, observationOutputError("snapshot-validation", "", "pm2-record-incomplete")
			}
			continue
		}
		if record.Status != "online" || record.PID <= 0 {
			return PM2Snapshot{}, observationOutputError("snapshot-validation", "", "pm2-record-incomplete")
		}
		if _, duplicate := proc[record.PID]; duplicate {
			return PM2Snapshot{}, observationOutputError("snapshot-validation", "", "pm2-identity-ambiguous")
		}
		identity, err := p.Proc.Read(ctx, record.PID)
		if err != nil {
			return PM2Snapshot{}, fmt.Errorf("proc identity is unavailable: %w", err)
		}
		if identity.CWD == "" || identity.ExecPath == "" || identity.StartTicks == 0 {
			return PM2Snapshot{}, observationOutputError("snapshot-validation", "", "proc-identity-incomplete")
		}
		if !samePath(record.CWD, identity.CWD) {
			return PM2Snapshot{}, observationOutputError("snapshot-validation", "", "pm2-proc-identity-mismatch")
		}
		proc[record.PID] = identity
	}
	if err := ctx.Err(); err != nil {
		return PM2Snapshot{}, err
	}
	return PM2Snapshot{
		Records: append([]PM2Record(nil), records...),
		Sockets: append([]SocketOwner(nil), sockets...),
		Proc:    proc,
	}, nil
}
