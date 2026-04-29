package deleter

import (
	"database/sql"
	"encoding/gob"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// sqlErrNoRows aliases the driver's sentinel to keep imports tidy.
var sqlErrNoRows = sql.ErrNoRows

// gob requires concrete types in []any payloads to be registered before
// Encode. The candidate set is []any of _id values - mostly bson.ObjectID
// for the default templates, plus string for UUID-keyed collections.
func init() {
	gob.Register(bson.ObjectID{})
	gob.Register("")
	gob.Register(int32(0))
	gob.Register(int64(0))
	gob.Register(bson.D{})
	gob.Register(bson.M{})
}

// storeDB returns the shared *sql.DB for the delete_candidates table.
// The Deleter carries a direct storage.Store reference on its depsStore
// field, populated by Deps.Store at New time.
func (d *Deleter) storeDB() *sql.DB {
	if d.depsStore == nil {
		panic(errors.New("deleter: store accessor not initialized"))
	}
	return d.depsStore.DB()
}
