package postgresql

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
)

func TestDataRowDecodeSecurity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  []byte
		valid bool
	}{
		{"negative length", []byte{0, 1, 255, 255, 255, 254}, false},
		{"minimum int32", []byte{0, 1, 128, 0, 0, 0}, false},
		{"truncated count", []byte{0}, false},
		{"truncated length", []byte{0, 1, 0, 0}, false},
		{"truncated value", []byte{0, 1, 0, 0, 0, 2, 'x'}, false},
		{"oversized length", []byte{0, 1, 127, 255, 255, 255}, false},
		{"null", []byte{0, 1, 255, 255, 255, 255}, true},
		{"empty", []byte{0, 1, 0, 0, 0, 0}, true},
		{"text", []byte{0, 1, 0, 0, 0, 1, 'x'}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var row pgproto3.DataRow
			if err := row.Decode(tc.body); (err == nil) != tc.valid {
				t.Fatalf("Decode error = %v, valid = %v", err, tc.valid)
			}
			if tc.valid {
				if len(row.Values) != 1 {
					t.Fatal("expected one field")
				}
				if (row.Values[0] == nil) != (tc.name == "null") {
					t.Fatal("NULL and empty field were confused")
				}
				if tc.name == "text" && string(row.Values[0]) != "x" {
					t.Fatal("text field changed")
				}
			}
		})
	}
}

// Exercise the registered database/sql driver, not just DataRow.Decode: a
// compromised server must return a query error without taking down the client.
func TestMalformedDataRowFromServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	result := make(chan error, 1)
	go func() { result <- sendMalformedDataRow(listener) }()
	ins, err := configuredInstance(t, fmt.Sprintf("host=127.0.0.1 port=%d user=test dbname=test sslmode=disable prefer_simple_protocol=true statement_cache_mode=describe min_read_buffer_size=16384", listener.Addr().(*net.TCPAddr).Port), "")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx/v5", ins.connConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var value string
	if err := db.QueryRowContext(ctx, "SELECT 'x'").Scan(&value); err == nil {
		t.Fatal("malformed DataRow should fail the query")
	} else if !strings.Contains(err.Error(), "DataRow") {
		t.Fatalf("expected DataRow decoding error, got %v", err)
	} else {
		t.Logf("malformed response rejected: %v", err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("malformed server did not finish")
	}
}

func sendMalformedDataRow(listener net.Listener) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	backend := pgproto3.NewBackend(conn, conn)
	message, err := backend.ReceiveStartupMessage()
	if err != nil {
		return err
	}
	startup, ok := message.(*pgproto3.StartupMessage)
	if !ok {
		return fmt.Errorf("expected StartupMessage, got %T", message)
	}
	for _, key := range []string{"prefer_simple_protocol", "statement_cache_mode", "min_read_buffer_size"} {
		if _, ok := startup.Parameters[key]; ok {
			return fmt.Errorf("client option %q sent to server", key)
		}
	}
	backend.Send(&pgproto3.AuthenticationOk{})
	backend.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
	backend.Send(&pgproto3.ParameterStatus{Name: "standard_conforming_strings", Value: "on"})
	backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := backend.Flush(); err != nil {
		return err
	}
	message, err = backend.Receive()
	if err != nil {
		return err
	}
	if _, ok := message.(*pgproto3.Query); !ok {
		return fmt.Errorf("expected simple Query, got %T", message)
	}
	backend.Send(&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("value"), DataTypeOID: 25, DataTypeSize: -1, TypeModifier: -1}}})
	if err := backend.Flush(); err != nil {
		return err
	}
	// One field, length -2 (only -1 is a legal negative length).
	packet := []byte{'D', 0, 0, 0, 10, 0, 1, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(packet[7:], 0xfffffffe)
	_, err = conn.Write(packet)
	return err
}
