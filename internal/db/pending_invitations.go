package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"trakka/internal/models"
)

// Invitation kinds, matching the CHECK constraint on
// pending_invitations.kind.
const (
	InvitationKindHouse = "house"
	InvitationKindList  = "list"
	InvitationKindSpace = "space"
)

// CreatePendingInvitation records an invitation addressed to an email rather
// than to a user id, so the caller learns nothing about whether that address
// has an account here (see the migration's own comment, and docs/AUDIT.md finding
// L-06). Re-inviting the same address for the same target updates the
// permission instead of failing, mirroring CreateOrUpdate{List,Space}Share.
func (d *DB) CreatePendingInvitation(ctx context.Context, kind string, targetID int64, email, permission string, invitedBy int64) (*models.PendingInvitation, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO pending_invitations (kind, target_id, email, permission, invited_by)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (kind, target_id, email)
		DO UPDATE SET permission = excluded.permission, invited_by = excluded.invited_by
	`, kind, targetID, email, permission, invitedBy)
	if err != nil {
		return nil, fmt.Errorf("recording pending invitation: %w", err)
	}
	return d.GetPendingInvitation(ctx, kind, targetID, email)
}

// GetPendingInvitation fetches one invitation. Returns ErrNotFound if there
// is none for that target and address.
func (d *DB) GetPendingInvitation(ctx context.Context, kind string, targetID int64, email string) (*models.PendingInvitation, error) {
	inv := &models.PendingInvitation{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, kind, target_id, email, permission, invited_by, created_at
		 FROM pending_invitations WHERE kind = ? AND target_id = ? AND email = ?`,
		kind, targetID, strings.ToLower(strings.TrimSpace(email)),
	).Scan(&inv.ID, &inv.Kind, &inv.TargetID, &inv.Email, &inv.Permission, &inv.InvitedBy, &inv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying pending invitation: %w", err)
	}
	return inv, nil
}

// ListPendingInvitations returns the outstanding invitations for one target,
// oldest first — the "invited, not yet joined" half of a roster.
func (d *DB) ListPendingInvitations(ctx context.Context, kind string, targetID int64) ([]*models.PendingInvitation, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, kind, target_id, email, permission, invited_by, created_at
		 FROM pending_invitations WHERE kind = ? AND target_id = ? ORDER BY created_at ASC`,
		kind, targetID)
	if err != nil {
		return nil, fmt.Errorf("querying pending invitations: %w", err)
	}
	defer rows.Close()

	invitations := []*models.PendingInvitation{}
	for rows.Next() {
		inv := &models.PendingInvitation{}
		if err := rows.Scan(&inv.ID, &inv.Kind, &inv.TargetID, &inv.Email, &inv.Permission, &inv.InvitedBy, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning pending invitation row: %w", err)
		}
		invitations = append(invitations, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pending invitation rows: %w", err)
	}
	return invitations, nil
}

// DeletePendingInvitation withdraws an invitation (a mistyped address, or a
// change of mind). Returns ErrNotFound if there was none.
func (d *DB) DeletePendingInvitation(ctx context.Context, kind string, targetID int64, email string) error {
	res, err := d.conn.ExecContext(ctx,
		`DELETE FROM pending_invitations WHERE kind = ? AND target_id = ? AND email = ?`,
		kind, targetID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return fmt.Errorf("deleting pending invitation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for pending invitation delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MaterializePendingInvitations turns every invitation addressed to this
// user's email into the real membership or share it stands for, and reports
// how many were applied.
//
// This is the half of the design that makes an invitation to an address with
// no account honest rather than a silent no-op: nothing is granted at invite
// time, and the grant only happens once the invited person authenticates
// here themselves. It is therefore impossible to attach data to an address
// whose owner never signs in.
//
// Called on authentication (handlers.handleMe, and right after registration
// or OIDC provisioning). Every step is idempotent, so a concurrent duplicate
// call can at worst do the same work twice; and each invitation is deleted
// whether or not its target still exists, so a house or list deleted between
// invitation and sign-in leaves nothing behind.
func (d *DB) MaterializePendingInvitations(ctx context.Context, userID int64, email string) (int, error) {
	invitations, err := d.pendingInvitationsForEmail(ctx, email)
	if err != nil {
		return 0, err
	}

	applied := 0
	for _, inv := range invitations {
		ok, err := d.applyInvitation(ctx, inv, userID)
		if err != nil {
			return applied, err
		}
		if err := d.deletePendingInvitationByID(ctx, inv.ID); err != nil {
			return applied, err
		}
		if ok {
			applied++
		}
	}
	return applied, nil
}

func (d *DB) pendingInvitationsForEmail(ctx context.Context, email string) ([]*models.PendingInvitation, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, kind, target_id, email, permission, invited_by, created_at
		 FROM pending_invitations WHERE email = ? ORDER BY created_at ASC`,
		strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, fmt.Errorf("querying pending invitations for email: %w", err)
	}
	defer rows.Close()

	invitations := []*models.PendingInvitation{}
	for rows.Next() {
		inv := &models.PendingInvitation{}
		if err := rows.Scan(&inv.ID, &inv.Kind, &inv.TargetID, &inv.Email, &inv.Permission, &inv.InvitedBy, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning pending invitation row: %w", err)
		}
		invitations = append(invitations, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pending invitation rows: %w", err)
	}
	return invitations, nil
}

// applyInvitation grants one invitation to userID. It reports ok=false
// (without an error) when the invitation can no longer be applied — the
// house/list/space it names has been deleted, or the user is already a
// member — since neither is a failure worth propagating: the caller deletes
// the row either way.
func (d *DB) applyInvitation(ctx context.Context, inv *models.PendingInvitation, userID int64) (bool, error) {
	switch inv.Kind {
	case InvitationKindHouse:
		if _, err := d.GetHouse(ctx, inv.TargetID); errors.Is(err, ErrNotFound) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		if _, err := d.AddHouseMember(ctx, inv.TargetID, userID, "member"); errors.Is(err, ErrAlreadyMember) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		return true, nil

	case InvitationKindList:
		if _, err := d.GetList(ctx, inv.TargetID); errors.Is(err, ErrNotFound) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		if _, err := d.CreateOrUpdateListShare(ctx, inv.TargetID, userID, inv.Permission); err != nil {
			return false, err
		}
		return true, nil

	case InvitationKindSpace:
		var exists int
		err := d.conn.QueryRowContext(ctx, `SELECT 1 FROM custom_categories WHERE id = ?`, inv.TargetID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("checking invited category %d: %w", inv.TargetID, err)
		}
		if _, err := d.CreateOrUpdateSpaceShare(ctx, inv.TargetID, userID, inv.Permission); err != nil {
			return false, err
		}
		return true, nil
	}
	// An unknown kind cannot happen (the column has a CHECK constraint), but
	// dropping the row is the right response if it somehow did.
	return false, nil
}

func (d *DB) deletePendingInvitationByID(ctx context.Context, id int64) error {
	if _, err := d.conn.ExecContext(ctx, `DELETE FROM pending_invitations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting applied invitation %d: %w", id, err)
	}
	return nil
}
