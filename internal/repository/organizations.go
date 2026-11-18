package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/coderianx/Strata/internal/models"
	"github.com/coderianx/Strata/internal/store"
	"github.com/jackc/pgx/v5"
)

// ErrOrganizationAlreadyExists is returned by CreateNewOrganization when
// the slug is already taken, so handlers can map it to a 409 Conflict.
var ErrOrganizationAlreadyExists = errors.New("repository: organization already exists")

// CreateNewOrganization stores a new organization and adds its creator as
// the first member with the "owner" role. Both rows are written in one
// transaction: an organization left without an owner membership row would
// be invisible to every membership-scoped query.
func CreateNewOrganization(
	ctx context.Context,
	name string,
	slug string,
	description string,
	logoURL string,
	ownerID int64,
) error {

	tx, err := store.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("repository: begin create organization: %w", err)
	}

	// Rollback is a no-op after a successful commit, so every early
	// return below safely discards the partial work.
	defer tx.Rollback(ctx)

	var orgID int64

	err = tx.QueryRow(
		ctx,
		`
		INSERT INTO organizations (
			name, slug, description, logo_url, owner_id
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
		`,
		name,
		slug,
		description,
		logoURL,
		ownerID,
	).Scan(&orgID)

	if err != nil {
		if store.IsUniqueErr(err) {
			return ErrOrganizationAlreadyExists
		}

		return fmt.Errorf("repository: insert organization: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO organization_members (
			organization_id, user_id, role
		)
		VALUES ($1, $2, $3)
		`,
		orgID,
		ownerID,
		"owner",
	)

	if err != nil {
		return fmt.Errorf("repository: insert owner membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("repository: commit create organization: %w", err)
	}

	return nil
}

// ErrOrganizationNotFound is returned by DeleteOrganization when no
// organization matches the id, or when the caller is not its owner. The
// two cases are indistinguishable on purpose: revealing "exists but you
// are not the owner" would leak which organizations other users own.
var ErrOrganizationNotFound = errors.New("repository: organization not found")

// DeleteOrganization removes the organization with the given id, but only
// when ownerID is the organization's owner. The organization_members rows
// are removed together with the organization by the schema's ON DELETE
// CASCADE, so a single statement is atomic and no transaction is needed.
func DeleteOrganization(
	ctx context.Context,
	organizationID int64,
	ownerID int64,
) error {

	tag, err := store.DB.Exec(
		ctx,
		`
		DELETE FROM organizations
		WHERE id = $1 AND owner_id = $2
		`,
		organizationID,
		ownerID,
	)

	if err != nil {
		return fmt.Errorf("repository: delete organization: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrOrganizationNotFound
	}

	return nil
}

// ErrUserAlreadyMember is returned by InviteOrganization when the
// receiver is already a member of the organization, so handlers can
// map it to a 409 Conflict.
var ErrUserAlreadyMember = errors.New("repository: user already member this organization")

// ErrInvitationAlreadyExists is returned by InviteOrganization when
// the receiver already has a pending invitation, so handlers can map
// it to a 409 Conflict.
var ErrInvitationAlreadyExists = errors.New("repository: invitation already exists")

func InviteOrganization(
	ctx context.Context,
	organizationID int64,
	senderID int64,
	receiverUsername string,
	role string,
) error {
	var orgIsExists bool

	err := store.DB.QueryRow(
		ctx,
		`
		SELECT EXISTS (
		    SELECT 1
		    FROM organizations
		    WHERE id = $1 AND owner_id = $2
		);
		`,
		organizationID,
		senderID,
	).Scan(&orgIsExists)

	if err != nil {
		return err
	}

	if !orgIsExists {
		return ErrOrganizationNotFound
	}

	var receiverID int64

	err = store.DB.QueryRow(
		ctx,
		`
		SELECT id
		FROM users
		WHERE username = $1
		`,
		receiverUsername,
	).Scan(
		&receiverID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}

		return err
	}

	var ReceiverAlreadyMember bool

	err = store.DB.QueryRow(
		ctx,
		`
		SELECT EXISTS (
			SELECT 1
			FROM organization_members
			WHERE organization_id = $1 AND user_id = $2
		);
		`,
		organizationID, receiverID,
	).Scan(
		&ReceiverAlreadyMember,
	)

	if err != nil {
		return err
	}

	if ReceiverAlreadyMember {
		return ErrUserAlreadyMember
	}

	_, err = store.DB.Exec(
		ctx,
		`
		INSERT INTO organization_invitations (
			organization_id, user_id, invited_by, role
		)
		VALUES ($1,$2,$3,$4);
		`,
		organizationID,
		receiverID,
		senderID,
		role,
	)

	if err != nil {
		if store.IsUniqueErr(err) {
			return ErrInvitationAlreadyExists
		}

		return fmt.Errorf("repository: insert invitation: %w", err)
	}

	return nil
}

// ListReceivedInvitations returns the receiver's pending invitations,
// oldest first. Only pending rows are actionable, so accepted and
// rejected history is intentionally left out.
func ListReceivedInvitations(
	ctx context.Context,
	receiverID int64,
) ([]models.ListReceivedInvitations, error) {
	rows, err := store.DB.Query(
		ctx,
		`
		SELECT id, organization_id, user_id, invited_by, role, status, created_at
		FROM organization_invitations
		WHERE user_id = $1 AND status = 'pending'
		ORDER BY created_at;
		`,
		receiverID,
	)

	if err != nil {
		return nil, fmt.Errorf("repository: list received invitations: %w", err)
	}

	defer rows.Close()

	// Start with an empty slice so an empty result encodes as []
	// instead of null.
	invitations := []models.ListReceivedInvitations{}

	for rows.Next() {
		var invitation models.ListReceivedInvitations

		err = rows.Scan(
			&invitation.ID,
			&invitation.OrganizationID,
			&invitation.UserID,
			&invitation.InvitedBy,
			&invitation.Role,
			&invitation.Status,
			&invitation.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("repository: scan received invitation: %w", err)
		}

		invitations = append(invitations, invitation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: list received invitations: %w", err)
	}

	return invitations, nil
}

// ErrInvitationNotFound is returned by AcceptInvitation and
// RejectInvitation when no pending invitation matches the id and the
// receiver. The two cases are indistinguishable on purpose: revealing
// "exists but belongs to someone else" would leak other users'
// invitations.
var ErrInvitationNotFound = errors.New("repository: invitation not found")

// AcceptInvitation turns the receiver's pending invitation into an
// organization membership with the invited role. The invitation is
// claimed atomically by its status update, so two concurrent accepts
// cannot both win, and a failed membership insert rolls the claim back
// with the transaction.
func AcceptInvitation(
	ctx context.Context,
	receiverID int64,
	invitationID int64,
) error {
	tx, err := store.DB.Begin(ctx)

	if err != nil {
		return fmt.Errorf("repository: begin accept invitation: %w", err)
	}

	// Rollback is a no-op after a successful commit, so every early
	// return below safely discards the partial work.
	defer tx.Rollback(ctx)

	var organizationID int64
	var role string

	err = tx.QueryRow(
		ctx,
		`
		UPDATE organization_invitations
		SET status = 'accepted'
		WHERE id = $1 AND user_id = $2 AND status = 'pending'
		RETURNING organization_id, role;
		`,
		invitationID,
		receiverID,
	).Scan(&organizationID, &role)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvitationNotFound
		}

		return fmt.Errorf("repository: claim invitation: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, $3);
		`,
		organizationID,
		receiverID,
		role,
	)

	if err != nil {
		if store.IsUniqueErr(err) {
			return ErrUserAlreadyMember
		}

		return fmt.Errorf("repository: insert membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("repository: commit accept invitation: %w", err)
	}

	return nil
}

// RejectInvitation marks the receiver's pending invitation as
// rejected instead of deleting it: the pending-only unique index then
// lets the organization invite the same user again later.
func RejectInvitation(
	ctx context.Context,
	receiverID int64,
	invitationID int64,
) error {
	tag, err := store.DB.Exec(
		ctx,
		`
		UPDATE organization_invitations
		SET status = 'rejected'
		WHERE id = $1 AND user_id = $2 AND status = 'pending';
		`,
		invitationID,
		receiverID,
	)

	if err != nil {
		return fmt.Errorf("repository: reject invitation: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrInvitationNotFound
	}

	return nil
}
