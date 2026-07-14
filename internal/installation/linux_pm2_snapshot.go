package installation

import (
	"context"
	"errors"
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
		return PM2Snapshot{}, errors.New("pm2 inventory is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return PM2Snapshot{}, err
	}
	sockets, err := p.Sockets.Snapshot(ctx)
	if err != nil {
		return PM2Snapshot{}, errors.New("socket ownership is unavailable")
	}
	proc := make(map[int]ProcIdentity, len(records))
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return PM2Snapshot{}, err
		}
		if record.ID <= 0 || record.PID <= 0 || record.CWD == "" || record.ExecPath == "" || record.Status == "" {
			return PM2Snapshot{}, errors.New("pm2 record is incomplete")
		}
		if _, duplicate := proc[record.PID]; duplicate {
			return PM2Snapshot{}, errors.New("pm2 process identity is ambiguous")
		}
		identity, err := p.Proc.Read(ctx, record.PID)
		if err != nil || identity.CWD == "" || identity.ExecPath == "" || identity.StartTicks == 0 {
			return PM2Snapshot{}, errors.New("proc identity is unavailable")
		}
		if !samePath(record.CWD, identity.CWD) || !samePath(record.ExecPath, identity.ExecPath) {
			return PM2Snapshot{}, errors.New("pm2 and proc identity disagree")
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
