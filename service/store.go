package service

import (
	"database/sql"
	"errors"
	"sync"

	"github.com/gdotgordon/ipverify/types"

	// Load sqlite3 driver
	_ "github.com/mattn/go-sqlite3"

	"go.uber.org/zap"
)

const sqlAdditem = `
	INSERT INTO items(
		Uuid,
		Username,
		Ipaddr,
        Unix
    ) values(?, ?, ?, ?)`

// Store is the datastore abstraction for storing IP verify requests and retrieving
// them for checks for suspicious activity.
type Store interface {
	AddRecord(types.VerifyRequest) error
	GetAllRowsForUser(username string) ([]types.VerifyRequest, error)
	GetPriorNext(username string, timestamp int64) (*types.VerifyRequest, *types.VerifyRequest, error)
	Shutdown()
}

// SQLiteStore is an implementation of the Store interface that uses the
// mattn/go-sqlite3 DB driver.  Note the documentation states that it the
// driver is safe for concurrent reads, but there are issues with concurrent
// writes, hence a mutex is required.
type SQLiteStore struct {
	db      *sql.DB
	addStmt *sql.Stmt
	mu      sync.RWMutex
	log     *zap.SugaredLogger
}

// NewSQLiteStore creates a new store for SQLite3 at the specifed file location.
func NewSQLiteStore(filepath string, log *zap.SugaredLogger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("Unable to open database")
	}

	if err := createTable(db, log); err != nil {
		return nil, err
	}
	addStmt, err := db.Prepare(sqlAdditem)
	if err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db, addStmt: addStmt, log: log}, nil
}

// AddRecord adds a single new request item to the database.
func (sqs *SQLiteStore) AddRecord(item types.VerifyRequest) error {
	sqs.mu.Lock()
	defer sqs.mu.Unlock()

	sqs.log.Debugw("adding db row", "item", item)
	_, err := sqs.addStmt.Exec(item.EventUUID, item.Username, item.IPAddress, item.UnixTimestamp)
	if err != nil {
		sqs.log.Errorw("adding db row failed", "error", err)
		return err
	}
	return nil
}

// GetAllRowsForUser gets all rows that match the specified username.
func (sqs *SQLiteStore) GetAllRowsForUser(username string) ([]types.VerifyRequest, error) {
	sqlReadall := `
		SELECT Uuid, Username, Ipaddr, Unix FROM items
		WHERE Username = ?
        ORDER BY Unix DESC
        `
	sqs.mu.RLock()
	defer sqs.mu.RUnlock()

	rows, err := sqs.db.Query(sqlReadall, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.VerifyRequest
	for rows.Next() {
		item := types.VerifyRequest{}
		err2 := rows.Scan(&item.EventUUID, &item.Username, &item.IPAddress, &item.UnixTimestamp)
		if err2 != nil {
			panic(err2)
		}
		result = append(result, item)
	}
	return result, nil
}

// GetPriorNext is the key method for the security check logic.  It queries
// to get both the item just prior to the current event and the one just
// subsequent to it.  As documented, we consider the presumably rare case
// of two logins for the same user at exactly the same Unix time as a
// suspicious login, and capture it along with the prior events.
func (sqs *SQLiteStore) GetPriorNext(username string,
	timestamp int64) (*types.VerifyRequest, *types.VerifyRequest, error) {
	var prev, next *types.VerifyRequest

	sqs.mu.RLock()
	defer sqs.mu.RUnlock()

	// Use two queries, one for prior timestamps (but return only the latest
	// of those) and one for subsequent logins, again, only capturing the
	// earliest of those.
	for _, v := range []string{`
        SELECT Uuid, Username, Ipaddr, Unix FROM items
        WHERE Username = ? AND Unix <= ?
		ORDER BY Unix DESC LIMIT 1`,
		`SELECT Uuid, Username, Ipaddr, Unix FROM items
        WHERE Username = ? AND Unix > ?
		ORDER BY Unix ASC LIMIT 1`,
	} {
		rows, err := sqs.db.Query(v, username, timestamp)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()

		for rows.Next() {
			item := types.VerifyRequest{}
			err2 := rows.Scan(&item.EventUUID, &item.Username, &item.IPAddress, &item.UnixTimestamp)
			if err2 != nil {
				panic(err2)
			}
			if item.UnixTimestamp <= timestamp {
				prev = &item
			} else {
				next = &item
			}
		}
	}
	return prev, next, nil
}

// Shutdown does cleanup on termination
func (sqs *SQLiteStore) Shutdown() {
	if err := sqs.addStmt.Close(); err != nil {
		sqs.log.Warnw("sqlite prepared statement close", "error", err)
	}
	if err := sqs.db.Close(); err != nil {
		sqs.log.Warnw("sqlite shutdown error", "error", err)
	}
}

func createTable(db *sql.DB, log *zap.SugaredLogger) error {
	// create table if not exists
	sqlTable := `
	CREATE TABLE IF NOT EXISTS items(
			Uuid TEXT NOT NULL PRIMARY KEY,
			Username TEXT NOT NULL,
			Ipaddr TEXT NOT NULL,
			Unix INT NOT NULL
	);
	`
	_, err := db.Exec(sqlTable)
	if err != nil {
		return err
	}
	log.Infow("Created table", "name", "items")
	return nil
}
