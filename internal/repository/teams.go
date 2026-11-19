package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/coderianx/Strata/internal/models"
	"github.com/coderianx/Strata/internal/store"
	"github.com/jackc/pgx/v5"
)

// ErrTeamAlreadyExists is returned by CreateNewTeam when the
// identifier is already taken inside the organization, so handlers
// can map it to a 409 Conflict.
var ErrTeamAlreadyExists = errors.New("repository: team already exists")

// CreateNewTeam stores a team inside the given organization together
// with the creator's membership (role='creator'), both in one
// transaction so a team is never left without its creator. It only
// works when createdBy is the organization's owner; otherwise it
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
	tx, err := store.DB.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("repository: begin create team: %w", err)
	}

	// Rollback is a no-op after a successful commit, so every early
	// return below safely discards the partial work.
	defer tx.Rollback(ctx)

	var orgIsExists bool

	err = tx.QueryRow(
		ctx,
		`
		SELECT EXISTS(
		    SELECT 1
		    FROM organizations
		    WHERE id = $1 AND owner_id = $2
		);
		`,
		orgID,
		created_by,
	).Scan(&orgIsExists)

	if err != nil {
		return 0, fmt.Errorf("repository: check organization owner: %w", err)
	}

	if !orgIsExists {
		return 0, ErrOrganizationNotFound
	}

	var idTeamDB int64

	err = tx.QueryRow(
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

		return 0, fmt.Errorf("repository: insert team: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO team_members (
			team_id, user_id, role
		)
		VALUES ($1, $2, 'creator')
		`,
		idTeamDB,
		created_by,
	)

	if err != nil {
		return 0, fmt.Errorf("repository: insert creator membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("repository: commit create team: %w", err)
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

// GetUserByUsername returns the id of the user with the given
// username. It resolves usernames coming from request bodies into
// ids for membership writes.
func GetUserByUsername(
	ctx context.Context,
	username string,
) (int64, error) {
	var userID int64

	err := store.DB.QueryRow(
		ctx,
		`
		SELECT id
		FROM users
		WHERE username = $1;
		`,
		username,
	).Scan(&userID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrUserNotFound
		}

		return 0, fmt.Errorf("repository: get user by username: %w", err)
	}

	return userID, nil
}

// ErrUserNotOrganizationMember is returned by AddNewMemberToTeam
// when the user being added is not a member of the team's
// organization. Team membership must build on organization
// membership, so a team can never pull an outside user in.
var ErrUserNotOrganizationMember = errors.New("repository: user is not a member of the organization")

// ErrTeamAccessDenied is returned by AddNewMemberToTeam when the
// caller is not a creator or admin of the team. Callers who are not
// members at all, plain members, and callers of a missing team are
// indistinguishable on purpose: the error must not reveal which
// teams exist.
var ErrTeamAccessDenied = errors.New("repository: only team creators and admins can add members")

// AddNewMemberToTeam stores newMemberID as a member of the team with
// the given role, but only when userID is a creator or admin of that
// team; otherwise it returns ErrTeamAccessDenied. The user being
// added must already be a member of the team's organization
// (ErrUserNotOrganizationMember otherwise), and a user who is
// already a member of the team answers ErrUserAlreadyMember.
func AddNewMemberToTeam(
	ctx context.Context,
	teamID int64,
	newMemberID int64,
	role string,
	userID int64,
) error {
	// One query checks both memberships: the caller must be a
	// creator or admin of the team, and the user being added must
	// belong to the team's organization. A missing team fails both
	// checks, so it stays indistinguishable from a denied caller.
	var isPrivileged bool
	var isOrgMember bool

	err := store.DB.QueryRow(
		ctx,
		`
		SELECT
			EXISTS (
				SELECT 1
				FROM team_members
				WHERE team_id = $1 AND user_id = $2
				  AND role IN ('creator', 'admin')
			),
			EXISTS (
				SELECT 1
				FROM organization_members om
				INNER JOIN teams t ON t.organization_id = om.organization_id
				WHERE t.id = $1 AND om.user_id = $3
			);
		`,
		teamID,
		userID,
		newMemberID,
	).Scan(&isPrivileged, &isOrgMember)

	if err != nil {
		return fmt.Errorf("repository: check team privileges: %w", err)
	}

	if !isPrivileged {
		return ErrTeamAccessDenied
	}

	if !isOrgMember {
		return ErrUserNotOrganizationMember
	}

	_, err = store.DB.Exec(
		ctx,
		`
		INSERT INTO team_members (
			team_id, user_id, role
		)
		VALUES ($1, $2, $3);
		`,
		teamID,
		newMemberID,
		role,
	)

	if err != nil {
		if store.IsUniqueErr(err) {
			return ErrUserAlreadyMember
		}

		return fmt.Errorf("repository: insert team member: %w", err)
	}

	return nil
}

// ListOrganizationTeams returns the teams of the given organization,
// but only when userID is one of its members; non-members and
// missing organizations are indistinguishably
// ErrOrganizationNotFound. Teams are ordered by creation, so the
// creator's first team comes first.
func ListOrganizationTeams(
	ctx context.Context,
	organizationID int64,
	userID int64,
) ([]models.OrganizationTeam, error) {
	var isMember bool

	err := store.DB.QueryRow(
		ctx,
		`
		SELECT EXISTS (
			SELECT 1
			FROM organization_members
			WHERE organization_id = $1 AND user_id = $2
		);
		`,
		organizationID,
		userID,
	).Scan(&isMember)

	if err != nil {
		return nil, fmt.Errorf("repository: check organization membership: %w", err)
	}

	if !isMember {
		return nil, ErrOrganizationNotFound
	}

	rows, err := store.DB.Query(
		ctx,
		`
		SELECT id, name, identifier, description, created_at
		FROM teams
		WHERE organization_id = $1
		ORDER BY created_at;
		`,
		organizationID,
	)

	if err != nil {
		return nil, fmt.Errorf("repository: list organization teams: %w", err)
	}

	defer rows.Close()

	// Start with an empty slice so an empty result encodes as []
	// instead of null.
	teams := []models.OrganizationTeam{}

	for rows.Next() {
		var team models.OrganizationTeam

		err = rows.Scan(
			&team.ID,
			&team.Name,
			&team.Identifier,
			&team.Description,
			&team.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("repository: scan organization team: %w", err)
		}

		teams = append(teams, team)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: list organization teams: %w", err)
	}

	return teams, nil
}

// ErrCannotRemoveCreator is returned by RemoveMemberFromTeam when the
// member being removed is the team's creator. The creator anchors the
// team's administration and can only leave by deleting the team.
var ErrCannotRemoveCreator = errors.New("repository: team creator cannot be removed")

// RemoveMemberFromTeam removes a member from the team, but only when
// userID is a creator or admin of that team; otherwise it returns
// ErrTeamAccessDenied. The team's creator cannot be removed by
// anyone, including the creator themselves (ErrCannotRemoveCreator):
// deleting the team is the creator's way out. A member that is not
// part of the team answers ErrMemberNotFound.
func RemoveMemberFromTeam(
	ctx context.Context,
	teamID int64,
	memberID int64,
	userID int64,
) error {
	// One query resolves both the caller's privileges and the target
	// member's role. A missing team fails the privilege check, so it
	// stays indistinguishable from a denied caller.
	var isPrivileged bool
	var memberRole string

	err := store.DB.QueryRow(
		ctx,
		`
		SELECT
			EXISTS (
				SELECT 1
				FROM team_members
				WHERE team_id = $1 AND user_id = $2
				  AND role IN ('creator', 'admin')
			),
			COALESCE((
				SELECT role
				FROM team_members
				WHERE team_id = $1 AND user_id = $3
			), '');
		`,
		teamID,
		userID,
		memberID,
	).Scan(&isPrivileged, &memberRole)

	if err != nil {
		return fmt.Errorf("repository: check team privileges: %w", err)
	}

	if !isPrivileged {
		return ErrTeamAccessDenied
	}

	if memberRole == "creator" {
		return ErrCannotRemoveCreator
	}

	if memberRole == "" {
		return ErrMemberNotFound
	}

	tag, err := store.DB.Exec(
		ctx,
		`
		DELETE FROM team_members
		WHERE team_id = $1 AND user_id = $2;
		`,
		teamID,
		memberID,
	)

	if err != nil {
		return fmt.Errorf("repository: remove team member: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}

	return nil
}
