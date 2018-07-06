package verthazel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kardianos/osext"
)

type ServletHandle func(*Servlet, string, string, string, *NotifiableRequestResponse, *http.Request)

type Servlet struct {
	paths         map[string]*ServletHandle
	defaultHandle *ServletHandle
	rootpath      string
	//lock          *sync.Mutex
	resourcesMap *IOResourcesMap
}

func (slet *Servlet) Resources() *IOResourcesMap {
	if slet.resourcesMap == nil {
		return MappedResources()
	} else {
		return slet.resourcesMap
	}
}

func (slet *Servlet) hasServletHandle(handlePath string) (hasSletHandle bool, sletHandle ServletHandle) {
	if slet.paths != nil {
		if sltHndle, hasSltHndle := slet.paths[handlePath]; hasSltHndle {
			hasSletHandle = hasSltHndle
			sletHandle = *sltHndle
		}
	}
	return hasSletHandle, sletHandle
}

type Servlets map[string]*Servlet

func (slets *Servlets) hasServlet(servletpath string) (hasSlet bool, servlet *Servlet) {

	servlet, hasSlet = (*slets)[servletpath]

	return hasSlet, servlet
}

type NotifiableRequestResponse struct {
	ActiveIORW
	httpw   http.ResponseWriter
	httpr   *http.Request
	servlet *Servlet
	wch     <-chan bool
	rch     <-chan struct{}
}

func (ntfyReqResp *NotifiableRequestResponse) cleanupNotifiableRequestResponse() {
	ntfyReqResp.ActiveIORW.cleanupActiveIORW()
	if ntfyReqResp.httpw != nil {
		ntfyReqResp.httpw = nil
	}
	if ntfyReqResp.wch != nil {
		ntfyReqResp.wch = nil
	}
	if ntfyReqResp.httpr != nil {
		ntfyReqResp.httpr = nil
	}
	if ntfyReqResp.rch != nil {
		ntfyReqResp.rch = nil
	}
	if ntfyReqResp.servlet != nil {
		ntfyReqResp.servlet = nil
	}
}

func (ntfyReqResp *NotifiableRequestResponse) Write(p []byte) (n int, err error) {
	n, err = ntfyReqResp.httpw.Write(p)
	if err == nil {
		if ntfyReqResp.wch != nil {
			select {
			case respdone := <-ntfyReqResp.wch:
				if respdone {
					ntfyReqResp.Interupt()
					err = fmt.Errorf("Response Writer Closed")
				}
			default:
			}
		}
		if err == nil && ntfyReqResp.rch != nil {
			select {
			case <-ntfyReqResp.rch:
				if ctxErr := ntfyReqResp.httpr.Context().Err(); ctxErr != nil {
					ntfyReqResp.Interupt()
					err = ctxErr
				} else {
					ntfyReqResp.Interupt()
					err = fmt.Errorf("Request Reader Closed")
				}
			default:
			}
		}
		if ntfyReqResp.interupted {
			err = io.EOF
		}
	}
	return n, err
}

func NewNotifiableRequestResponse(httpw *http.ResponseWriter, httpr *http.Request) *NotifiableRequestResponse {
	notifiableRequestResponse := &NotifiableRequestResponse{httpw: *httpw, httpr: httpr}
	notifiableRequestResponse.altWriteHandle = RWHandle(notifiableRequestResponse.Write)
	if httpw != nil {
		notifiableRequestResponse.SetActive("DeactivatePort", DeactivatePort)
		notifiableRequestResponse.SetActive("ActivatePort", ActivatePort)
		if f, ok := (*httpw).(http.CloseNotifier); ok {
			notifiableRequestResponse.wch = f.CloseNotify()
		}
	}
	if httpr != nil {
		notifiableRequestResponse.rch = httpr.Context().Done()
	}
	return notifiableRequestResponse
}

func (slets *Servlets) handleRequest(w http.ResponseWriter, r *http.Request) {
	ntfyRespW := NewNotifiableRequestResponse(&w, r)
	defer func() {
		ntfyRespW.cleanupNotifiableRequestResponse()
		ntfyRespW = nil
	}()
	spath := r.URL.Path
	dir, file := filepath.Split(spath)
	if dir != "/" && strings.HasSuffix(dir, "/") {
		if hasSlet, servlet := slets.hasServlet(dir); hasSlet {
			ntfyRespW.servlet = servlet
			(*servlet.defaultHandle)(servlet, dir, "", file, ntfyRespW, r)
			return
		}
		subdir := dir[strings.LastIndex(dir, "/"):]
		dir = dir[:len(dir)-1]
		for dir != "/" && strings.LastIndex(dir, "/") > 0 {
			subdir = dir[strings.LastIndex(dir, "/"):] + subdir
			dir = dir[:strings.LastIndex(dir, "/")]
			if hasSlet, servlet := slets.hasServlet(subdir); hasSlet {
				ntfyRespW.servlet = servlet
				if hasSletHandle, sletHandle := servlet.hasServletHandle(subdir); hasSletHandle {
					queueServletRequest(servlet, dir, subdir, file, ntfyRespW, &sletHandle, r)
					return
				} else {
					queueServletRequest(servlet, dir, subdir, file, ntfyRespW, servlet.defaultHandle, r)
					return
				}
			}
		}
		dir += subdir
	}
	ntfyRespW.servlet = defaultServlet
	queueServletRequest(defaultServlet, dir, "", file, ntfyRespW, defaultServlet.defaultHandle, r)
}

type QueuedServletRequest struct {
	servlet    *Servlet
	dir        string
	subdir     string
	file       string
	ntfyRespW  *NotifiableRequestResponse
	sletHandle *ServletHandle
	r          *http.Request
	done       chan bool
}

var queuedServletRequests []chan *QueuedServletRequest
var queuedSletReqsI int32

func queueServletRequest(servlet *Servlet, dir string, subdir string, file string, ntfyRespW *NotifiableRequestResponse, sletHandle *ServletHandle, r *http.Request) {
	sletRequest := &QueuedServletRequest{servlet: servlet, dir: dir, subdir: subdir, file: file, ntfyRespW: ntfyRespW, sletHandle: sletHandle, r: r, done: make(chan bool, 1)}

	go func() {
		(*sletRequest.sletHandle)(sletRequest.servlet, sletRequest.dir, sletRequest.subdir, sletRequest.file, sletRequest.ntfyRespW, sletRequest.r)
		sletRequest.done <- true
	}()

	for {
		select {
		case <-sletRequest.done:
			sletRequest = nil
			return
		}
	}
}

var servlets Servlets

var defaultServlet *Servlet

var mappedServers map[int]*http.Server

func servers() map[int]*http.Server {
	if mappedServers == nil {
		mappedServers = make(map[int]*http.Server)
		http.HandleFunc("/",
			func(w http.ResponseWriter, r *http.Request) {
				done := make(chan bool, 1)
				defer close(done)
				go func() {
					servlets.handleRequest(w, r)
					done <- true
				}()
				<-done
			})
	}
	return mappedServers
}

func isPortActive(port int) bool {
	if mappedServers != nil {
		if _, containsPort := mappedServers[port]; containsPort {
			return true
		}
	}
	return false
}

func portsIsActive() bool {
	if mappedServers != nil && len(mappedServers) > 0 {
		return true
	}
	return false
}

func DeactivatePort(port int) {
	go func() {
		if isPortActive(port) {
			httpserver := mappedServers[port]
			fmt.Printf("Server is shutting down...[port:%d]\r\n", port)
			//logger.Println("Server is shutting down...[port:", port, "]")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			httpserver.SetKeepAlivesEnabled(false)
			if err := httpserver.Shutdown(ctx); err != nil {
				fmt.Printf("Could not gracefully shutdown the server:[port:%d] %v\r\n", port, err)
				//	logger.Fatalf("Could not gracefully shutdown the server:[port:%d] %v\n", port, err)
			}
		}
	}()
}

func ActivatePort(port ...int) {
	if len(port) == 0 {
		return
	}
	for _, p := range port {
		if p <= 0 {
			continue
		}
		if isPortActive(p) {
			return
		}
		httpserver := &http.Server{
			ReadTimeout:       (30 * time.Second),
			WriteTimeout:      (30 * time.Second),
			Addr:              (":" + fmt.Sprint(p)),
			ReadHeaderTimeout: 5 * time.Second,
		}

		if monitoringPorts == nil {
			monitoringPorts = make(chan bool, 1)
		}

		go func() {
			fmt.Println("[port:", p, "]", httpserver.ListenAndServe())
			//log.Fatal(httpserver.ListenAndServe())
			mappedServers[p] = nil
			delete(mappedServers, p)
			if !portsIsActive() {
				if monitoringPorts != nil {
					monitoringPorts <- false
				}
			}
		}()

		servers()[p] = httpserver
	}
}

var monitoringPorts chan bool

func checkActivePorts() bool {
	if portsIsActive() {
		if monitoringPorts != nil {
			for {
				select {
				case mon := <-monitoringPorts:
					if !mon {
						return true
					}
					break
				}
			}
		} else {
			return false
		}
	} else {
		return false
	}
}

func MonitorActiveEnvironment() {
	checkActivePorts()

}

func NewServlet(defaulthHandle ServletHandle, pathHandle ServletHandle, paths ...string) (servlet *Servlet) {
	servlet = &Servlet{defaultHandle: &defaulthHandle}
	servlet.mapPathHandle(pathHandle, paths...)
	return servlet
}

func (servlet *Servlet) mapPathHandle(pathHandle ServletHandle, paths ...string) {
	if servlet.paths == nil {
		servlet.paths = make(map[string]*ServletHandle)
	}
	if len(paths) > 0 {
		for _, path := range paths {
			servlet.paths[path] = &pathHandle
		}
	}
}

func (servlet *Servlet) ServletResource(path string, defaultPage ...string) *IOResource {
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	if path != "" && strings.HasSuffix(path, "/") {
		path = path[0 : len(path)-1]
	}
	if res := servlet.Resources().Resource(path); res != nil {
		return res
	} else if filepath.Ext(path) == "" {
		if path != "" {
			path = path + "/"
		}
		for _, defPage := range defaultPage {
			if res := servlet.Resources().Resource(path + defPage); res != nil {
				return res
			}
		}
	}

	return nil
}

func DefaultServletHandle(servlet *Servlet, dir string, subdir string, file string, reqresp *NotifiableRequestResponse, r *http.Request) {
	iorw := &IORW{}
	iorw.InputReader(r.Body)
	fmt.Println(r.Header)
	fmt.Println()
	fmt.Println(iorw.String())

	resource := servlet.ServletResource(dir+subdir+file, "index.html")
	mimeDetails := FindMimeTypeByExt(dir+subdir+file, ".txt", "text/plain")
	if resource != nil {
		if filepath.Ext(resource.path) != "" && filepath.Ext(dir+subdir+file) != filepath.Ext(resource.path) {
			mimeDetails = FindMimeTypeByExt(resource.path, ".txt", "text/plain")
		}
	}

	reqresp.httpw.Header().Set("CONTENT-TYPE", mimeDetails[0])
	if resource != nil {
		respHeader := reqresp.httpw.Header()
		session := NewSession(&r.Header, &respHeader)
		defer func() {
			session.clearSession()
			session = nil
		}()
		if strings.HasPrefix(strings.TrimSpace(r.Header.Get("CONTENT-TYPE")), "multipart/form-data") {
			if err := r.ParseMultipartForm(4096); err == nil {
				session.multipartForm = r.MultipartForm
			}
		} else if err := r.ParseForm(); err == nil {
			if r.Method == "POST" {
				session.postForm = &r.PostForm
			}
			session.form = &r.Form
		}
		reqresp.executeResource(resource, session)
	}
}

func registerServlet(name string, pathHandle ServletHandle, paths ...string) {
	if servlet, containsServlet := servlets[name]; containsServlet {
		servlet.mapPathHandle(pathHandle, paths...)
	} else {
		servlets[name] = NewServlet(DefaultServletHandle, pathHandle, paths...)
	}
}

func ExecuteGlobalRequest(resourcePath string) {
	atvIORW := &ActiveIORW{}
	atvIORW.path = resourcePath
	atvIORW.SetActive("ActivatePort", ActivatePort)
	atvIORW.SetActive("ActiveConnections", ActiveConnections)
	atvIORW.executeResource(MappedResources().Resource(resourcePath), nil)
	if !atvIORW.Empty() {
		os.Stdout.WriteString(atvIORW.String())
	}
	atvIORW.cleanupActiveIORW()
	atvIORW = nil
}

func init() {
	if servlets == nil {
		servlets = make(Servlets)
	}
	if defaultServlet == nil {
		defaultServlet = NewServlet(DefaultServletHandle, nil)
		localExecDir, err := osext.ExecutableFolder()
		if err == nil {
			defaultServlet.rootpath = strings.Replace(localExecDir, "\\", "/", -1)
		}
	}
}
