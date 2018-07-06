package verthazel

import (
	"log"
	"os"
	"strings"

	fsnotify "github.com/fsnotify/fsnotify"
)

type RootMonitor struct {
	rootMon       *RootMonitor
	root          string
	subroots      map[string]bool
	watcher       *fsnotify.Watcher
	watchedEvents map[string]bool
	resourcesMap  *IOResourcesMap
}

func (rootMon *RootMonitor) InvokeWatcher(resourcesMap *IOResourcesMap) (err error) {
	if err = rootMon.initWatcher(); err == nil {
		if err = rootMon.AddPathToMonitor(rootMon.root); err == nil {
			monitorFSNotifyWatcher(rootMon, rootMon.watcher)
			if rootMon.resourcesMap == nil {
				rootMon.resourcesMap = resourcesMap
				//resourcesMap.rootMon = rootMon
			}
		}

	}
	return err
}

func (rootMon *RootMonitor) initWatchedEvents() {
	if rootMon.watchedEvents == nil {
		rootMon.watchedEvents = make(map[string]bool)
	}
}

func (rootMon *RootMonitor) initSubRoots() {
	if rootMon.subroots == nil {
		rootMon.subroots = make(map[string]bool)
	}
}

func (rootMon *RootMonitor) MonitorEvent(root string, name string, event fsnotify.Event, op fsnotify.Op) {
	if op == fsnotify.Create || op == fsnotify.Write {
		fi, fierr := os.Stat(root + name)
		if fierr == nil {
			if op == fsnotify.Create {
				if !fi.IsDir() {
					if rootMon.resourcesMap != nil {
						rootMon.resourcesMap.RegisterFileResource(root, name)
					}
				}
				rootMon.CreatedMonitoredPath(root, name, fi.IsDir())
				if fi.IsDir() {
					rootMon.AddPathToMonitor(root + name)
				}
			} else if op == fsnotify.Write {
				rootMon.WroteMonitoredPath(root, name, fi.IsDir())
			}
		}
	} else if op == fsnotify.Remove {
		if rootMon.subroots != nil {
			_, containsPath := rootMon.subroots[root+name]
			if containsPath {
				if rootMon.subroots[root+name] {
					rootMon.subroots[root+name] = false
				} else {
					delete(rootMon.subroots, root+name)
					rootMon.RemovedMonitoredPath(root, name, containsPath)
				}
			} else {
				if rootMon.resourcesMap != nil {
					rootMon.resourcesMap.RemoveFileResource(root, name)
				}
				rootMon.RemovedMonitoredPath(root, name, containsPath)
			}
		} else {
			if rootMon.resourcesMap != nil {
				rootMon.resourcesMap.RemoveFileResource(root, name)
			}
			rootMon.RemovedMonitoredPath(root, name, false)
		}
	}
	//log.Println(op, "->", name)
}

func (rootMon *RootMonitor) CreatedMonitoredPath(root string, path string, dir bool) {
	log.Println("CreatedMonitoredPath(", root, ","+path+",", dir, ")")

}

func (rootMon *RootMonitor) RemovedMonitoredPath(root string, path string, dir bool) {
	log.Println("RemovedMonitoredPath(", root, ","+path+",", dir, ")")
}

func (rootMon *RootMonitor) WroteMonitoredPath(root string, path string, dir bool) {
	log.Println("WroteMonitoredPath(", root, ","+path+",", dir, ")")
}

func monitorFSNotifyWatcher(rootMon *RootMonitor, watcher *fsnotify.Watcher) {
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				op := fsnotify.Op(0)
				if event.Op&fsnotify.Write == fsnotify.Write {
					op = fsnotify.Write
				} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
					op = fsnotify.Remove
				} else if event.Op&fsnotify.Create == fsnotify.Create {
					op = fsnotify.Create
				} else if event.Op&fsnotify.Chmod == fsnotify.Chmod {
					op = fsnotify.Chmod
				}
				rootMon.MonitorEvent(rootMon.root, strings.Replace(event.Name, "\\", "/", -1), event, op)
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()
}

func (rootMon *RootMonitor) cleanupRootMonitor() {

}

func (rootMon *RootMonitor) FSNotifyWatcher() *fsnotify.Watcher {
	err := rootMon.initWatcher()
	if err == nil {
		return rootMon.watcher
	}
	return nil
}

func (rootMon *RootMonitor) initWatcher() (err error) {
	if rootMon.watcher == nil {
		rootMon.watcher, err = fsnotify.NewWatcher()
	}
	return err
}

func (rootMon *RootMonitor) AddPathToMonitor(path string) (err error) {
	if watcher := rootMon.FSNotifyWatcher(); watcher != nil {
		err = watcher.Add(path)
		if err == nil {
			if path != rootMon.root {
				rootMon.initSubRoots()
				if _, containsPath := rootMon.subroots[path]; !containsPath {
					rootMon.subroots[path] = true
				}
			}
		}
	}
	return err
}

func (rootMon *RootMonitor) RemovePathToMonitor(path string) (err error) {
	if rootMon.watcher != nil {
		err = rootMon.watcher.Remove(path)
		log.Println(err)
	}
	return err
}

var rootMonitor *RootMonitor

func SystemMonitor() *RootMonitor {
	if rootMonitor == nil {
		rootMonitor = &RootMonitor{root: "./"}
	}
	return rootMonitor
}
