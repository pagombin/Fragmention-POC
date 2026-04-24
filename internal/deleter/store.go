package deleter

import (
	"database/sql"
	"errors"
)

// sqlErrNoRows aliases the driver's sentinel to keep imports tidy.
var sqlErrNoRows = sql.ErrNoRows

// storeDB returns the shared *sql.DB for the delete_candidates table.
// The Deleter carries a direct storage.Store reference on its depsStore
// field, populated by Deps.Store at New time.
func (d *Deleter) storeDB() *sql.DB {
	if d.depsStore == nil {
		panic(errors.New("deleter: store accessor not initialized"))
	}
	return d.depsStore.DB()
}
