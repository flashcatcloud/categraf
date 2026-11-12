package go_ora

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

type StmtType int

const (
	SELECT StmtType = 1
	DML    StmtType = 2
	PLSQL  StmtType = 3
	OTHERS StmtType = 4
)

type StmtInterface interface {
	hasMoreRows() bool
	noOfRowsToFetch() int
	fetch(dataSet *DataSet) error
	hasBLOB() bool
	hasLONG() bool
	read(dataSet *DataSet) error
	Close() error
	CanAutoClose() bool
}
type defaultStmt struct {
	connection *Connection
	text       string
	//disableCompression bool
	_hasLONG          bool
	_hasBLOB          bool
	_hasMoreRows      bool
	_hasReturnClause  bool
	_noOfRowsToFetch  int
	stmtType          StmtType
	cursorID          int
	queryID           uint64
	Pars              []ParameterInfo
	columns           []ParameterInfo
	scnForSnapshot    []int
	arrayBindCount    int
	containOutputPars bool
	autoClose         bool
	temporaryLobs     [][]byte
}

func (stmt *defaultStmt) CanAutoClose() bool {
	return stmt.autoClose
}
func (stmt *defaultStmt) hasMoreRows() bool {
	return stmt._hasMoreRows
}

func (stmt *defaultStmt) noOfRowsToFetch() int {
	return stmt._noOfRowsToFetch
}

func (stmt *defaultStmt) hasLONG() bool {
	return stmt._hasLONG
}

func (stmt *defaultStmt) hasBLOB() bool {
	return stmt._hasBLOB
}

// basicWrite this is the default write procedure for the all type of stmt
// through it the stmt data will send to network stream
func (stmt *defaultStmt) basicWrite(exeOp int, parse, define bool) error {
	session := stmt.connection.session
	strConv, _ := stmt.connection.getStrConv(stmt.connection.tcpNego.ServerCharset)
	session.PutBytes(3, 0x5E, 0)
	session.PutUint(exeOp, 4, true, true)
	session.PutUint(stmt.cursorID, 2, true, true)
	if stmt.cursorID == 0 {
		session.PutBytes(1)

	} else {
		session.PutBytes(0)
	}
	if parse {
		session.PutUint(len(strConv.Encode(stmt.text)), 4, true, true)
		//session.PutUint(len(stmt.connection.strConv.Encode(stmt.text)), 4, true, true)
		session.PutBytes(1)
	} else {
		session.PutBytes(0, 1)
	}
	session.PutUint(13, 2, true, true)
	session.PutBytes(0, 0)
	if exeOp&0x40 == 0 && exeOp&0x20 != 0 && exeOp&0x1 != 0 && stmt.stmtType == SELECT {
		session.PutBytes(0)
		session.PutUint(stmt._noOfRowsToFetch, 4, true, true)
	} else {
		session.PutBytes(0, 0)
		//session.PutUint(0, 4, true, true)
		//session.PutUint(0, 4, true, true)
	}
	//switch (longFetchSize)
	//{
	//case -1:
	//	this.m_marshallingEngine.MarshalUB4((long) int.MaxValue);
	//	break;
	//case 0:
	//	this.m_marshallingEngine.MarshalUB4(1L);
	//	break;
	//default:
	//	this.m_marshallingEngine.MarshalUB4((long) longFetchSize);
	//	break;
	//}
	// we use here int.MaxValue
	if stmt.connection.connOption.Lob == 0 {
		session.PutInt(0x3FFFFFFF, 4, true, true)
		//session.PutUint(0, 4, true, true)
	} else {
		session.PutUint(0x7FFFFFFF, 4, true, true)
	}

	if len(stmt.Pars) > 0 && !define {
		session.PutBytes(1)
		session.PutUint(len(stmt.Pars), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	session.PutBytes(0, 0, 0, 0, 0)
	if define {
		session.PutBytes(1)
		session.PutUint(len(stmt.columns), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	if session.TTCVersion >= 4 {
		session.PutBytes(0, 0, 1)
	}
	if session.TTCVersion >= 5 {
		session.PutBytes(0, 0, 0, 0, 0)
	}
	if session.TTCVersion >= 7 {
		if stmt.stmtType == DML && stmt.arrayBindCount > 0 {
			session.PutBytes(1)
			session.PutInt(stmt.arrayBindCount, 4, true, true)
			session.PutBytes(1)
		} else {
			session.PutBytes(0, 0, 0)
		}
	}
	if session.TTCVersion >= 8 {
		session.PutBytes(0, 0, 0, 0, 0)
	}
	if session.TTCVersion >= 9 {
		session.PutBytes(0, 0)
	}
	if parse {
		session.PutClr(strConv.Encode(stmt.text))
	}
	al8i4 := make([]int, 13)
	if exeOp&1 <= 0 {
		al8i4[0] = 0
	} else {
		al8i4[0] = 1
	}
	switch stmt.stmtType {
	case DML:
		fallthrough
	case PLSQL:
		if stmt.arrayBindCount > 0 {
			al8i4[1] = stmt.arrayBindCount
			if stmt.stmtType == DML {
				al8i4[9] = 0x4000
			}
		} else {
			al8i4[1] = 1
		}
	case OTHERS:
		al8i4[1] = 1
	default:
		//this.m_al8i4[1] = !fetch ? 0L : noOfRowsToFetch;
		//al8i4[1] = stmt._noOfRowsToFetch
		if stmt.connection.connOption.Lob == 0 {
			if parse {
				al8i4[1] = 0
			} else {
				al8i4[1] = stmt._noOfRowsToFetch
			}
		} else {
			al8i4[1] = stmt._noOfRowsToFetch
		}

	}
	if len(stmt.scnForSnapshot) == 2 {
		al8i4[5] = stmt.scnForSnapshot[0]
		al8i4[6] = stmt.scnForSnapshot[1]
	} else {
		al8i4[5] = 0
		al8i4[6] = 0
	}
	if stmt.stmtType == SELECT {
		al8i4[7] = 1
	} else {
		al8i4[7] = 0
	}
	if exeOp&32 != 0 {
		al8i4[9] |= 0x8000
	} else {
		al8i4[9] &= -0x8000
	}
	for x := 0; x < len(al8i4); x++ {
		session.PutUint(al8i4[x], 4, true, true)
	}
	if define {
		err := stmt.writeDefine()
		if err != nil {
			return err
		}
	} else {
		for _, par := range stmt.Pars {
			_ = par.write(session)
		}
	}
	return nil
}

func (stmt *defaultStmt) writeDefine() error {
	session := stmt.connection.session
	num := 0x7FFFFFFF
	for _, col := range stmt.columns {
		temp := new(ParameterInfo)
		*temp = col
		temp.oaccollid = 0
		temp.Precision = 0
		temp.Scale = 0
		temp.MaxCharLen = 0
		if temp.DataType == OCIBlobLocator || temp.DataType == OCIClobLocator {
			num = 0
			temp.ContFlag |= 0x2000000
			if stmt.connection.connOption.Lob != 0 {
				num = 0x7FFFFFFF
				temp.MaxCharLen = 0
				if temp.DataType == OCIBlobLocator {
					temp.DataType = RAW
				} else {
					temp.DataType = NCHAR
				}
			} else {
				temp.MaxCharLen = 0x8000
			}
		} else {
			temp.ContFlag = 0
		}
		temp.Flag = 3
		temp.MaxLen = num
		err := temp.write(session)
		if err != nil {
			return err
		}
	}
	return nil
}

type Stmt struct {
	defaultStmt
	//reExec           bool
	reSendParDef bool
	parse        bool // means parse the command in the server this occurs if the stmt is not cached
	execute      bool
	define       bool
	bulkExec     bool
	//noOfDefCols        int
}

type QueryResult struct {
	lastInsertedID int64
	rowsAffected   int64
}

func (rs *QueryResult) LastInsertId() (int64, error) {
	return rs.lastInsertedID, nil
}

func (rs *QueryResult) RowsAffected() (int64, error) {
	return rs.rowsAffected, nil
}

// NewStmt create new stmt and set its connection properties
func NewStmt(text string, conn *Connection) *Stmt {
	ret := &Stmt{
		reSendParDef: false,
		parse:        true,
		execute:      true,
		define:       false,
	}
	ret.connection = conn
	ret.text = text
	ret._hasBLOB = false
	ret._hasLONG = false
	//ret.disableCompression = false
	ret.arrayBindCount = 0
	ret.scnForSnapshot = make([]int, 2)
	// get stmt type
	uCmdText := strings.ToUpper(refineSqlText(text))
	//for {
	//	uCmdText = strings.TrimSpace(uCmdText) // trim leading white-space
	//	if strings.HasPrefix(uCmdText, "--") {
	//		i := strings.Index(uCmdText, "\n")
	//		if i <= 0 {
	//			break
	//		}
	//		uCmdText = uCmdText[i+1:]
	//	} else if strings.HasPrefix(uCmdText, "/*") {
	//		i := strings.Index(uCmdText, "*/")
	//		if i <= 0 {
	//			break
	//		}
	//		uCmdText = uCmdText[i+2:]
	//	} else {
	//		break
	//	}
	//}
	if strings.HasPrefix(uCmdText, "(") {
		uCmdText = uCmdText[1:]
	}
	if strings.HasPrefix(uCmdText, "SELECT") || strings.HasPrefix(uCmdText, "WITH") {
		ret.stmtType = SELECT
	} else if strings.HasPrefix(uCmdText, "INSERT") ||
		strings.HasPrefix(uCmdText, "MERGE") {
		ret.stmtType = DML
		ret.bulkExec = true
	} else if strings.HasPrefix(uCmdText, "UPDATE") ||
		strings.HasPrefix(uCmdText, "DELETE") {
		ret.stmtType = DML
	} else if strings.HasPrefix(uCmdText, "DECLARE") || strings.HasPrefix(uCmdText, "BEGIN") {
		ret.stmtType = PLSQL
	} else {
		ret.stmtType = OTHERS
	}
	// returning clause
	var err error
	if ret.stmtType != PLSQL {
		//ret._hasReturnClause, err = regexp.MatchString(`\bRETURNING\b\s+(\w+\s*,\s*)*\s*\w+\s+\bINTO\b`, uCmdText)
		ret._hasReturnClause, err = regexp.MatchString(`(\bRETURNING\b|\bRETURN\b)\s+.*\s+\bINTO\b`, uCmdText)
		if err != nil {
			ret._hasReturnClause = false
		}
	}
	return ret
}

func (stmt *Stmt) writePars() error {
	session := stmt.connection.session
	buffer := bytes.Buffer{}
	for _, par := range stmt.Pars {
		if par.Flag == 0x80 {
			continue
		}
		if !stmt.parse && par.Direction == Output && stmt.stmtType != PLSQL {
			continue
		}
		if par.DataType == REFCURSOR {
			session.WriteBytes(&buffer, 1, 0)
		} else if par.Direction == Input &&
			(par.DataType == OCIClobLocator || par.DataType == OCIBlobLocator || par.DataType == OCIFileLocator) {
			if len(par.BValue) > 0 {
				session.WriteUint(&buffer, len(par.BValue), 2, true, true)
			}
			session.WriteClr(&buffer, par.BValue)
		} else {
			if par.cusType != nil {
				session.WriteBytes(&buffer, 0, 0, 0, 0)
				size := len(par.BValue)
				session.WriteUint(&buffer, size, 4, true, true)
				session.WriteBytes(&buffer, 1, 1)
				session.WriteClr(&buffer, par.BValue)
			} else {
				if par.MaxNoOfArrayElements > 0 {
					if par.BValue == nil {
						session.WriteBytes(&buffer, 0)
					} else {
						session.WriteBytes(&buffer, par.BValue...)
					}
				} else {
					session.WriteClr(&buffer, par.BValue)
				}
			}
		}
	}
	if buffer.Len() > 0 {
		session.PutBytes(7)
		session.PutBytes(buffer.Bytes()...)
	}
	return nil
}

// write stmt data to network stream
func (stmt *Stmt) write() error {
	session := stmt.connection.session
	if !stmt.parse && !stmt.reSendParDef {
		exeOf := 0
		execFlag := 0
		count := 1
		if stmt.arrayBindCount > 0 {
			count = stmt.arrayBindCount
		}
		if stmt.stmtType == SELECT {
			session.PutBytes(3, 0x4E, 0)
			count = stmt._noOfRowsToFetch
			exeOf = 0x20
			if stmt._hasReturnClause || stmt.stmtType == PLSQL /*|| stmt.disableCompression*/ {
				exeOf |= 0x40000
			}
		} else {
			session.PutBytes(3, 4, 0)
		}
		if stmt.connection.autoCommit {
			execFlag = 1
		}

		session.PutUint(stmt.cursorID, 2, true, true)
		session.PutUint(count, 2, true, true)
		session.PutUint(exeOf, 2, true, true)
		session.PutUint(execFlag, 2, true, true)
		//err := stmt.writePars()
		//if err != nil {
		//	return err
		//}
		var err error
		if stmt.bulkExec {
			// take copy of parameter values
			arrayValues := make([][][]byte, len(stmt.Pars))
			for x := 0; x < len(stmt.Pars); x++ {
				if stmt.Pars[x].Flag == 0x80 {
					continue
				}
				if tempVal, ok := stmt.Pars[x].iPrimValue.([][]byte); ok {
					arrayValues[x] = tempVal
				} else {
					return errors.New("")
				}
			}
			for valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
				for parIndex, arrayValue := range arrayValues {
					if stmt.Pars[parIndex].Flag == 0x80 {
						continue
					}
					stmt.Pars[parIndex].BValue = arrayValue[valueIndex]
				}
				err = stmt.writePars()
				if err != nil {
					return err
				}
			}

			//for valueIndex, values := range arrayValue {
			//	stmt.Pars[parIndex].BValue = values[valueIndex]
			//}

			// valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
			// each value represented an array of []byte

			//}
			//for valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
			//	for parIndex, arrayValue := range arrayValues {
			//		tempVal := reflect.ValueOf(arrayValue)
			//		err = stmt.Pars[parIndex].encodeValue(tempVal.Index(valueIndex).Interface(), 0, stmt.connection)
			//		if err != nil {
			//			return err
			//		}
			//	}
			//	err = stmt.writePars()
			//	if err != nil {
			//		return err
			//	}
			//}
		} else {
			err = stmt.writePars()
			if err != nil {
				return err
			}
		}
	} else {
		//stmt.reExec = true
		err := stmt.basicWrite(stmt.getExeOption(), stmt.parse, stmt.define)
		if err != nil {
			return err
		}
		if stmt.bulkExec {

			arrayValues := make([][][]byte, len(stmt.Pars))
			for x := 0; x < len(stmt.Pars); x++ {
				if stmt.Pars[x].Flag == 0x80 {
					continue
				}
				if tempVal, ok := stmt.Pars[x].iPrimValue.([][]byte); ok {
					arrayValues[x] = tempVal
				} else {
					return errors.New("incorrect array type")
				}
			}
			for valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
				for parIndex, arrayValue := range arrayValues {
					if stmt.Pars[parIndex].Flag == 0x80 {
						continue
					}
					stmt.Pars[parIndex].BValue = arrayValue[valueIndex]
				}
				err = stmt.writePars()
				if err != nil {
					return err
				}
			}
			//arrayValues := make([]driver.Value, len(stmt.Pars))
			//for x := 0; x < len(stmt.Pars); x++ {
			//	if stmt.Pars[x].Flag == 0x80 {
			//		continue
			//	}
			//	arrayValues[x] = stmt.Pars[x].Value
			//}
			//for valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
			//	for parIndex, arrayValue := range arrayValues {
			//		if stmt.Pars[parIndex].Flag == 0x80 {
			//			continue
			//		}
			//		tempVal := reflect.ValueOf(arrayValue)
			//		err = stmt.Pars[parIndex].encodeValue(tempVal.Index(valueIndex).Interface(), 0, stmt.connection)
			//		if err != nil {
			//			return err
			//		}
			//	}
			//	err = stmt.writePars()
			//	if err != nil {
			//		return err
			//	}
			//}
		} else {
			err = stmt.writePars()
			if err != nil {
				return err
			}
		}
		stmt.parse = false
		stmt.define = false
		stmt.reSendParDef = false
	}
	return session.Write()
}

// getExeOption return an integer that act like a flag carry bit value set according
// to stmt properties
func (stmt *Stmt) getExeOption() int {
	op := 0
	if stmt.stmtType == PLSQL || stmt._hasReturnClause {
		op |= 0x40000
	}
	if stmt.arrayBindCount > 1 {
		op |= 0x80000
	}
	if stmt.connection.autoCommit && (stmt.stmtType == DML || stmt.stmtType == PLSQL) {
		op |= 0x100
	}
	if stmt.parse {
		op |= 1
	}
	if stmt.execute {
		op |= 0x20
	}
	if !stmt.parse && !stmt.execute {
		op |= 0x40
	}
	if len(stmt.Pars) > 0 && !stmt.define {
		op |= 0x8
		if stmt.stmtType == PLSQL || stmt._hasReturnClause {
			op |= 0x400
		}
	}
	if stmt.stmtType != PLSQL && !stmt._hasReturnClause {
		op |= 0x8000
	}
	if stmt.define {
		op |= 0x10
	}
	return op

	/* HasReturnClause
	if  stmt.PLSQL or cmdText == "" return false
	Regex.IsMatch(cmdText, "\\bRETURNING\\b"
	*/
}

// fetch get more rows from network stream
func (stmt *defaultStmt) fetch(dataSet *DataSet) error {
	if stmt._noOfRowsToFetch == 25 {
		//m_maxRowSize = m_maxRowSize + m_numOfLOBColumns * Math.Max(86, 86 + (int) lobSize) + m_numOfLONGColumns * Math.Max(2, longSize) + m_numOfBFileColumns * 86;
		maxRowSize := 0
		for _, col := range dataSet.Cols {
			if col.DataType == OCIClobLocator || col.DataType == OCIBlobLocator {
				maxRowSize += 86
			} else if col.DataType == LONG || col.DataType == LongRaw {
				maxRowSize += 2
			} else if col.DataType == OCIFileLocator {
				maxRowSize += 86
			} else {
				maxRowSize += col.MaxLen
			}
		}
		if maxRowSize > 0 {
			stmt._noOfRowsToFetch = (0x20000 / maxRowSize) + 1
		}
		stmt.connection.connOption.Tracer.Printf("Fetch Size Calculated: %d", stmt._noOfRowsToFetch)
	}

	tracer := stmt.connection.connOption.Tracer
	var err = stmt._fetch(dataSet)
	if err != nil {
		if isBadConn(err) {
			stmt.connection.setBad()
			tracer.Print("Error: ", err)
			return driver.ErrBadConn
		}
		return err
	}
	//for colIndex, col := range dataSet.Cols {
	//	if col.DataType == REFCURSOR {
	//		for rowIndex, row := range dataSet.rows {
	//			if cursor, ok := row[colIndex].(*RefCursor); ok {
	//				dataSet.rows[rowIndex][colIndex], err = cursor.Query()
	//				if err != nil {
	//					return err
	//				}
	//			}
	//		}
	//	}
	//}
	return nil
}

func (stmt *defaultStmt) _fetch(dataSet *DataSet) error {
	session := stmt.connection.session
	//defer func() {
	//	err := stmt.freeTemporaryLobs()
	//	if err != nil {
	//		stmt.connection.connOption.Tracer.Printf("Error free temporary lobs: %v", err)
	//	}
	//}()
	session.ResetBuffer()
	session.PutBytes(3, 5, 0)
	session.PutInt(stmt.cursorID, 2, true, true)
	session.PutInt(stmt._noOfRowsToFetch, 2, true, true)
	err := session.Write()
	if err != nil {
		return err
	}
	err = stmt.read(dataSet)
	if err != nil {
		return err
	}
	//if stmt.connection.connOption.Lob > 0 {
	//
	//}
	return stmt.decodePrim(dataSet)
	//return nil
}
func (stmt *defaultStmt) queryLobPrefetch(exeOp int, dataSet *DataSet) error {
	if stmt._noOfRowsToFetch == 25 {
		//m_maxRowSize = m_maxRowSize + m_numOfLOBColumns * Math.Max(86, 86 + (int) lobSize) + m_numOfLONGColumns * Math.Max(2, longSize) + m_numOfBFileColumns * 86;
		maxRowSize := 0
		for _, col := range dataSet.Cols {
			if col.DataType == OCIClobLocator || col.DataType == OCIBlobLocator {
				maxRowSize += 86
			} else if col.DataType == LONG || col.DataType == LongRaw {
				maxRowSize += 2
			} else if col.DataType == OCIFileLocator {
				maxRowSize += 86
			} else {
				maxRowSize += col.MaxLen
			}
		}
		if maxRowSize > 0 {
			stmt._noOfRowsToFetch = (0x20000 / maxRowSize) + 1
		}
		stmt.connection.connOption.Tracer.Printf("Fetch Size Calculated: %d", stmt._noOfRowsToFetch)
	}
	stmt.connection.session.ResetBuffer()
	err := stmt.basicWrite(exeOp, false, true)
	if err != nil {
		return err
	}
	//err = stmt.writePars()
	//if err != nil {
	//	return err
	//}
	err = stmt.connection.session.Write()
	if err != nil {
		return err
	}
	return stmt.read(dataSet)
}

// read this is common read for stmt it read much information related to
// columns, dataset information, output parameter information, rows values
// and at the end summary object about this operation
func (stmt *defaultStmt) read(dataSet *DataSet) error {
	loop := true
	after7 := false
	dataSet.parent = stmt
	session := stmt.connection.session
	for loop {
		msg, err := session.GetByte()
		if err != nil {
			if session.Summary != nil {
				stmt.cursorID = stmt.connection.session.Summary.CursorID
			}
			return err
		}
		switch msg {
		case 6:
			//_, err = session.GetByte()
			err = dataSet.load(session)
			if err != nil {
				return err
			}
			if !after7 {
				if stmt.stmtType == SELECT {
					//b, _ := session.GetBytes(0x10)
					//fmt.Printf("%#v\n", b)
					//return errors.New("interrupt")
				}
			}
		case 7:
			after7 = true
			if stmt._hasReturnClause && stmt.containOutputPars {
				for x := 0; x < len(stmt.Pars); x++ {
					if stmt.Pars[x].Direction == Output {
						num, err := session.GetInt(4, true, true)
						if err != nil {
							return err
						}
						if num > 1 {
							return errors.New("more than one row affected with return clause")
						}
						if num == 0 {
							stmt.Pars[x].BValue = nil
							stmt.Pars[x].Value = nil
						} else {
							err = stmt.calculateParameterValue(&stmt.Pars[x])
							if err != nil {
								return err
							}
							_, err = session.GetInt(2, true, true)
							if err != nil {
								return err
							}
						}
					}
				}
			} else {
				if stmt.containOutputPars {
					for x := 0; x < len(stmt.Pars); x++ {
						if stmt.Pars[x].DataType == REFCURSOR {
							typ := reflect.TypeOf(stmt.Pars[x].Value)
							if typ.Kind() == reflect.Ptr {
								if cursor, ok := stmt.Pars[x].Value.(*RefCursor); ok {
									cursor.connection = stmt.connection
									cursor.parent = stmt
									cursor.autoClose = true
									err = cursor.load()
									if err != nil {
										return err
									}
									if stmt.stmtType == PLSQL {
										_, err = session.GetInt(2, true, true)
										if err != nil {
											return err
										}
									}
								} else {
									return errors.New("RefCursor parameter should contain pointer to  RefCursor struct")
								}
							} else {
								return errors.New("RefCursor parameter should contain pointer to  RefCursor struct")
							}
						} else {
							if stmt.Pars[x].Direction != Input {
								err = stmt.calculateParameterValue(&stmt.Pars[x])
								if err != nil {
									return err
								}
								_, err = session.GetInt(2, true, true)
								if err != nil {
									return err
								}
							} else {
								//_, err = session.GetClr()
							}

						}
					}
				} else {
					// see if it is re-executed
					if len(dataSet.Cols) == 0 && len(stmt.columns) > 0 {
						dataSet.Cols = make([]ParameterInfo, len(stmt.columns))
						copy(dataSet.Cols, stmt.columns)
					}
					for x := 0; x < len(dataSet.Cols); x++ {
						if dataSet.Cols[x].getDataFromServer {
							err = stmt.calculateColumnValue(&dataSet.Cols[x], false)
							if err != nil {
								return err
							}
							if dataSet.Cols[x].DataType == LONG || dataSet.Cols[x].DataType == LongRaw {
								_, err = session.GetInt(4, true, true)
								if err != nil {
									return err
								}
								_, err = session.GetInt(4, true, true)
								if err != nil {
									return err
								}
							}
						}
					}
					newRow := make(Row, dataSet.columnCount)
					for x := 0; x < len(dataSet.Cols); x++ {
						newRow[x] = dataSet.Cols[x].oPrimValue
					}
					//copy(newRow, dataSet.currentRow)
					dataSet.rows = append(dataSet.rows, newRow)
				}
			}
		case 8:
			size, err := session.GetInt(2, true, true)
			if err != nil {
				return err
			}
			for x := 0; x < 2; x++ {
				stmt.scnForSnapshot[x], err = session.GetInt(4, true, true)
				if err != nil {
					return err
				}
			}
			for x := 2; x < size; x++ {
				_, err = session.GetInt(4, true, true)
				if err != nil {
					return err
				}
			}
			_, err = session.GetInt(2, true, true)
			if err != nil {
				return err
			}
			size, err = session.GetInt(2, true, true)
			for x := 0; x < size; x++ {
				_, val, num, err := session.GetKeyVal()
				if err != nil {
					return err
				}
				//fmt.Println(key, val, num)
				if num == 163 {
					session.TimeZone = val
					//fmt.Println("session time zone", session.TimeZone)
				}
			}
			if session.TTCVersion >= 4 {
				// get queryID
				size, err = session.GetInt(4, true, true)
				if err != nil {
					return err
				}
				if size > 0 {
					bty, err := session.GetBytes(size)
					if err != nil {
						return err
					}
					if len(bty) >= 8 {
						stmt.queryID = binary.LittleEndian.Uint64(bty[size-8:])
						fmt.Println("query ID: ", stmt.queryID)
					}
				}
			}
			if session.TTCVersion >= 7 && stmt.stmtType == DML && stmt.arrayBindCount > 0 {
				length, err := session.GetInt(4, true, true)
				if err != nil {
					return err
				}
				//for (int index = 0; index < length3; ++index)
				//	rowsAffectedByArrayBind[index] = this.m_marshallingEngine.UnmarshalSB8();
				for i := 0; i < length; i++ {
					_, err = session.GetInt(8, true, true)
					if err != nil {
						return err
					}
				}
			}
		case 11:
			err = dataSet.load(session)
			if err != nil {
				return err
			}
			//dataSet.BindDirections = make([]byte, dataSet.columnCount)
			for x := 0; x < dataSet.columnCount; x++ {
				direction, err := session.GetByte()
				switch direction {
				case 32:
					stmt.Pars[x].Direction = Input
				case 16:
					stmt.Pars[x].Direction = Output
					stmt.containOutputPars = true
				case 48:
					stmt.Pars[x].Direction = InOut
					stmt.containOutputPars = true
				}
				if err != nil {
					return err
				}
			}
		case 16:
			size, err := session.GetByte()
			if err != nil {
				return err
			}
			_, err = session.GetBytes(int(size))
			if err != nil {
				return err
			}
			dataSet.maxRowSize, err = session.GetInt(4, true, true)
			if err != nil {
				return err
			}
			dataSet.columnCount, err = session.GetInt(4, true, true)
			if err != nil {
				return err
			}
			if dataSet.columnCount > 0 {
				_, err = session.GetByte() // session.GetInt(1, false, false)
			}
			dataSet.Cols = make([]ParameterInfo, dataSet.columnCount)
			for x := 0; x < dataSet.columnCount; x++ {
				err = dataSet.Cols[x].load(stmt.connection)
				if err != nil {
					return err
				}
				if dataSet.Cols[x].DataType == LONG || dataSet.Cols[x].DataType == LongRaw {
					stmt._hasLONG = true
				}
				if dataSet.Cols[x].DataType == OCIClobLocator || dataSet.Cols[x].DataType == OCIBlobLocator ||
					dataSet.Cols[x].DataType == OCIFileLocator {
					stmt._hasBLOB = true
				}
			}
			stmt.columns = make([]ParameterInfo, dataSet.columnCount)
			copy(stmt.columns, dataSet.Cols)
			_, err = session.GetDlc()
			if session.TTCVersion >= 3 {
				_, err = session.GetInt(4, true, true)
				_, err = session.GetInt(4, true, true)
			}
			if session.TTCVersion >= 4 {
				_, err = session.GetInt(4, true, true)
				_, err = session.GetInt(4, true, true)
			}
			if session.TTCVersion >= 5 {
				_, err = session.GetDlc()
			}
		case 19:
			session.ResetBuffer()
			session.PutBytes(19)
			err = session.Write()
			if err != nil {
				return err
			}
			continue
		case 21:
			_, err := session.GetInt(2, true, true) // noOfColumnSent
			if err != nil {
				return err
			}
			bitVectorLen := dataSet.columnCount / 8
			if dataSet.columnCount%8 > 0 {
				bitVectorLen++
			}
			bitVector := make([]byte, bitVectorLen)
			for x := 0; x < bitVectorLen; x++ {
				bitVector[x], err = session.GetByte()
				if err != nil {
					return err
				}
			}
			dataSet.setBitVector(bitVector)
		case 27:
			count, err := session.GetInt(4, true, true)
			if err != nil {
				return err
			}
			for x := 0; x < count; x++ {
				//refCursorAccessor.UnmarshalOneRow();
				// this function is equal to load cursor so each item is a cursor
				cursor := RefCursor{}
				cursor.connection = stmt.connection
				cursor.parent = stmt
				cursor.autoClose = true
				err = cursor.load()
				if err != nil {
					return err
				}
				// what we will do with cursor?
			}
			//internal List<TTCResultSet> ProcessImplicitResultSet(
			//ref List<TTCResultSet> implicitRSList)
			//{
			//int num = (int) this.m_marshallingEngine.UnmarshalUB4();
			//TTCRefCursorAccessor refCursorAccessor = new TTCRefCursorAccessor((ColumnDescribeInfo) null, this.m_marshallingEngine);
			//for (int index = 0; index < num; ++index)
			//refCursorAccessor.UnmarshalOneRow();
			//if (implicitRSList != null)
			//implicitRSList.AddRange((IEnumerable<TTCResultSet>) refCursorAccessor.m_TTCResultSetList);
			//else
			//implicitRSList = refCursorAccessor.m_TTCResultSetList;
			//return implicitRSList;
			//}
		default:
			err = stmt.connection.readResponse(msg)
			if err != nil {
				return err
			}
			if msg == 4 {
				stmt.cursorID = stmt.connection.session.Summary.CursorID
				//stmt.disableCompression = stmt.connection.session.Summary.Flags&0x20 != 0
				if stmt.connection.session.HasError() {
					if stmt.connection.session.Summary.RetCode == 1403 {
						stmt._hasMoreRows = false
						stmt.connection.session.Summary = nil
					} else {
						return stmt.connection.session.GetError()
					}

				}
				loop = false
			} else if msg == 9 {
				loop = false
			}
			//return errors.New(fmt.Sprintf("TTC error: received code %d during stmt reading", msg))
		}
	}
	//if session.IsBreak() {
	//	err := (&simpleObject{
	//		connection: stmt.connection,
	//	}).read()
	//	if err != nil {
	//		return err
	//	}
	//}
	if stmt.connection.connOption.Tracer.IsOn() {
		dataSet.Trace(stmt.connection.connOption.Tracer)
	}
	//return stmt.readLobs(dataSet)
	return nil
}

func (stmt *defaultStmt) freeTemporaryLobs() error {
	//var locators = collectLocators(stmt.Pars)
	if len(stmt.temporaryLobs) == 0 {
		return nil
	}
	stmt.connection.connOption.Tracer.Printf("Free %d Temporary Lobs", len(stmt.temporaryLobs))
	session := stmt.connection.session
	defer func(input *[][]byte) {
		*input = nil
	}(&stmt.temporaryLobs)
	freeTemp := func(locators [][]byte) {
		totalLen := 0
		for _, locator := range locators {
			totalLen += len(locator)
		}
		session.PutBytes(0x11, 0x60, 0, 1)
		session.PutUint(totalLen, 4, true, true)
		session.PutBytes(0, 0, 0, 0, 0, 0, 0)
		session.PutUint(0x80111, 4, true, true)
		session.PutBytes(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		for _, locator := range locators {
			session.PutBytes(locator...)
		}
	}
	start := 0
	end := 0
	session.ResetBuffer()
	for start < len(stmt.temporaryLobs) {
		end = start + 25000
		if end > len(stmt.temporaryLobs) {
			end = len(stmt.temporaryLobs)
		}
		freeTemp(stmt.temporaryLobs[start:end])
		start += end
	}
	session.PutBytes(0x3, 0x93, 0x0)
	err := session.Write()
	if err != nil {
		return err
	}
	return (&simpleObject{
		connection: stmt.connection,
	}).read()
}

// requestCustomTypeInfo an experimental function to ask for UDT information
func (stmt *defaultStmt) requestCustomTypeInfo(typeName string) error {
	session := stmt.connection.session
	session.SaveState(nil)
	session.PutBytes(0x3, 0x5c, 0)
	session.PutInt(3, 4, true, true)
	//session.PutInt(0x5C0003, 4, true, true)
	//session.PutBytes(bytes.Repeat([]byte{0}, 79)...)

	session.PutBytes(bytes.Repeat([]byte{0}, 19)...)
	session.PutInt(2, 4, true, true)
	//session.PutBytes(2)
	session.PutInt(len(stmt.connection.connOption.UserID), 4, true, true)
	//session.PutBytes(0, 0, 0)
	session.PutClr(stmt.connection.sStrConv.Encode(stmt.connection.connOption.UserID))
	session.PutInt(len(typeName), 4, true, true)
	//session.PutBytes(0, 0, 0)
	session.PutClr(stmt.connection.sStrConv.Encode(typeName))
	//session.PutBytes(0, 0, 0)
	//if session.TTCVersion >= 4 {
	//	session.PutBytes(0, 0, 1)
	//}
	//if session.TTCVersion >= 5 {
	//	session.PutBytes(0, 0, 0, 0, 0)
	//}
	//if session.TTCVersion >= 7 {
	//	if stmt.stmtType == DML && stmt.arrayBindCount > 0 {
	//		session.PutBytes(1)
	//		session.PutInt(stmt.arrayBindCount, 4, true, true)
	//		session.PutBytes(1)
	//	} else {
	//		session.PutBytes(0, 0, 0)
	//	}
	//}
	//if session.TTCVersion >= 8 {
	//	session.PutBytes(0, 0, 0, 0, 0)
	//}
	//if session.TTCVersion >= 9 {
	//	session.PutBytes(0, 0)
	//}
	//session.PutBytes(0, 0)
	//session.PutInt(1, 4, true, true)
	//session.PutBytes(0)
	session.PutBytes(0, 0, 0, 0, 0, 1, 0, 0, 0, 0)
	session.PutBytes(bytes.Repeat([]byte{0}, 50)...)
	//session.PutBytes(0)
	//session.PutInt(0x10000, 4, true, true)
	//session.PutBytes(0, 0)
	err := session.Write()
	if err != nil {
		return err
	}
	data, err := session.GetBytes(0x10)
	if err != nil {
		return err
	}
	fmt.Printf("%#v\n", data)
	session.LoadState()
	return nil
}

func (stmt *defaultStmt) calculateColumnValue(col *ParameterInfo, udt bool) error {
	session := stmt.connection.session
	//if col.DataType == OCIBlobLocator || col.DataType == OCIClobLocator {
	//	stmt._hasBLOB = true
	//}
	if col.DataType == REFCURSOR {
		var cursor = new(RefCursor)
		cursor.connection = stmt.connection
		cursor.parent = stmt
		cursor.autoClose = true
		err := cursor.load()
		if err != nil {
			return err
		}
		if stmt.stmtType == PLSQL {
			_, err = session.GetInt(2, true, true)
			if err != nil {
				return err
			}
		}
		//col.Value = cursor
		col.oPrimValue = cursor
		return nil
	}

	return col.decodeColumnValue(stmt.connection, &stmt.temporaryLobs, udt)
}

// get values of rows and output parameter according to DataType and binary value (bValue)
func (stmt *defaultStmt) calculateParameterValue(param *ParameterInfo) error {
	if param.DataType == OCIBlobLocator || param.DataType == OCIClobLocator || param.DataType == OCIFileLocator {
		stmt._hasBLOB = true
	}
	return param.decodeParameterValue(stmt.connection, &stmt.temporaryLobs)
}

// Close stmt cursor in the server
func (stmt *defaultStmt) Close() error {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return driver.ErrBadConn
	}
	err := stmt.freeTemporaryLobs()
	if err != nil {
		stmt.connection.connOption.Tracer.Printf("Error free temporary lobs: %v", err)
	}
	if stmt.cursorID != 0 {
		session := stmt.connection.session
		session.ResetBuffer()
		session.PutBytes(0x11, 0x69, 0, 1, 1, 1)
		session.PutInt(stmt.cursorID, 4, true, true)
		return (&simpleObject{
			connection:  stmt.connection,
			operationID: 0x93,
			data:        nil,
			err:         nil,
		}).exec()
	}
	return nil
}

func (stmt *Stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.connOption.Tracer
	tracer.Printf("Exec With Context:")
	stmt.connection.session.StartContext(ctx)
	defer stmt.connection.session.EndContext()
	tracer.Printf("Exec:\n%s", stmt.text)
	stmt.arrayBindCount = 0
	result, err := stmt._exec(args)
	if err != nil {
		if isBadConn(err) {
			//tracer.Print("Error: ", err)
			stmt.connection.setBad()
			return nil, err
		}
		return nil, err
	}
	return result, nil
}
func (stmt *Stmt) fillStructPar(parValue driver.Value) error {
	structType := reflect.TypeOf(parValue)
	structVal := reflect.ValueOf(parValue)
	if parValue != nil && structType.Kind() == reflect.Ptr && structVal.Elem().Kind() == reflect.Struct {
		structType = structType.Elem()
		structVal = structVal.Elem()
		structFieldCount := structType.NumField()
		for i := 0; i < structFieldCount; i++ {
			name, _type, _, dir := extractTag(structType.Field(i).Tag.Get("db"))
			var err error
			if len(name) > 0 && dir != Input && len(_type) > 0 {
				for _, par := range stmt.Pars {
					if par.Name == name {
						fieldValue := structVal.Field(i)
						fieldType := structVal.Field(i).Type()
						switch tempVal := par.Value.(type) {
						case *sql.NullFloat64:
							if tempVal.Valid {
								err = setNumber(fieldValue, tempVal.Float64)
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *sql.NullString:
							if tempVal.Valid {
								err = setString(fieldValue, tempVal.String)
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *NullNVarChar:
							if tempVal.Valid {
								err = setString(fieldValue, string(tempVal.NVarChar))
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *sql.NullTime:
							if tempVal.Valid {
								err = setTime(fieldValue, tempVal.Time)
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *NullTimeStamp:
							if tempVal.Valid {
								err = setTime(fieldValue, time.Time(tempVal.TimeStamp))
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *NullTimeStampTZ:
							if tempVal.Valid {
								err = setTime(fieldValue, time.Time(tempVal.TimeStampTZ))
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *[]byte:
							if tempVal == nil {
								fieldValue.Set(reflect.Zero(fieldType))
							} else {
								err = setBytes(fieldValue, *tempVal)
							}
						case *Clob:
							if tempVal.Valid {
								err = setString(fieldValue, tempVal.String)
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *NClob:
							if tempVal.Valid {
								err = setString(fieldValue, tempVal.String)
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						case *Blob:
							if tempVal.Valid {
								err = setBytes(fieldValue, tempVal.Data)
							} else {
								fieldValue.Set(reflect.Zero(fieldType))
							}
						default:
							return errors.New("unknown go type associated with " + _type)
						}
						if err != nil {
							return err
						}

						//var pFieldValue reflect.Value
						//if fieldValue.Kind() != reflect.Ptr && fieldValue.CanAddr() {
						//	pFieldValue = fieldValue.Addr()
						//} else {
						//	pFieldValue = fieldValue
						//}
						//if _, ok := pFieldValue.Interface().(sql.Scanner); ok {
						//err := scanner.Scan(par.Value)
						//if err != nil {
						//	return err
						//}
						//continue
						//}
						//if valuer, ok := par.Value.(driver.Valuer); ok {
						//	tempVal, err := valuer.Value()
						//	if err != nil {
						//		return err
						//	}
						//	if tempVal == nil {
						//		fieldValue.Set(reflect.Zero(fieldType))
						//	} else {
						//		if fieldType.Kind() == reflect.Ptr {
						//			if fieldValue.IsNil() {
						//				temp := reflect.New(fieldType.Elem())
						//				fieldValue.Set(temp)
						//			}
						//			fieldValue = fieldValue.Elem()
						//		}
						//		if scanner, ok := fieldValue.Interface().(sql.Scanner); ok {
						//			err = scanner.Scan(par.Value)
						//			if err != nil {
						//				return err
						//			}
						//			continue
						//		}
						//	}
						//} else {
						//
						//}
					}
				}
			}
		}
	}
	return nil
}

func (stmt *Stmt) structPar(parValue driver.Value, parIndex int) (processedPars int, err error) {
	tempType := reflect.TypeOf(parValue)
	tempVal := reflect.ValueOf(parValue)
	addOutputField := func(name, _type string, size int, dir ParameterDirection, fieldIndex int) (tempPar *ParameterInfo, err error) {
		field := tempVal.Field(fieldIndex)
		fieldValue := field.Interface()
		fieldType := field.Type()
		hasNullValue := false
		if fieldType.Kind() == reflect.Ptr {
			if tempVal.Field(fieldIndex).IsNil() {
				hasNullValue = true
				fieldType = fieldType.Elem()
			}
		}
		// if type mentioned so driver should create a temporary type and then update the current value
		typeErr := fmt.Errorf("error passing filed %s as type %s", tempType.Field(fieldIndex).Name, _type)
		switch _type {
		case "number":
			var fieldVal = &sql.NullFloat64{}
			if !hasNullValue {
				fieldVal.Float64, err = getFloat(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
				fieldVal.Valid = true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "varchar":
			var fieldVal = &sql.NullString{}
			if !hasNullValue {
				fieldVal.String, fieldVal.Valid = getString(fieldValue), true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "nvarchar":
			var fieldVal = &NullNVarChar{}
			if !hasNullValue {
				fieldVal.NVarChar, fieldVal.Valid = NVarChar(getString(fieldValue)), true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "date":
			var fieldVal = &sql.NullTime{}
			if !hasNullValue {
				fieldVal.Time, err = getDate(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
				fieldVal.Valid = true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "timestamp":
			var fieldVal = &NullTimeStamp{}
			if !hasNullValue {
				var tempDate time.Time
				tempDate, err = getDate(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
				fieldVal.TimeStamp = TimeStamp(tempDate)
				fieldVal.Valid = true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "timestamptz":
			var fieldVal = &NullTimeStampTZ{}
			if !hasNullValue {
				var tempDate time.Time
				tempDate, err = getDate(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
				fieldVal.TimeStampTZ = TimeStampTZ(tempDate)
				fieldVal.Valid = true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "raw":
			var fieldVal []byte
			if !hasNullValue {
				fieldVal, err = getBytes(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
			}
			tempPar, err = stmt.NewParam(name, &fieldVal, size, dir)
		case "clob":
			fieldVal := &Clob{}
			if !hasNullValue {
				fieldVal.String, fieldVal.Valid = getString(fieldValue), true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "nclob":
			fieldVal := &NClob{}
			if !hasNullValue {
				fieldVal.String, fieldVal.Valid = getString(fieldValue), true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "blob":
			fieldVal := &Blob{}
			if !hasNullValue {
				fieldVal.Data, err = getBytes(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "":
			if field.Kind() == reflect.Ptr {
				if field.IsNil() {
					field.Set(reflect.New(fieldType))
				}
				tempPar, err = stmt.NewParam(name, field.Interface(), size, dir)
			} else {
				if field.CanAddr() {
					tempPar, err = stmt.NewParam(name, field.Addr().Interface(), size, dir)
				} else {
					err = fmt.Errorf("can't take address for field: %s", name)
				}
			}
		default:
			err = fmt.Errorf("unknown type: %s for parameter: %s", _type, name)
		}
		return
		//if _, ok := fieldValue.(driver.Valuer); ok {
		//	if _, ok = tempVal.Field(fieldIndex).Addr().Interface().(sql.Scanner); ok {
		//		tempPar, err = stmt.NewParam(name, tempVal.Field(fieldIndex).Addr().Interface(), size, dir)
		//		return
		//	}
		//}
		//if len(_type) > 0 {
		//} else {
		//	//fieldType := reflect.TypeOf(fieldValue)
		//	if tNumber(fieldType) {
		//		var fieldVal = &sql.NullFloat64{}
		//		if !hasNullValue {
		//			fieldVal.Float64, err = getFloat(fieldValue)
		//			if err != nil {
		//				err = typeErr
		//				return
		//			}
		//			fieldVal.Valid = true
		//		}
		//		tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		//	}
		//	switch fieldType.Kind() {
		//	case reflect.Bool:
		//		var fieldVal = &sql.NullFloat64{}
		//		if !hasNullValue {
		//			fieldVal.Float64, err = getFloat(fieldValue)
		//			if err != nil {
		//				err = typeErr
		//				return
		//			}
		//			fieldVal.Valid = true
		//		}
		//		tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		//	case reflect.String:
		//		fieldVal := &sql.NullString{}
		//		if !hasNullValue {
		//			fieldVal.String, fieldVal.Valid = getString(fieldValue), true
		//		}
		//		tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		//	default:
		//		switch aval := fieldValue.(type) {
		//		case NVarChar:
		//			fieldVal := &NullNVarChar{}
		//			if !hasNullValue {
		//				fieldVal.NVarChar, fieldVal.Valid = aval, true
		//			}
		//			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		//		case []byte:
		//			var fieldVal []byte
		//			if !hasNullValue {
		//				fieldVal = aval
		//			}
		//			tempPar, err = stmt.NewParam(name, &fieldVal, size, dir)
		//		case time.Time:
		//			fieldVal := &sql.NullTime{}
		//			if !hasNullValue {
		//				fieldVal.Time, fieldVal.Valid = aval, true
		//			}
		//			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		//		case TimeStamp:
		//			fieldVal := &NullTimeStamp{}
		//			if !hasNullValue {
		//				fieldVal.TimeStamp, fieldVal.Valid = aval, true
		//			}
		//			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		//		case TimeStampTZ:
		//			fieldVal := &NullTimeStampTZ{}
		//			if !hasNullValue {
		//				fieldVal.TimeStampTZ, fieldVal.Valid = aval, true
		//			}
		//			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		//		case Clob:
		//			tempPar, err = stmt.NewParam(name, &aval, size, dir)
		//		case NClob:
		//			tempPar, err = stmt.NewParam(name, &aval, size, dir)
		//		case Blob:
		//			tempPar, err = stmt.NewParam(name, &aval, size, dir)
		//		}
		//	}
		//}
		//return
	}
	addInputField := func(name, _type string, fieldIndex int) (tempPar *ParameterInfo, err error) {
		var fieldValue = tempVal.Field(fieldIndex).Interface()
		if fieldValue == nil {
			tempPar, err = stmt.NewParam(name, fieldValue, 0, Input)
			return
		}
		// value is pointer
		if tempType.Field(fieldIndex).Type.Kind() == reflect.Ptr {
			if tempVal.Field(fieldIndex).IsNil() {
				tempPar, err = stmt.NewParam(name, nil, 0, Input)
				return
			} else {
				fieldValue = tempVal.Field(fieldIndex).Elem().Interface()
			}
		}
		typeErr := fmt.Errorf("error passing field %s as type %s", tempType.Field(fieldIndex).Name, _type)
		switch _type {
		case "number":
			var fieldVal float64
			fieldVal, err = getFloat(fieldValue)
			if err != nil {
				err = typeErr
				return
			}
			tempPar, err = stmt.NewParam(name, fieldVal, 0, Input)
		case "varchar":
			fieldVal := getString(fieldValue)
			tempPar, err = stmt.NewParam(name, fieldVal, 0, Input)
		case "nvarchar":
			fieldVal := getString(fieldValue)
			tempPar, err = stmt.NewParam(name, NVarChar(fieldVal), 0, Input)
		case "date":
			var fieldVal time.Time
			fieldVal, err = getDate(fieldValue)
			if err != nil {
				err = typeErr
				return
			}
			tempPar, err = stmt.NewParam(name, fieldVal, 0, Input)
		case "timestamp":
			var fieldVal time.Time
			fieldVal, err = getDate(fieldValue)
			if err != nil {
				err = typeErr
				return
			}
			tempPar, err = stmt.NewParam(name, TimeStamp(fieldVal), 0, Input)
		case "timestamptz":
			var fieldVal time.Time
			fieldVal, err = getDate(fieldValue)
			if err != nil {
				err = typeErr
				return
			}
			tempPar, err = stmt.NewParam(name, TimeStampTZ(fieldVal), 0, Input)
		case "raw":
			var fieldVal []byte
			fieldVal, err = getBytes(fieldValue)
			if err != nil {
				err = typeErr
				return
			}
			tempPar, err = stmt.NewParam(name, fieldVal, 0, Input)
		case "clob":
			fieldVal := getString(fieldValue)
			tempPar, err = stmt.NewParam(name, Clob{String: fieldVal, Valid: true}, 0, Input)
		case "nclob":
			fieldVal := getString(fieldValue)
			tempPar, err = stmt.NewParam(name, NClob{String: fieldVal, Valid: true}, 0, Input)
		case "blob":
			var fieldVal []byte
			fieldVal, err = getBytes(fieldValue)
			if err != nil {
				err = typeErr
				return
			}
			tempPar, err = stmt.NewParam(name, Blob{Data: fieldVal, Valid: true}, 0, Input)
		case "":
			tempPar, err = stmt.NewParam(name, tempVal.Field(fieldIndex).Interface(), 0, Input)
		default:
			err = typeErr
		}
		return
	}
	// deal with struct types
	if parValue != nil && tempType.Kind() == reflect.Struct {
		structFieldCount := tempType.NumField()

		for i := 0; i < structFieldCount; i++ {
			name, _type, _, _ := extractTag(tempType.Field(i).Tag.Get("db"))
			if name != "" {
				var tempPar *ParameterInfo
				tempPar, err = addInputField(name, _type, i)
				if err != nil {
					return
				}
				stmt.setParam(parIndex, *tempPar)
				processedPars++
				parIndex++
			}
		}
	}

	// deal with Ptr struct types
	if parValue != nil && tempType.Kind() == reflect.Ptr && tempVal.Elem().Kind() == reflect.Struct {
		tempType = tempType.Elem()
		tempVal = tempVal.Elem()
		structFieldCount := tempType.NumField()
		for i := 0; i < structFieldCount; i++ {
			name, _type, size, dir := extractTag(tempType.Field(i).Tag.Get("db"))
			if dir == 0 {
				dir = Input
			}
			if name != "" {
				var tempPar *ParameterInfo
				if dir == Input {
					tempPar, err = addInputField(name, _type, i)
				} else {
					tempPar, err = addOutputField(name, _type, size, dir, i)
				}
				if err != nil {
					return
				}
				stmt.setParam(parIndex, *tempPar)
				processedPars++
				parIndex++
			}
		}
	}
	return
}

func (stmt *Stmt) _exec(args []driver.NamedValue) (*QueryResult, error) {
	var err error
	var useNamedPars = len(args) > 0
	parIndex := 0
	structPars := make([]driver.Value, 0, 2)
	for x := 0; x < len(args); x++ {
		var par *ParameterInfo
		switch tempOut := args[x].Value.(type) {
		case sql.Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, 0, direction)
			if err != nil {
				return nil, err
			}
		case *sql.Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, 0, direction)
			if err != nil {
				return nil, err
			}
		case Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, tempOut.Size, direction)
			if err != nil {
				return nil, err
			}
		case *Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, tempOut.Size, direction)
			if err != nil {
				return nil, err
			}
		default:
			var processedPars = 0
			processedPars, err = stmt.structPar(args[x].Value, parIndex)
			if err != nil {
				return nil, err
			}
			if processedPars > 0 {
				stmt.bulkExec = false
				stmt.connection.connOption.Tracer.Printf("    %d:\n%v", x, args[x])
				parIndex += processedPars
				structPars = append(structPars, args[x].Value)
				continue
			}
			if stmt.bulkExec {
				tempType := reflect.TypeOf(args[x].Value)
				tempVal := reflect.ValueOf(args[x].Value)
				if args[x].Value != nil && tempType != reflect.TypeOf([]byte{}) && (tempType.Kind() == reflect.Array || tempType.Kind() == reflect.Slice) {
					// setup array count
					if stmt.arrayBindCount == 0 {
						stmt.arrayBindCount = tempVal.Len()
					} else {
						if stmt.arrayBindCount > tempVal.Len() {
							stmt.arrayBindCount = tempVal.Len()
						}
					}
					// see if first item is struct
					firstItem := tempVal.Index(0)
					//lobData := make([]*Lob, stmt.arrayBindCount)
					if firstItem.Kind() == reflect.Struct {
						fieldCount := firstItem.NumField()
						structArrayAsNamedPars := make([]driver.NamedValue, 0, fieldCount)
						for i := 0; i < fieldCount; i++ {
							db, _, _, _ := extractTag(firstItem.Type().Field(i).Tag.Get("db"))
							if db != "" {
								arrayValues := make([]driver.Value, stmt.arrayBindCount)
								for x := 0; x < stmt.arrayBindCount; x++ {

									if (tempVal.Index(x).Field(i).Kind() == reflect.Ptr ||
										tempVal.Index(x).Field(i).Kind() == reflect.Slice ||
										tempVal.Index(x).Field(i).Kind() == reflect.Array) && tempVal.Index(x).Field(i).IsNil() {
										arrayValues[x] = nil
									} else {
										arrayValues[x] = tempVal.Index(x).Field(i).Interface()
									}
								}
								structArrayAsNamedPars = append(structArrayAsNamedPars, driver.NamedValue{Name: db, Value: arrayValues})
							}
						}
						if len(structArrayAsNamedPars) > 0 {
							return stmt._exec(structArrayAsNamedPars)
						}
					}

					//err := param.encodeValue(val, size, stmt.connection)
					//if err != nil {
					//	return nil, err
					//}
					//return param, err
					//par, err = stmt.NewParam(args[x].Name, firstItem.Interface(), 0, Input)
					//if err != nil {
					//	return nil, err
					//}

					par = &ParameterInfo{
						Name:      args[x].Name,
						Direction: Input,
					}
					// calculate maxLen, maxCharLen and DataType
					//maxLen := par.MaxLen
					//maxCharLen := par.MaxCharLen
					//dataType := par.DataType
					maxLen := 0
					maxCharLen := 0
					dataType := TNSType(0)
					arrayValues := make([][]byte, stmt.arrayBindCount)
					for y := 0; y < stmt.arrayBindCount; y++ {
						err = par.encodeValue(tempVal.Index(y).Interface(), 0, stmt.connection)
						if err != nil {
							return nil, err
						}
						switch value := par.iPrimValue.(type) {
						case *Lob:
							if value != nil && value.sourceLocator != nil {
								stmt.temporaryLobs = append(stmt.temporaryLobs, value.sourceLocator)
							}
						case *BFile:
							if value != nil && value.lob.sourceLocator != nil {
								stmt.temporaryLobs = append(stmt.temporaryLobs, value.lob.sourceLocator)
							}
						case []ParameterInfo:
							temp := collectLocators(value)
							stmt.temporaryLobs = append(stmt.temporaryLobs, temp...)
						}
						if maxLen < par.MaxLen {
							maxLen = par.MaxLen
						}
						if maxCharLen < par.MaxCharLen {
							maxCharLen = par.MaxCharLen
						}
						// here i can take the binary value and store it into array
						arrayValues[y] = par.BValue
						if len(par.BValue) == 0 && par.DataType == NCHAR {
							continue
						}
						dataType = par.DataType
						//if y == 0 {
						//	dataType = par.DataType
						//} else {
						//	if par.DataType != dataType && par.DataType != NCHAR {
						//
						//	}
						//}
					}
					// save arrayValues into primitive
					par.iPrimValue = arrayValues
					//_ = par.encodeValue(tempVal.Index(0).Interface(), 0, stmt.connection)
					par.MaxLen = maxLen
					par.MaxCharLen = maxCharLen
					if int(dataType) == 0 {
						dataType = NCHAR
					}
					par.DataType = dataType
				} else {
					if stmt.arrayBindCount > 0 {
						return nil, errors.New("to activate bulk insert/merge all parameters should be arrays")
					}
					stmt.bulkExec = false
				}
			}
			if par == nil {
				par, err = stmt.NewParam(args[x].Name, args[x].Value, 0, Input)
				if err != nil {
					return nil, err
				}
			}

		}
		if len(par.Name) == 0 && useNamedPars {
			useNamedPars = false
		}
		stmt.setParam(parIndex, *par)
		parIndex++
		stmt.connection.connOption.Tracer.Printf("    %d:\n%v", x, args[x])
	}
	if useNamedPars {
		err = stmt.useNamedParameters()
		if err != nil {
			return nil, err
		}
	}
	//defer func() {
	//	err = stmt.freeTemporaryLobs()
	//	if err != nil {
	//		stmt.connection.connOption.Tracer.Printf("Error free temporary lobs: %v", err)
	//	}
	//}()

	session := stmt.connection.session
	session.ResetBuffer()
	err = stmt.write()
	if err != nil {
		stmt.connection.setBad()
		return nil, err
	}
	dataSet := new(DataSet)
	err = stmt.read(dataSet)
	if err != nil {
		return nil, err
	}
	// need to deal with lobs
	//err = stmt.readLobs(dataSet)
	//if err != nil {
	//	return nil, err
	//}

	// before release results decode parameters
	for _, par := range stmt.Pars {
		if par.Direction != Input && par.DataType != REFCURSOR {
			fieldValue := reflect.ValueOf(par.Value)
			if fieldValue.Kind() != reflect.Ptr {
				return nil, errors.New("output parameter should be pointer type")
			}
			fieldValue = fieldValue.Elem()
			if par.MaxNoOfArrayElements > 0 {
				if pars, ok := par.oPrimValue.([]ParameterInfo); ok {
					err = setArray(fieldValue, pars)
					if err != nil {
						return nil, err
					}
				}
			} else {
				err = setFieldValue(fieldValue, par.cusType, par.oPrimValue)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	result := new(QueryResult)
	if session.Summary != nil {
		result.rowsAffected = int64(session.Summary.CurRowNumber)
	}
	for _, par := range structPars {
		err = stmt.fillStructPar(par)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// useNamedParameters: re-arrange parameters according parameter defined in sql text
func (stmt *Stmt) useNamedParameters() error {
	names, err := parseSqlText(stmt.text)
	if err != nil {
		return err
	}
	var parCollection = make([]ParameterInfo, 0, len(names))
	if stmt.stmtType == SELECT || stmt.stmtType == DML {
		for x := 0; x < len(names); x++ {
			found := false
			for _, par := range stmt.Pars {
				if par.Name == names[x] {
					parCollection = append(parCollection, par)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("parameter %s is not defined in parameter list", names[x])
			}
			for y := x - 1; y >= 0; y-- {
				if names[y] == names[x] {
					parCollection[x].Flag = 0x80
					break
				}
			}
		}
	} else {
		for x := 0; x < len(names); x++ {
			// search if name is repeated
			repeated := false
			for y := x - 1; y >= 0; y-- {
				if names[y] == names[x] {
					repeated = true
					//parCollection[x].Flag = 0x80
					break
				}
			}
			found := false
			for _, par := range stmt.Pars {
				if par.Name == names[x] {
					if !repeated {
						parCollection = append(parCollection, par)
					}
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("parameter %s is not defined in parameter list", names[x])
			}

		}
	}

	stmt.Pars = parCollection
	return nil
}

// Exec execute stmt (INSERT, UPDATE, DELETE, DML, PLSQL) and return driver.Result object
func (stmt *Stmt) Exec(args []driver.Value) (driver.Result, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.connOption.Tracer
	tracer.Printf("Exec:\n%s", stmt.text)
	var result *QueryResult
	var err error
	stmt.arrayBindCount = 0
	if len(args) == 0 {
		result, err = stmt._exec(nil)
	} else {
		var namedArgs = make([]driver.NamedValue, len(args))
		for x := 0; x < len(args); x++ {
			namedArgs[x].Value = args[x]
		}
		result, err = stmt._exec(namedArgs)
	}
	if err != nil {
		if isBadConn(err) {
			stmt.connection.setBad()
			tracer.Print("Error: ", err)
			return nil, driver.ErrBadConn
		}
		return nil, err
	}
	return result, err
}

func (stmt *Stmt) CheckNamedValue(_ *driver.NamedValue) error {
	return nil
}

func (stmt *Stmt) NewParam(name string, val driver.Value, size int, direction ParameterDirection) (*ParameterInfo, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	param := &ParameterInfo{
		Name:      name,
		Direction: direction,
	}
	// initialize bfile
	if file, ok := val.(*BFile); ok {
		if !file.isInit() {
			err := file.init(stmt.connection)
			if err != nil {
				return nil, err
			}
		}
	}
	err := param.encodeValue(val, size, stmt.connection)
	if err != nil {
		return nil, err
	}
	return param, err
}

func (stmt *Stmt) setParam(pos int, par ParameterInfo) {
	if pos >= 0 && pos < len(stmt.Pars) {
		if par.MaxLen > stmt.Pars[pos].MaxLen {
			stmt.reSendParDef = true
		}
		stmt.Pars[pos] = par
	} else {
		stmt.Pars = append(stmt.Pars, par)
	}
	// set temporary lobs
	switch value := par.iPrimValue.(type) {
	case *Lob:
		if value != nil && value.sourceLocator != nil {
			stmt.temporaryLobs = append(stmt.temporaryLobs, value.sourceLocator)
		}
	case *BFile:
		if value != nil && value.lob.sourceLocator != nil {
			stmt.temporaryLobs = append(stmt.temporaryLobs, value.lob.sourceLocator)
		}
	case []ParameterInfo:
		temp := collectLocators(value)
		stmt.temporaryLobs = append(stmt.temporaryLobs, temp...)
	}
}

// addParam
//func (stmt *Stmt) adram(name string, val driver.Value, size int, direction ParameterDirection) error {
//	par, err := stmt.NewParam(name, val, size, direction)
//	if err != nil {
//		return err
//	}
//	stmt.setParam(-1, *par)
//	return nil
//}
// AddRefCursorParam add new output parameter of type REF CURSOR
//
// note: better to use sql.Out structure see examples for more information
//func (stmt *Stmt) AddRefCursorParam(name string) {
//	par, _ := stmt.NewParam(name, new(RefCursor), 0, Output)
//	stmt.Pars = append(stmt.Pars, *par)
//}

// Query_ execute a query command and return oracle dataset object
//
// args is an array of values that corresponding to parameters in sql
func (stmt *Stmt) Query_(namedArgs []driver.NamedValue) (*DataSet, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.connOption.Tracer
	stmt._noOfRowsToFetch = stmt.connection.connOption.PrefetchRows
	stmt._hasMoreRows = true
	var useNamedPars = len(namedArgs) > 0
	for x := 0; x < len(namedArgs); x++ {
		par, err := stmt.NewParam(namedArgs[x].Name, namedArgs[x].Value, 0, Input)
		if err != nil {
			return nil, err
		}
		if len(par.Name) == 0 && useNamedPars {
			useNamedPars = false
		}
		stmt.setParam(x, *par)
		tracer.Printf("    %d:\n%v", x, namedArgs[x])
	}

	if useNamedPars {
		err := stmt.useNamedParameters()
		if err != nil {
			return nil, err
		}
	}

	//failOver := stmt.connection.connOption.Failover
	//retryTime := stmt.connection.connOption.RetryTime
	//if failOver == 0 {
	//	failOver = 1
	//}

	dataSet, err := stmt._query()
	if err != nil {
		if isBadConn(err) {
			stmt.connection.setBad()
			tracer.Print("Error: ", err)
			return nil, driver.ErrBadConn
		}
		return nil, err
	}
	return dataSet, nil
	//	if err == nil {
	//		break
	//	}
	//var dataSet *DataSet
	//var err error
	//
	//var reconnect bool
	//for writeTrials := 0; writeTrials < failOver; writeTrials++ {
	//	reconnect, err = stmt.connection.reConnect(nil, writeTrials+1)
	//	if err != nil {
	//		tracer.Print("Error: ", err)
	//		if !reconnect {
	//			return nil, err
	//		}
	//		continue
	//	}
	//	// reset statement if connection break and reconnect
	//	if writeTrials > 0 {
	//		stmt.reset()
	//	}
	//	// call query
	//	dataSet, err = stmt._query()
	//	if err == nil {
	//		break
	//	}
	//	reconnect, err = stmt.connection.reConnect(err, writeTrials+1)
	//	if err != nil {
	//		tracer.Print("Error: ", err)
	//		if !reconnect {
	//			return nil, err
	//		}
	//	}
	//	if retryTime > 0 {
	//		time.Sleep(time.Duration(retryTime) * time.Second)
	//	}
	//}
	//return dataSet, err

	//result, err := stmt.Query(args)
	//if err != nil {
	//	return nil, err
	//}
	//if dataSet, ok := result.(*DataSet); ok {
	//	return dataSet, nil
	//}
	//return nil, errors.New("the returned driver.rows is not an oracle DataSet")
}

func (stmt *Stmt) QueryContext(ctx context.Context, namedArgs []driver.NamedValue) (driver.Rows, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.connOption.Tracer
	tracer.Print("Query With Context:", stmt.text)

	stmt.connection.session.StartContext(ctx)
	defer stmt.connection.session.EndContext()
	return stmt.Query_(namedArgs)
}

func (stmt *Stmt) reset() {
	stmt.reSendParDef = false
	stmt.parse = true
	stmt.execute = true
	stmt.define = false
	stmt._hasBLOB = false
	stmt._hasLONG = false
	stmt.bulkExec = false
	//stmt.disableCompression = false
	stmt.arrayBindCount = 0
}

func (stmt *Stmt) _query() (*DataSet, error) {
	var err error
	var dataSet *DataSet
	//defer func() {
	//	err = stmt.freeTemporaryLobs()
	//	if err != nil {
	//		stmt.connection.connOption.Tracer.Printf("Error free temporary lobs: %v", err)
	//	}
	//}()
	stmt.connection.session.ResetBuffer()
	err = stmt.write()
	if err != nil {
		return nil, err
	}
	dataSet = new(DataSet)
	err = stmt.read(dataSet)
	if err != nil {
		return nil, err
	}
	// deal with ref cursor
	//for colIndex, col := range dataSet.Cols {
	//	if col.DataType == REFCURSOR {
	//		for rowIndex, row := range dataSet.rows {
	//			if cursor, ok := row[colIndex].(*RefCursor); ok {
	//				dataSet.rows[rowIndex][colIndex], err = cursor.Query()
	//				if err != nil {
	//					return nil, err
	//				}
	//			}
	//		}
	//	}
	//}
	// deal with lobs
	if stmt._hasBLOB {
		if stmt.connection.connOption.Lob == 0 {
			stmt.define = true
			stmt.execute = false
			stmt.parse = false
			stmt.reSendParDef = false
			err = stmt.queryLobPrefetch(stmt.getExeOption(), dataSet)
			if err != nil {
				return nil, err
			}
		} else {
			//err = stmt.readLobs(dataSet)
			//if err != nil {
			//	return nil, err
			//}
		}
	}
	err = stmt.decodePrim(dataSet)
	if err != nil {
		return nil, err
	}
	return dataSet, err
}

func (stmt *defaultStmt) decodePrim(dataSet *DataSet) error {
	var err error
	// convert from go-ora primitives to sql primitives
	for rowIndex, row := range dataSet.rows {
		for colIndex, col := range dataSet.Cols {
			if row == nil {
				continue
			}
			switch val := row[colIndex].(type) {
			case *RefCursor:
				dataSet.rows[rowIndex][colIndex], err = val.Query()
				if err != nil {
					return err
				}
			case Lob:
				if col.DataType == OCIClobLocator {
					var tempString = sql.NullString{"", false}
					err = setLob(reflect.ValueOf(&tempString).Elem(), val)
					if err != nil {
						return err
					}
					if tempString.Valid {
						dataSet.rows[rowIndex][colIndex] = tempString.String
					} else {
						dataSet.rows[rowIndex][colIndex] = nil
					}
				} else {
					var tempByte []byte
					err = setLob(reflect.ValueOf(&tempByte).Elem(), val)
					if err != nil {
						return err
					}
					dataSet.rows[rowIndex][colIndex] = tempByte
				}
			case []ParameterInfo:
				if col.cusType != nil {
					tempObject := reflect.New(col.cusType.typ)
					err = setUDTObject(tempObject.Elem(), col.cusType, val)
					if err != nil {
						return err
					}
					dataSet.rows[rowIndex][colIndex] = tempObject.Elem().Interface()
				}
			}
		}
	}
	return nil
}

// Query execute a query command and return dataset object in form of driver.Rows interface
//
// args is an array of values that corresponding to parameters in sql
func (stmt *Stmt) Query(args []driver.Value) (driver.Rows, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.connOption.Tracer
	tracer.Printf("Query:\n%s", stmt.text)
	var dataSet *DataSet
	var err error
	if len(args) == 0 {
		dataSet, err = stmt.Query_(nil)
	} else {
		var namedArgs = make([]driver.NamedValue, len(args))
		for x := 0; x < len(args); x++ {
			namedArgs[x].Value = args[x]
		}
		dataSet, err = stmt.Query_(namedArgs)
	}
	return dataSet, err
	//stmt._noOfRowsToFetch = stmt.connection.connOption.PrefetchRows
	//stmt._hasMoreRows = true
	//for x := 0; x < len(args); x++ {
	//	par, err := stmt.NewParam("", args[x], 0, Input)
	//	if err != nil {
	//		return nil, err
	//	}
	//	stmt.setParam(x, *par)
	//}
	//
	//// if the connection lost
	//failOver := stmt.connection.connOption.Failover
	//retryTime := stmt.connection.connOption.RetryTime
	//if failOver == 0 {
	//	failOver = 1
	//}
	//var dataSet *DataSet
	//var err error
	//var reconnect bool
	//for writeTrials := 0; writeTrials < failOver; writeTrials++ {
	//	reconnect, err = stmt.connection.reConnect(nil, writeTrials+1)
	//	if err != nil {
	//		tracer.Print("Error: ", err)
	//		if !reconnect {
	//			return nil, err
	//		}
	//		continue
	//	}
	//	// reset statement if connection break and reconnect
	//	if writeTrials > 0 {
	//		stmt.reset()
	//	}
	//	// call query
	//	dataSet, err = stmt._query()
	//	if err == nil {
	//		break
	//	}
	//	reconnect, err = stmt.connection.reConnect(err, writeTrials+1)
	//	if err != nil {
	//		tracer.Print("Error: ", err)
	//		if !reconnect {
	//			return nil, err
	//		}
	//	}
	//	if retryTime > 0 {
	//		time.Sleep(time.Duration(retryTime) * time.Second)
	//	}
	//}
	//return dataSet, err
	//return stmt._query()
}

func (stmt *Stmt) NumInput() int {
	return -1
}

/*
parse = true
execute = true
fetch = true if hasReturn or PLSQL
define = false
*/

//func ReadFromExternalBuffer(buffer []byte) error {
//	connOption := &network.ConnectionOption{
//		Port:                  0,
//		TransportConnectTo:    0,
//		SSLVersion:            "",
//		WalletDict:            "",
//		TransportDataUnitSize: 0,
//		SessionDataUnitSize:   0,
//		Protocol:              "",
//		Host:                  "",
//		UserID:                "",
//		SID:                   "",
//		ServiceName:           "",
//		InstanceName:          "",
//		DomainName:            "",
//		DBName:                "",
//		ClientData:            network.ClientData{},
//		Tracer:                trace.NilTracer(),
//		SNOConfig:             nil,
//	}
//	conn := &Connection {
//		State:             Opened,
//		LogonMode:         0,
//		SessionProperties: nil,
//		connOption: connOption,
//	}
//	conn.session = &network.Session{
//		Context:         nil,
//		Summary:         nil,
//		UseBigClrChunks: true,
//		ClrChunkSize:    0x40,
//	}
//	conn.strConv = converters.NewStringConverter(871)
//	conn.session.StrConv = conn.strConv
//	conn.session.FillInBuffer(buffer)
//	conn.session.TTCVersion = 11
//	stmt := &Stmt{
//		defaultStmt:  defaultStmt{
//			connection: conn,
//			scnForSnapshot: make([]int, 2),
//		},
//		reSendParDef: false,
//		parse:        true,
//		execute:      true,
//		define:       false,
//	}
//	dataSet := new(DataSet)
//	err := stmt.read(dataSet)
//	return err
//}
