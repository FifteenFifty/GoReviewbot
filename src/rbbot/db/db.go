package db

import (
    "database/sql"
    "fmt"
    "log"
    "strconv"
    "sync"

    _ "github.com/mattn/go-sqlite3"
)

var (
    dbPath string
    mutex  sync.Mutex
)

/**
 * Configures the database.
 */
func Configure(dbPathStr string) {
    dbPath = dbPathStr
    fmt.Println("DB: Configured db path as: " + dbPath)
}

/**
 * Retrieves a value from the key/value store.
 *
 * @param key The key
 *
 * @returns Whether the value was found and, if so, the value.
 */
func KvGet(key string) (string, bool) {

    // Probably overkill, but we mutex database access
    mutex.Lock()
    defer mutex.Unlock()

    var success bool = true

    db, err := sql.Open("sqlite3", dbPath)

    if (err != nil) {
        log.Fatal(err)
    }
    defer db.Close()

    rows, err := db.Query("SELECT VALUE FROM KVSTORE WHERE KEY=?;", key)

    if (err != nil) {
        log.Fatal(err)
    }
    defer rows.Close()

    var value string = ""
    if (rows.Next()) {
        err = rows.Scan(&value)

        if (err != nil) {
            log.Fatal(err)
        }
    } else {
        success = false
    }

    return value, success
}

/**
 * Writes a value to the key/value store.
 *
 * @param key   The key
 * @param value The value
 *
 * @retval bool Whether the value was successfully added.
 */
func KvPut(key string, value string) bool {

    // Probably overkill, but we mutex database access
    mutex.Lock()
    defer mutex.Unlock()

    var success bool = true

    db, err := sql.Open("sqlite3", dbPath)

    if (err != nil) {
        log.Fatal(err)
    }
    defer db.Close()

    ps, err := db.Prepare("INSERT OR REPLACE INTO KVSTORE (KEY, VALUE) " +
                          "VALUES (?,?);")

    if (err != nil) {
        log.Fatal(err)
    }
    defer ps.Close()

    tx, err := db.Begin()

    if (err != nil) {
        log.Fatal(err)
    }

    stmt := tx.Stmt(ps)
    defer stmt.Close()

    _, err = stmt.Exec(key, value)

    if (err != nil) {
        success = false
        fmt.Println("Failed to write database value. Error: %s\n", err)
        tx.Rollback()
    } else {
        tx.Commit()
    }

    return success
}

/**
 * Increments an integral value in the KV store by a specified amount.
 *
 * @param key  The key
 * @param incr The amount by which to increment the value.
 *
 * @retval bool Whether the operation was successful.
 */
func KvIncr(key string, incr int) {
    // Get the value, if it exists. If it doesn't, assume zero
    currentVal, found := KvGet(key)

    var intVal int = 0

    if (found) {
        shadowedIntVal, err := strconv.Atoi(currentVal)
        if (err != nil) {
            log.Fatal(err)
        }
        intVal = shadowedIntVal
    }

    intVal += incr

    KvPut(key, strconv.Itoa(intVal))
}
