package verthazel

import (
	/*"fmt"
	"net/url"
	"strings"
	"time"*/

	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

type Connections struct {
	conns map[string]*Connection
}

type Connection struct {
	name     string
	settings map[string]string
	db       *sql.DB
	cnLock   *sync.Mutex
	active   bool
}

func (con *Connection) User() (user string) {
	return con.Setting("user")
}

func (con *Connection) Password() (password string) {
	return con.Setting("password")
}

func (con *Connection) Driver() (driver string) {
	return con.Setting("driver")
}

func (con *Connection) Host() (host string) {
	return con.Setting("host")
}

func (con *Connection) Setting(setting string) (value string) {
	if con.settings != nil {
		if val, containSetting := con.settings[setting]; containSetting {
			value = val
		}
	}
	return value
}

func (con *Connection) Connect() (dbcn *sql.DB) {
	query := url.Values{}
	//query.Add("connection timeout", fmt.Sprintf("%d", 60))
	hostname := con.Host()
	instancePath := ""
	if strings.Index(hostname, "/") > 0 {
		instancePath = hostname[strings.Index(hostname, "/")+1:]
		hostname = hostname[:strings.Index(hostname, "/")]
	}
	u := &url.URL{
		Scheme:   con.Driver(),
		User:     url.UserPassword(con.User(), con.Password()),
		Host:     hostname,
		Path:     instancePath, // if connecting to an instance instead of a port
		RawQuery: query.Encode(),
	}

	connectionString := u.String()
	db, err := sql.Open(con.Driver(), connectionString)
	if err == nil {
		err = db.Ping()
		if err == nil {
			dbcn = db
		} else {
			fmt.Println(err.Error())
		}
	}
	return dbcn
}

func (con *Connection) SetSetting(setting string, value string) {
	if con.settings == nil {
		con.settings = make(map[string]string)
	}
	con.settings[setting] = value
}

var connections *Connections

func ActiveConnections() *Connections {
	if connections == nil {
		connections = &Connections{}
	}
	return connections
}

func (conns *Connections) RegisterConnection(alias string, settings []string) {
	if conns.conns == nil {
		conns.conns = make(map[string]*Connection)
	}
	if _, containsCn := conns.conns[alias]; !containsCn {
		var cn *Connection = &Connection{cnLock: &sync.Mutex{}}
		if len(settings) > 0 && strings.Contains(settings[0], "|") {
			settings = strings.Split(settings[0], "|")
		}
		for _, setting := range settings {
			if strings.Index(setting, "=") > 0 {
				if setting[0:strings.Index(setting, "=")] != "" && setting[strings.Index(setting, "=")+1:] != "" {
					cn.SetSetting(setting[0:strings.Index(setting, "=")], setting[strings.Index(setting, "=")+1:])
				}
			}
		}
		cn.db = cn.Connect()
		if cn.db == nil {
			cn.active = false
		} else {
			cn.active = true
		}
		conns.conns[alias] = cn
	}
}

var blankConnection Connection

func (conns *Connections) Connection(alias string) (con *Connection) {
	if conns.conns != nil {
		if cn, containsCn := conns.conns[alias]; containsCn {
			con = cn
		} else {
			con = &blankConnection
		}
	}
	return con
}

func (con *Connection) ReConnect() {
	if con.db == nil {
		con.db = con.Connect()
		con.active = con.db != nil
	}
}

var statementTypes []string = []string{"select", "insert", "update", "delete", "procedure", "function"}

func (con *Connection) ExecuteSQL(sqlstatement string, params ...interface{}) (resultSet *ResultSet) {
	return con.ExecuteSQLType(sqlstatement, "", "", params)
}

func (con *Connection) ExecuteSQLType(sqlstatement string, statementType string, defaultType string, params ...interface{}) (resultSet *ResultSet) {
	if con.active {
		dberrp := con.db.Ping()
		con.active = dberrp == nil
		if !con.active {
			con.db = nil
		}
	} else {
		con.ReConnect()
	}

	if con.active {
		if statementType == "" {
			if strings.Index(sqlstatement, " ") > 0 && sqlstatement[0:strings.Index(sqlstatement, " ")] != "" {

				statementpe := strings.ToLower(sqlstatement[0:strings.Index(sqlstatement, " ")])

				for n, _ := range statementTypes {
					if statementpe == statementTypes[n] {
						statementType = statementpe
						break
					}
				}

				if statementType == "" {
					if defaultType == "" {
						statementType = statementTypes[0]
					} else {
						statementType = defaultType
					}
				}

			} else {
				if defaultType == "" {
					defaultType = "select"
				}

				for n, _ := range statementTypes {
					if defaultType == statementTypes[n] {
						statementType = defaultType
						break
					}
				}

				if statementType == "" {
					statementType = "select"
				}

			}
		} else {
			for n, _ := range statementTypes {
				if statementType == statementTypes[n] {
					break
				}
				if n == len(statementTypes)-1 {
					statementType = "select"
				}
			}
		}

		sqlPrepStmnt, errSqlPrep := con.db.Prepare(sqlstatement)
		if errSqlPrep == nil {
			if statementType == "select" {
				resultSet = NewResultSet(sqlPrepStmnt)
			} else {
				_, execErr := sqlPrepStmnt.Exec(params...)
				if execErr == nil {

				}
				execClose := sqlPrepStmnt.Close()
				if execClose == nil {

				}
				sqlPrepStmnt = nil
			}
		} else {
			resultSet = &ResultSet{err: errSqlPrep.Error()}
		}
	} else {
		resultSet = &blankResultset
	}
	return resultSet
}

var blankResultset ResultSet

func NewResultSet(sqlStmt *sql.Stmt, params ...interface{}) (rset *ResultSet) {
	rows, errSqlRows := sqlStmt.Query(params...)
	if errSqlRows == nil {
		var resultSet ResultSet = ResultSet{sqlStmt: sqlStmt, sqlRows: rows}
		rset = &resultSet

		if rset.columns == nil {
			if cols, errCols := rset.sqlRows.Columns(); errCols == nil {
				if colTypes, errColTypes := rset.sqlRows.ColumnTypes(); errColTypes == nil {
					loadCols := make(chan bool, 1)
					go func() {
						rset.rawData = make([]interface{}, len(cols))
						rset.rawDataRefs = make([]interface{}, len(cols))
						for coln, col := range cols {
							column := NewColumn(rset, col, colTypes[coln], coln)

							if rset.columns == nil {
								rset.columns = make([]*Column, len(cols))
							}
							rset.columns[coln] = column
							rset.rawDataRefs[coln] = &rset.rawData[coln]
						}
						loadCols <- true
					}()
					<-loadCols
					close(loadCols)
				}
			}
		}
		rset.active = true
	} else {
		var resultSet ResultSet = ResultSet{err: errSqlRows.Error()}
		rset = &resultSet
	}

	return rset
}

type ResultSet struct {
	sqlRows     *sql.Rows
	sqlStmt     *sql.Stmt
	columns     []*Column
	rawData     []interface{}
	rawDataRefs []interface{}
	active      bool
	err         string
}

func (rset *ResultSet) Error() string {
	return rset.err
}

func (rset *ResultSet) Cleanup() {
	if rset.sqlRows != nil {
		if rset.columns != nil {
			for _, col := range rset.columns {
				col.CleanupColumn()
			}
			rset.columns = nil
		}
		if rset.rawData != nil {
			for n, _ := range rset.rawData {
				rset.rawData[n] = nil
			}
			rset.rawData = nil
		}
		if rset.rawDataRefs != nil {
			for n, _ := range rset.rawDataRefs {
				rset.rawDataRefs[n] = nil
			}
			rset.rawDataRefs = nil
		}

		sqlRowsErr := rset.sqlRows.Err()
		if sqlRowsErr != nil {
			fmt.Println(sqlRowsErr.Error())
		}
		rset.sqlRows = nil
	}

	if rset.sqlStmt != nil {
		stmtErr := rset.sqlStmt.Close()
		if stmtErr != nil {
			fmt.Println(stmtErr.Error())
		}
		rset.sqlStmt = nil
	}
}

func (rset *ResultSet) Columns() []*Column {
	return rset.columns
}

func (rset *ResultSet) EOF() (eof bool) {
	eof = (rset.active && rset.sqlRows == nil)
	return eof
}

func (rset *ResultSet) Next() (next bool) {
	if rset.active && rset.sqlRows != nil {
		if rset.sqlRows.Next() {
			for n, _ := range rset.rawData {
				rset.rawData[n] = nil
			}
			errScan := rset.sqlRows.Scan(rset.rawDataRefs...)
			next = errScan == nil
			if !next {
				rset.Close()
			}
		} else {
			rset.Close()
		}
	}
	return next
}

func (rset *ResultSet) Data() (data []interface{}) {
	if rset.active && rset.sqlRows != nil {
		data = rset.rawData
	}
	return data
}

func (rset *ResultSet) Strings() (data []string) {
	if rset.active && rset.sqlRows != nil {
		data = make([]string, len(rset.rawData))
		for di, col := range rset.Columns() {
			data[di] = col.String()
		}
	}
	return data
}

func (rset *ResultSet) Close() {
	if rset.sqlRows != nil {
		if err := rset.sqlRows.Close(); err == nil {
		}
		rset.Cleanup()
	}
}

func (rset *ResultSet) Open() bool {
	return rset.active && rset.sqlRows != nil
}

type Column struct {
	rset       *ResultSet
	col        string
	colType    *sql.ColumnType
	colenabled bool
	colindex   int
}

func (col *Column) Delete() {
	col.colenabled = false
}

func (col *Column) CleanupColumn() {
	if col.rset != nil {
		col.rset = nil
	}
	if col.colType != nil {
		col.colType = nil
	}
}

func (col *Column) Name() string {
	return col.col
}

func (col *Column) String() (colstring string) {
	colstring = ""
	if col.rset.rawData != nil && col.colindex < len(col.rset.rawData) {
		if col.colType != nil && col.rset.rawData[col.colindex] != nil {
			if col.colType.ScanType().Name() == "string" || strings.Contains(strings.ToLower(col.colType.DatabaseTypeName()), "char") {
				colstring = col.rset.rawData[col.colindex].(string)
			} else if col.colType.ScanType().Name() == "time" {
				colstring = col.rset.rawData[col.colindex].(time.Time).Format("2006-01-02 15:04:05")
			} else {
				if uintdata, ok := col.rset.rawData[col.colindex].([]uint8); ok {
					colstring = string(uintdata)
				} else if timedata, ok := col.rset.rawData[col.colindex].(time.Time); ok {
					colstring = timedata.Format("2006-01-02 15:04:05")
				} else {
					colstring = col.rset.rawData[col.colindex].(string)
				}
			}
		}
	}
	return colstring
}

func NewColumn(rest *ResultSet, col string, colType *sql.ColumnType, coli int) *Column {
	var column = Column{rset: rest, col: col, colType: colType, colindex: coli}
	return &column
}
