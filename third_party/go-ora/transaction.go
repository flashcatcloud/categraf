package go_ora

import (
	"context"
	"database/sql/driver"
)

type Transaction struct {
	conn *Connection
	ctx  context.Context
}

func (tx *Transaction) Commit() error {
	if tx.conn.State != Opened {
		return driver.ErrBadConn
	}
	tx.conn.autoCommit = true
	tx.conn.session.ResetBuffer()
	tx.conn.session.StartContext(tx.ctx)
	defer tx.conn.session.EndContext()
	return (&simpleObject{connection: tx.conn, operationID: 0xE}).exec()
}

func (tx *Transaction) Rollback() error {
	if tx.conn.State != Opened {
		return driver.ErrBadConn
	}
	tx.conn.autoCommit = true
	tx.conn.session.ResetBuffer()
	tx.conn.session.StartContext(tx.ctx)
	defer tx.conn.session.EndContext()
	return (&simpleObject{connection: tx.conn, operationID: 0xF}).exec()
}
