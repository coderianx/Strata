package repository

import (
	"context"
	"errors"

	"github.com/coderianx/Strata/internal/store"
)

// ErrTeamAlreadyExists is returned by CreateNewTeam when the
// identifier is already taken inside the organization, so handlers
// can map it to a 409 Conflict.
var ErrTeamAlreadyExists = errors.New("repository: team already exists")

// CreateNewTeam stores a team inside the given organization, but
// only when createdBy is the organization's owner; otherwise it
// returns ErrOrganizationNotFound. A duplicate identifier inside
// the organization answers ErrTeamAlreadyExists.
func CreateNewTeam(
	ctx context.Context,
	orgID int64,
	name string,
	identifier string,
	description string,
	created_by int64,
) (int64, error) {
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
		orgID,
		created_by,
	).Scan(&orgIsExists)

	if err != nil {
		return 0, err
	}

	if !orgIsExists {
		return 0, ErrOrganizationNotFound
	}

	var idTeamDB int64

	err = store.DB.QueryRow(
		ctx,
		`
		INSERT INTO teams (
			organization_id, name, identifier, description, created_by
		)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id
		`,
		orgID,
		name,
		identifier,
		description,
		created_by,
	).Scan(&idTeamDB)

	if err != nil {

		if store.IsUniqueErr(err) {
			return 0, ErrTeamAlreadyExists
		}

		return 0, err
	}

	return idTeamDB, nil
}

// ErrTeamNotFound is returned by DeleteTeam when no team with the id
// belongs to the given organization. The "not the owner" case is
// indistinguishable on purpose.
var ErrTeamNotFound = errors.New("repository: team not found")

// DeleteTeam removes the team from the given organization, but only
// when userID is the organization's owner; missing and not-owned
// organizations are indistinguishably ErrOrganizationNotFound.
func DeleteTeam(
	ctx context.Context,
	orgID int64,
	teamID int64,
	userID int64,
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
		orgID,
		userID,
	).Scan(&orgIsExists)

	if err != nil {
		return err
	}

	if !orgIsExists {
		return ErrOrganizationNotFound
	}

	result, err := store.DB.Exec(
		ctx,
		`
        DELETE FROM teams
        WHERE id = $1
        AND organization_id = $2;
        `,
		teamID,
		orgID,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrTeamNotFound
	}

	return nil
}
