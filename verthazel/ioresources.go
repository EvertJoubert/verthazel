package verthazel

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type IOResourceType int

const FileResourceType IOResourceType = 0
const EmbeddedResourceType IOResourceType = 1
const WebResourceType IOResourceType = 2

type IOResource struct {
	IORW
	path         string
	root         string
	modified     time.Time
	resourceType IOResourceType
	resourceMap  *IOResourcesMap
	rlock        *sync.Mutex
	locked       bool
}

func (ioRes *IOResource) Lock() {
	ioRes.locked = true
	ioRes.initLock()
	ioRes.rlock.Lock()
}

func (ioRes *IOResource) IsValid() (valid bool) {
	if ioRes.resourceType == FileResourceType {
		fi, fierr := os.Stat(ioRes.root + ioRes.path)
		if fierr == nil {
			valid = fi.ModTime() == ioRes.modified
		}
	} else {
		valid = true
	}
	return valid
}

func (ioRes *IOResource) Unlock() {
	if ioRes.rlock != nil {
		ioRes.rlock.Unlock()
	}
}

func (ioRes *IOResource) Locked() bool {
	return ioRes.locked
}

func (ioRes *IOResource) reloadResource() (err error) {
	if ioRes.resourceType == FileResourceType {
		ioRes.Lock()
		defer ioRes.Unlock()
		f, ferr := os.Open(ioRes.root + ioRes.path)
		defer f.Close()
		if ferr == nil {
			ioRes.IORW.CleanupIORW()
			if err = ioRes.InputFile(f); err != nil && err == io.EOF {
				err = nil
			}
			if err == nil {
				fi, _ := os.Stat(ioRes.root + ioRes.path)
				ioRes.modified = fi.ModTime()
			}
		} else {
			err = ferr
		}
	}
	return err
}

func (ioRes *IOResource) initLock() {
	if ioRes.rlock == nil {
		ioRes.rlock = &sync.Mutex{}
	}
}

func (ioRes *IOResource) cleanupIOResource() {
	ioRes.IORW.CleanupIORW()
	if ioRes.resourceMap != nil {
		ioRes.resourceMap.resources[ioRes.path] = nil
		delete(ioRes.resourceMap.resources, ioRes.path)
		ioRes.resourceMap = nil
	}
	if ioRes.rlock != nil {
		ioRes.rlock = nil
	}
}

func (ioRes *IOResource) ReadToWriter(w io.Writer) {
	ioRes.IORW.ReadToWriter(w)
}

func newIOResource(ioResMap *IOResourcesMap, root string, path string, ioResourceType IOResourceType) (ioResource *IOResource) {
	ioResource = &IOResource{root: root, path: path, resourceMap: ioResMap, modified: time.Now(), resourceType: ioResourceType}
	return ioResource
}

func (ioRes *IOResource) ResourceType() IOResourceType {
	return ioRes.resourceType
}

type IOResourcesMap struct {
	resources map[string]*IOResource
	//rootMon   *RootMonitor
	rmlock *sync.Mutex
	root   string
}

func (ioResMap *IOResourcesMap) Root() string {
	if ioResMap.root == "" {
		return "./"
	} else {
		return ioResMap.root
	}
}

func (ioResMap *IOResourcesMap) SetRoot(r string) {
	if r != "" {
		r = strings.Replace(r, "\\", "/", -1)
		if !strings.HasSuffix(r, "/") {
			r = r + "/"
		}
		ioResMap.root = r
	}
}

func (ioResMap *IOResourcesMap) RegisterEmbeddedResource(path string, r io.Reader) {
	ioResMap.initResources()
	if ioRes := ioResMap.CheckResource(path); ioRes == nil {
		ioResMap.resources[path] = newIOResource(ioResMap, "", path, EmbeddedResourceType)
		ioRes = ioResMap.resources[path]
		ioRes.Lock()
		defer ioRes.Unlock()
		ioRes.InputReader(r)
		ioRes.modified = time.Now()
	} else {
		ioResMap.Lock()
		defer ioResMap.Unlock()
		ioRes = ioResMap.resources[path]
		ioRes.cleanupIOResource()
		ioRes.modified = time.Now()
		ioRes.path = path
		ioRes.root = ""
		ioRes.resourceMap = ioResMap
		ioRes.resourceType = EmbeddedResourceType
		ioResMap.resources[path] = ioRes
		ioRes.initLock()
		ioRes.rlock.Lock()
		defer ioRes.rlock.Unlock()
		ioRes.InputReader(r)
	}
}

func (ioResMap *IOResourcesMap) ReloadFileResource(root string, path string) (err error) {
	if ioResMap.resources == nil {
		err = ioResMap.RegisterFileResource(root, path)
	} else if resRep, containsResource := ioResMap.resources[path]; containsResource {
		if ioResMap.resources[path].root == root {
			ioResMap.resources[path].reloadResource()
		}
	} else if resRep == nil {
		err = ioResMap.RegisterFileResource(root, path)
	}
	return err
}

func (ioResMap *IOResourcesMap) initLocking() {
	if ioResMap.rmlock == nil {
		ioResMap.rmlock = &sync.Mutex{}
	}
}

func (ioResMap *IOResourcesMap) Lock() {
	ioResMap.initLocking()
	ioResMap.rmlock.Lock()
}

func (ioResMap *IOResourcesMap) Unlock() {
	if ioResMap.rmlock != nil {
		ioResMap.rmlock.Unlock()
	}
}

func (ioResMap *IOResourcesMap) RemoveFileResource(root string, path string) (err error) {
	if _, containsResource := ioResMap.resources[path]; containsResource {
		ioResMap.Lock()
		defer ioResMap.Unlock()
		ioResMap.resources[path].cleanupIOResource()
	}
	return err
}

func (ioResMap *IOResourcesMap) RegisterFileResource(root string, path string) (err error) {
	ioResMap.initResources()
	finfo, finfoerr := os.Stat(root + path)
	if finfoerr == nil {
		if !finfo.IsDir() {
			if ioRes := ioResMap.CheckResource(path); ioRes == nil {
				ioRes = newIOResource(ioResMap, root, path, FileResourceType)
				ioRes.initLock()
				ioResMap.resources[path] = ioRes
			}
			err = ioResMap.resources[path].reloadResource()
		}
	} else {
		err = finfoerr
	}
	return err
}

func (ioResMap *IOResourcesMap) CheckResource(path string) (iores *IOResource) {
	if ioResMap.resources != nil {
		if ioRes, containsIoRes := ioResMap.resources[path]; containsIoRes {
			iores = ioRes
		}
	}
	return iores
}

func (ioResMap *IOResourcesMap) Resource(path string) (iores *IOResource) {
	if ioResMap.resources != nil {
		if ioRes, containsIoRes := ioResMap.resources[path]; containsIoRes {
			iores = ioRes
			if iores.resourceType == FileResourceType {
				fi, fierr := os.Stat(ioResMap.Root() + path)
				if fierr != nil {
					ioResMap.RemoveFileResource(ioResMap.Root(), path)
				} else {
					diff := fi.ModTime().Sub(iores.modified)
					if diff > 0 {
						iores.reloadResource()
					}
				}
			}
		} else {
			if fi, fierr := os.Stat(ioResMap.Root() + path); fierr == nil && !fi.IsDir() {
				if fierr = ioResMap.RegisterFileResource(ioResMap.Root(), path); fierr == nil {
					iores = ioResMap.resources[path]
				}
			}

		}
	}
	return iores
}

func (ioResMap *IOResourcesMap) initResources() {
	if ioResMap.resources == nil {
		ioResMap.resources = make(map[string]*IOResource)
	}
}

var mappedIOResources *IOResourcesMap

func MappedResources() *IOResourcesMap {
	if mappedIOResources == nil {
		mappedIOResources = &IOResourcesMap{}
	}
	return mappedIOResources
}
