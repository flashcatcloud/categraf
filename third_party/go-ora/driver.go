package go_ora

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/sijms/go-ora/v2/converters"
)

type OracleDriver struct {
	dataCollected bool
	cusTyp        map[string]customType
	sessionParam  map[string]string
	mu            sync.Mutex
	sStrConv      converters.IStringConverter
	nStrConv      converters.IStringConverter
	//serverCharset  int
	//serverNCharset int
	//Conn      *Connection
	//Server    string
	//Port      int
	//Instance  string
	//Service   string
	//DBName    string
	UserId string
	//SessionId int
	//SerialNum int
}

var oracleDriver = &OracleDriver{
	cusTyp:       map[string]customType{},
	sessionParam: map[string]string{},
}

type GetDriverInterface interface {
	Driver() driver.Driver
}

func init() {
	sql.Register("oracle", oracleDriver)
}

func GetDefaultDriver() *OracleDriver {
	return oracleDriver
}

func NewDriver() *OracleDriver {
	return &OracleDriver{
		cusTyp:       map[string]customType{},
		sessionParam: map[string]string{},
	}
}

func (driver *OracleDriver) init(conn *Connection) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if !driver.dataCollected {
		driver.UserId = conn.connOption.UserID
		if driver.sStrConv == nil {
			driver.sStrConv = conn.sStrConv.Clone()
		}
		if driver.nStrConv == nil {
			driver.nStrConv = conn.nStrConv.Clone()
		}
		driver.dataCollected = true
	}
	// update session parameters
	var err error
	for key, value := range driver.sessionParam {
		_, err = conn.Exec(fmt.Sprintf("alter session set %s='%s'", key, value))
		if err != nil {
			return err
		}
	}
	return err
}

// SetStringConverter this function is used to set a custom string converter interface
// that will be used to encode and decode strings and bytearrays
// passing nil will use driver string converter for supported langs
func SetStringConverter(db GetDriverInterface, charset, nCharset converters.IStringConverter) {
	if driver, ok := db.Driver().(*OracleDriver); ok {
		driver.sStrConv = charset
		driver.nStrConv = nCharset
	}
}
func DelSessionParam(db *sql.DB, key string) {
	if drv, ok := db.Driver().(*OracleDriver); ok {
		drv.mu.Lock()
		defer drv.mu.Unlock()
		delete(drv.sessionParam, key)
	}
}
func AddSessionParam(db *sql.DB, key, value string) error {
	_, err := db.Exec(fmt.Sprintf("alter session set %s='%s'", key, value))
	if err != nil {
		return err
	}
	if drv, ok := db.Driver().(*OracleDriver); ok {
		drv.mu.Lock()
		defer drv.mu.Unlock()
		drv.sessionParam[key] = value
	}
	return nil
}

func RegisterType(conn *sql.DB, typeName, arrayTypeName string, typeObj interface{}) error {
	// ping first to avoid error when calling register type after open connection
	err := conn.Ping()
	if err != nil {
		return err
	}

	if driver, ok := conn.Driver().(*OracleDriver); ok {
		return RegisterTypeWithOwner(conn, driver.UserId, typeName, arrayTypeName, typeObj)
	}
	return errors.New("the driver used is not a go-ora driver type")
}

func RegisterTypeWithOwner(conn *sql.DB, owner, typeName, arrayTypeName string, typeObj interface{}) error {
	if len(owner) == 0 {
		return errors.New("owner can't be empty")
	}
	if driver, ok := conn.Driver().(*OracleDriver); ok {

		if typeObj == nil {
			return errors.New("type object cannot be nil")
		}
		typ := reflect.TypeOf(typeObj)
		switch typ.Kind() {
		case reflect.Ptr:
			return errors.New("unsupported type object: Ptr")
		case reflect.Array:
			return errors.New("unsupported type object: Array")
		case reflect.Chan:
			return errors.New("unsupported type object: Chan")
		case reflect.Map:
			return errors.New("unsupported type object: Map")
		case reflect.Slice:
			return errors.New("unsupported type object: Slice")
		}
		if typ.Kind() != reflect.Struct {
			return errors.New("type object should be of structure type")
		}
		cust := customType{
			owner:         owner,
			name:          typeName,
			arrayTypeName: arrayTypeName,
			typ:           typ,
			fieldMap:      map[string]int{},
		}
		sqlText := `SELECT type_oid FROM ALL_TYPES WHERE UPPER(OWNER)=:1 AND UPPER(TYPE_NAME)=:2`
		err := conn.QueryRow(sqlText, strings.ToUpper(owner), strings.ToUpper(typeName)).Scan(&cust.toid)
		if err != nil {
			return err
		}
		if len(cust.arrayTypeName) > 0 {
			err = conn.QueryRow(sqlText, strings.ToUpper(owner), strings.ToUpper(arrayTypeName)).Scan(&cust.arrayTOID)
			if err != nil {
				return err
			}
		}
		sqlText = `SELECT ATTR_NAME, ATTR_TYPE_NAME, LENGTH, ATTR_NO 
					FROM ALL_TYPE_ATTRS 
					WHERE UPPER(OWNER)=:1 AND UPPER(TYPE_NAME)=:2
					ORDER BY ATTR_NO`
		rows, err := conn.Query(sqlText, strings.ToUpper(owner), strings.ToUpper(typeName))
		if err != nil {
			return err
		}
		var (
			attName     sql.NullString
			attOrder    int64
			attTypeName sql.NullString
			length      sql.NullInt64
		)
		for rows.Next() {
			err = rows.Scan(&attName, &attTypeName, &length, &attOrder)
			if err != nil {
				return err
			}
			//for int(attOrder) > len(cust.attribs) {
			//	cust.attribs = append(cust.attribs, ParameterInfo{
			//		Direction: Input,
			//		Flag:      3,
			//	})
			//}
			//param := &cust.attribs[attOrder-1]
			param := ParameterInfo{Direction: Input, Flag: 3}
			param.Name = attName.String
			param.TypeName = attTypeName.String
			switch strings.ToUpper(attTypeName.String) {
			case "NUMBER":
				param.DataType = NUMBER
				param.MaxLen = converters.MAX_LEN_NUMBER
			case "VARCHAR2":
				param.DataType = NCHAR
				param.CharsetForm = 1
				param.ContFlag = 16
				param.MaxCharLen = int(length.Int64)
				param.CharsetID = driver.sStrConv.GetLangID()
				param.MaxLen = int(length.Int64) * converters.MaxBytePerChar(param.CharsetID)
			case "NVARCHAR2":
				param.DataType = NCHAR
				param.CharsetForm = 2
				param.ContFlag = 16
				param.MaxCharLen = int(length.Int64)
				param.CharsetID = driver.nStrConv.GetLangID()
				param.MaxLen = int(length.Int64) * converters.MaxBytePerChar(param.CharsetID)
			case "TIMESTAMP":
				fallthrough
			case "DATE":
				param.DataType = DATE
				param.MaxLen = 11
			case "RAW":
				param.DataType = RAW
				param.MaxLen = int(length.Int64)
			case "BLOB":
				param.DataType = OCIBlobLocator
				param.MaxLen = int(length.Int64)
			case "CLOB":
				param.DataType = OCIClobLocator
				param.CharsetForm = 1
				param.ContFlag = 16
				param.CharsetID = driver.sStrConv.GetLangID()
				param.MaxCharLen = int(length.Int64)
				param.MaxLen = int(length.Int64) * converters.MaxBytePerChar(param.CharsetID)
			case "NCLOB":
				param.DataType = OCIClobLocator
				param.CharsetForm = 2
				param.ContFlag = 16
				param.CharsetID = driver.nStrConv.GetLangID()
				param.MaxCharLen = int(length.Int64)
				param.MaxLen = int(length.Int64) * converters.MaxBytePerChar(param.CharsetID)
			default:
				found := false
				for name, value := range driver.cusTyp {
					if strings.EqualFold(name, attTypeName.String) {
						found = true
						//param.DataType = XMLType
						param.cusType = new(customType)
						*param.cusType = value
						param.cusType.isArray = false
						param.ToID = value.toid
						break
					}
					if strings.EqualFold(value.arrayTypeName, attTypeName.String) {
						found = true
						param.cusType = new(customType)
						param.DataType = XMLType
						*param.cusType = value
						param.cusType.isArray = true
						param.ToID = value.arrayTOID
						break
					}
				}
				if !found {
					return fmt.Errorf("unsupported attribute type: %s", attTypeName.String)
				}
			}
			cust.attribs = append(cust.attribs, param)
		}
		if len(cust.attribs) == 0 {
			return fmt.Errorf("unknown or empty type: %s", typeName)
		}
		cust.loadFieldMap()
		driver.mu.Lock()
		defer driver.mu.Unlock()
		driver.cusTyp[strings.ToUpper(typeName)] = cust
		return nil
	}
	return errors.New("the driver used is not a go-ora driver type")
}
