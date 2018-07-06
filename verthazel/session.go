package verthazel

import (
	"mime/multipart"
	"net/http"
	"net/url"
)

type ISession interface {
	Parameter(string) []string
	RequestHeader(string) string
	DataConnection(string) *Connection
}

type Session struct {
	reqHeaders    *http.Header
	respHeaders   *http.Header
	multipartForm *multipart.Form
	postForm      *url.Values
	form          *url.Values
}

func NewSession(reqHeaders *http.Header, respheader *http.Header) (session *Session) {
	session = &Session{reqHeaders: reqHeaders, respHeaders: respheader}

	return session
}

func (ses *Session) DataConnection(alias string) *Connection {
	return ActiveConnections().Connection(alias)
}

func (ses *Session) Parameter(param string) (values []string) {
	if ses.multipartForm != nil {
		for paramName, paramValue := range ses.multipartForm.Value {
			if paramName == param {
				if values == nil {
					values = []string{}
				}
				values = append(values, paramValue...)
			}
		}
	} else if ses.postForm != nil {
		for paramName, paramValue := range *ses.postForm {
			if paramName == param {
				if values == nil {
					values = []string{}
				}
				values = append(values, paramValue...)
			}
		}
	}
	if ses.form != nil {
		for paramName, paramValue := range *ses.form {
			if paramName == param {
				if values == nil {
					values = []string{}
				}
				values = append(values, paramValue...)
			}
		}
	}
	if values == nil {
		return emptyParamVal
	}
	return values
}

func (ses *Session) clearSession() {
	if ses.reqHeaders != nil {
		ses.reqHeaders = nil
	}
	if ses.respHeaders != nil {
		ses.respHeaders = nil
	}
	if ses.multipartForm != nil {
		ses.multipartForm = nil
	}
	if ses.postForm != nil {
		ses.postForm = nil
	}
	if ses.form != nil {
		ses.form = nil
	}
}

func (ses *Session) RequestHeader(header string) string {
	if ses.reqHeaders != nil {
		ses.reqHeaders.Get(header)
	}
	return ""
}

func (ses *Session) ResponseHeader(header string) string {
	if ses.respHeaders != nil {
		ses.respHeaders.Get(header)
	}
	return ""
}

var emptyParamVal []string

func init() {
	if emptyParamVal == nil {
		emptyParamVal = []string{}
	}
}
