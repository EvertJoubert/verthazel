package verthazel

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	interupted bool
	connector  chan *Connector
}

type IConnector interface {
	PerformConnector(*Client) (bool, error)
}

type Connector struct {
	reqType     RequestType
	params      []*RequestParameter
	reqHeaders  map[string]string
	respHeaders map[string]string
	method      string
	path        string
	reqIORW     *IORW
	respIORW    *IORW
	done        chan bool
}

func (con *Connector) PerformConnector(client *Client) (done bool, err error) {
	if con.reqType == WebRequest {
		cl := &http.Client{
			Timeout: time.Second * 10,
		}

		reqio := con.reqIORW
		if reqio == nil {
			reqio = EmptyIO()
		} else {
			fmt.Println(reqio)
		}

		if req, err := http.NewRequest("POST", con.path, reqio); err == nil {
			for key := range con.reqHeaders {
				req.Header.Set(key, con.reqHeaders[key])
			}

			if resp, resperr := cl.Do(req); resperr == nil {
				if con.respIORW != nil {
					con.respIORW.InputReader(resp.Body)
				}
				done = true
			} else {
				done = true
				err = resperr
			}
		}
	}
	return done, err
}

func (con *Connector) cleanupConnector() {
	if con.done != nil {
		close(con.done)
		con.done = nil
	}
	con.cleanupRequestParameters()
}

func (con *Connector) cleanupRequestParameters() {
	if con.params != nil {
		for len(con.params) > 0 {
			if param := con.params[0]; param != nil && len(param.value) > 0 {
				for len(param.value) > 0 {
					param.value[0] = nil
					if len(param.value) > 1 {
						param.value = param.value[1:]
					} else {
						break
					}
				}
				param.value = nil
			}
			con.params[0] = nil
			if len(con.params) > -1 {
				con.params = con.params[1:]
			} else {
				break
			}
		}
		con.params = nil
	}
}

func NewClient() (client *Client) {
	client = &Client{connector: make(chan *Connector)}
	go client.dequeueConnectors()
	return client
}

func (client *Client) Interupt() {
	if !client.interupted {
		client.interupted = true
	}
}

func (client *Client) dequeueConnectors() {
	for {
		select {
		case con := <-client.connector:
			go func() {
				if done, err := IConnector(con).PerformConnector(client); err == nil {
					if done {
						con.cleanupConnector()
					} else {
						client.connector <- con
					}
					con = nil
				}
			}()
			break
		}
	}
}

var defClient *Client

func DefaultClient() *Client {
	if defClient == nil {
		defClient = NewClient()
	}
	return defClient
}

type RequestType int

const ConsoleRequest RequestType = 0
const WebRequest = 1

type RequestParameter struct {
	param string
	value []interface{}
}

func Parameter(name string, value ...interface{}) *RequestParameter {
	return &RequestParameter{param: name, value: value}
}

func (client *Client) Request(wait bool, reqType RequestType, method string, reqIORW *IORW, path string, reqHeaders map[string]string, params ...*RequestParameter) (respIORW *IORW) {
	con := &Connector{reqType: reqType, method: method, path: path, reqHeaders: reqHeaders, params: params}

	if reqIORW != nil {
		con.reqIORW = reqIORW
		if strings.ToUpper(con.method) != "POST" {
			con.method = "POST"
		}
	}
	if wait {
		con.done = make(chan bool, 1)
		respIORW = &IORW{}
		con.respIORW = respIORW
	}
	client.connector <- con
	if wait {
		<-con.done
		con.cleanupConnector()
		if reqIORW != nil {
			reqIORW.CleanupIORW()
		}
	}
	con = nil
	reqIORW = nil
	return respIORW
}
