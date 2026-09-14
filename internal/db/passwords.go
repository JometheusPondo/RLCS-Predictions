package db

import "context"

// ResetParticipantPassword changes only the password of an existing account.
func (db *DB) ResetParticipantPassword(ctx context.Context, id, password string) error {
	result, err := db.ExecContext(ctx, `UPDATE participants SET password = ? WHERE id = ?`, password, id)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
