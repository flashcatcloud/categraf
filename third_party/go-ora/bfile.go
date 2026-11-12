package go_ora

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"errors"
)

type BFile struct {
	dirName  string
	fileName string
	Valid    bool
	isOpened bool
	lob      Lob
}

func CreateNullBFile() *BFile {
	return &BFile{Valid: false}
}
func CreateBFile(db *sql.DB, dirName, fileName string) (*BFile, error) {
	output := &BFile{fileName: fileName, dirName: dirName, Valid: true}
	_, err := db.Exec("SELECT :1 FROM DUAL", output)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func CreateBFile2(connection *Connection, dirName, fileName string) (*BFile, error) {
	output := &BFile{fileName: fileName, dirName: dirName, Valid: true}
	err := output.init(connection)
	return output, err
}

func (file *BFile) init(conn *Connection) error {
	if file.Valid {
		dirName := conn.sStrConv.Encode(file.dirName)
		fileName := conn.sStrConv.Encode(file.fileName)
		totalLen := 16 + len(dirName) + len(fileName) + 4
		locatorBuffer := new(bytes.Buffer)
		err := binary.Write(locatorBuffer, binary.BigEndian, uint16(totalLen-2))
		if err != nil {
			return err
		}
		locatorBuffer.Write([]byte{0, 1, 8, 8, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0})
		err = binary.Write(locatorBuffer, binary.BigEndian, uint16(len(dirName)))
		if err != nil {
			return err
		}
		if len(dirName) > 0 {
			locatorBuffer.Write(dirName)
		}
		err = binary.Write(locatorBuffer, binary.BigEndian, uint16(len(fileName)))
		if err != nil {
			return err
		}
		if len(fileName) > 0 {
			locatorBuffer.Write(fileName)
		}
		file.lob.connection = conn
		file.lob.sourceLocator = locatorBuffer.Bytes()
		file.lob.sourceLen = locatorBuffer.Len()
	}
	return nil
}
func (file *BFile) GetDirName() string {
	return file.dirName
}
func (file *BFile) GetFileName() string {
	return file.fileName
}
func (file *BFile) IsOpen() bool {
	return file.isOpened
}
func (file *BFile) isInit() bool {
	return len(file.lob.sourceLocator) > 0
}
func (file *BFile) Open() error {
	if file.isOpened {
		return nil
	}
	if !file.isInit() {
		return errors.New("BFile is not initialized")
	}
	err := file.lob.open(0xB, 0x100)
	if err != nil {
		return err
	}
	file.isOpened = true
	return nil
}
func (file *BFile) Close() error {
	if !file.isOpened {
		return nil
	}
	if !file.isInit() {
		return errors.New("BFile is not initialized")
	}
	err := file.lob.close(0x200)
	if err != nil {
		return err
	}
	file.isOpened = false
	return nil
}
func (file *BFile) Exists() (bool, error) {
	if !file.isOpened {
		return false, errors.New("invalid operation on closed object")
	}
	if !file.isInit() {
		return false, errors.New("BFile is not initialized")
	}
	file.lob.initialize()
	file.lob.bNullO2U = true
	session := file.lob.connection.session
	session.ResetBuffer()
	file.lob.writeOp(0x800)
	err := session.Write()
	if err != nil {
		return false, err
	}
	err = file.lob.read()
	if err != nil {
		return false, err
	}
	return file.lob.isNull, nil
}

func (file *BFile) GetLength() (int64, error) {
	if !file.isOpened {
		return 0, errors.New("invalid operation on closed object")
	}
	return file.lob.getSize()
}

func (file *BFile) Read() ([]byte, error) {
	return file.lob.getDataWithOffsetSize(0, 0)
}
func (file *BFile) ReadFromPos(pos int64) ([]byte, error) {
	return file.lob.getDataWithOffsetSize(pos, 0)
}
func (file *BFile) ReadBytesFromPos(pos, count int64) ([]byte, error) {
	return file.lob.getDataWithOffsetSize(pos, count)
}

func (file *BFile) Scan(value interface{}) error {
	if value == nil {
		file.Valid = false
		file.fileName = ""
		file.dirName = ""
		file.lob.sourceLocator = nil
		file.lob.sourceLen = 0
		return nil
	}
	switch temp := value.(type) {
	case *BFile:
		file = temp
	case BFile:
		*file = temp
	default:
		return errors.New("BFILE column type require BFile value")
	}
	return nil
}
