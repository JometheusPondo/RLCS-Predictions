package db

// WithOwnerOverride returns a separate event scope for owner-authorized pick
// corrections. The API derives this permission from the requester, never the
// participant being edited or a request-body flag.
func (db *EventStore) WithOwnerOverride() *EventStore {
	return &EventStore{DB: db.DB, tournamentID: db.tournamentID, ownerOverride: true}
}
