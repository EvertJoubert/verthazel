package verthazel

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

func (atvIORW *ActiveIORW) eval(code string) {

	vm := goja.New()

	if atvIORW.activeMap != nil {
		for key := range atvIORW.activeMap {
			vm.Set(key, atvIORW.activeMap[key])
		}
	}

	vm.Set("_script", atvIORW)
	vm.Set("_session", atvIORW.Session)
	_, scriptErr := vm.RunString(code)
	if scriptErr != nil {
		fmt.Println("err:" + scriptErr.Error())
		fmt.Println("code->")
		for sn, sline := range strings.Split(code, "\n") {
			fmt.Println(int64(sn+1), ":", strings.TrimSpace(sline))
		}
		fmt.Println("<-code")
	}
	vm = nil
}

func (atvIORW *ActiveIORW) Session() ISession {
	return atvIORW.session
}

type ActiveIORWResource struct {
	psvContents             []*IORW
	psvContentsI            int
	atvCode                 *IORW
	internalMappedResources map[string]time.Time
	atvIORWTokens           []*ActiveIORWToken
	atvpsvlabels            []string
	lastResourceMap         *IOResourcesMap
	path                    string
	modified                time.Time
}

func (atvIORWRes *ActiveIORWResource) appendInternalResource(resToAppend *IOResource) {
	atvIORWRes.lastResourceMap = resToAppend.resourceMap
	if atvIORWRes.internalMappedResources == nil {
		atvIORWRes.internalMappedResources = make(map[string]time.Time)
	}
	atvIORWRes.internalMappedResources[resToAppend.path] = resToAppend.modified
}

func (atvIORWRes *ActiveIORWResource) valid(ioResMap *IOResourcesMap) (v bool) {
	if checkRes := ioResMap.CheckResource(atvIORWRes.path); checkRes != nil && checkRes.IsValid() {
		if checkRes.modified == atvIORWRes.modified {
			if atvIORWRes.internalMappedResources != nil {
				if len(atvIORWRes.internalMappedResources) > 0 {
					v = true
					for resKey, _ := range atvIORWRes.internalMappedResources {
						if resFound := ioResMap.CheckResource(resKey); resFound == nil {
							v = false
							break
						} else {
							v = resFound.IsValid()
						}
					}
				}
			}
		}
	}
	return v
}

func (atvIORWRes *ActiveIORWResource) cleanupInternalResources() {
	if atvIORWRes.internalMappedResources != nil {
		for internKey, _ := range atvIORWRes.internalMappedResources {
			delete(atvIORWRes.internalMappedResources, internKey)
		}
		atvIORWRes.internalMappedResources = nil
	}
}

func (atvIORWRes *ActiveIORWResource) cleanupActiveIORWResource() {
	if atvIORWRes.atvpsvlabels != nil {
		atvIORWRes.atvpsvlabels = nil
	}
	atvIORWRes.cleanupPassiveContent()
	atvIORWRes.cleanupActiveCode()
	atvIORWRes.cleanupInternalResources()
	atvIORWRes.cleanupActiveIORWTokens()
	if atvIORWRes.lastResourceMap != nil {
		atvIORWRes.lastResourceMap = nil
	}
}

func (atvIORWRes *ActiveIORWResource) cleanupPassiveContent() {
	if atvIORWRes.psvContents != nil {
		for len(atvIORWRes.psvContents) > 0 {
			atvIORWRes.psvContents[0].CleanupIORW()
			atvIORWRes.psvContents[0] = nil
			if len(atvIORWRes.psvContents) > 1 {
				atvIORWRes.psvContents = atvIORWRes.psvContents[1:]
			} else {
				break
			}
		}
		atvIORWRes.psvContents = nil
	}
}

func (atvIORWRes *ActiveIORWResource) activeCode() *IORW {
	if atvIORWRes.atvCode == nil {
		atvIORWRes.atvCode = &IORW{}
	}
	return atvIORWRes.atvCode
}

func (atvIORWRes *ActiveIORWResource) cleanupActiveCode() {
	if atvIORWRes.atvCode != nil {
		atvIORWRes.atvCode.CleanupIORW()
		atvIORWRes.atvCode = nil
	}
}

type ActiveIORWResourceMap struct {
	internalMap map[string]*ActiveIORWResource
}

func (atvIORWResMap *ActiveIORWResourceMap) activeResource(resource *IOResource, atvIORW *ActiveIORW) (atvIORWRes *ActiveIORWResource) {
	atvIORWResChan := make(chan bool, 1)
	defer close(atvIORWResChan)
	go func() {
		if atvIORWResMap.internalMap == nil {
			atvIORWResMap.internalMap = make(map[string]*ActiveIORWResource)
		}

		if atvIORWRes = atvIORWResMap.internalMap[resource.path]; atvIORWRes == nil || !atvIORWRes.valid(resource.resourceMap) {
			if atvIORWRes != nil {
				atvIORWRes.cleanupActiveIORWResource()
				_, atvIORWRes.atvpsvlabels = ContainsActiveExtension(resource.path)
				parseActiveIORW(atvIORWRes, resource, "", nil)
				atvIORWRes.path = resource.path
				atvIORWRes.modified = resource.modified
			}
		}

		if atvIORWRes == nil {
			atvIORWRes = &ActiveIORWResource{}
			atvIORWRes.path = resource.path
			atvIORWRes.modified = resource.modified
			_, atvIORWRes.atvpsvlabels = ContainsActiveExtension(resource.path)
			if atvIORWRes.atvpsvlabels != nil && len(atvIORWRes.atvpsvlabels) > 0 {
				parseActiveIORW(atvIORWRes, resource, "", nil)
				atvIORWResMap.internalMap[resource.path] = atvIORWRes
			} else {
				atvIORWRes.cleanupActiveIORWResource()
				atvIORWRes = nil
			}
		}

		atvIORWResChan <- true
	}()
	<-atvIORWResChan
	return atvIORWRes
}

var atvIORWResoures *ActiveIORWResourceMap

func ActiveIORWResources() *ActiveIORWResourceMap {
	if atvIORWResoures == nil {
		atvIORWResoures = &ActiveIORWResourceMap{}
	}
	return atvIORWResoures
}

type ActiveIORW struct {
	session ISession
	IORW
	atvElemSettingsByLevel map[int]*ActiveIORWElemSettings
	lastElemSettingsLevel  int
	path                   string

	interupted      bool
	activeMap       map[string]interface{}
	atvIORWResource *ActiveIORWResource
}

func (atvIORW *ActiveIORW) cleanupActiveElemSettings() {
	if atvIORW.atvElemSettingsByLevel != nil {
		for keyLevel, _ := range atvIORW.atvElemSettingsByLevel {
			atvIORW.atvElemSettingsByLevel[keyLevel].CleanupActiveIORWElem()
			delete(atvIORW.atvElemSettingsByLevel, keyLevel)
		}
		atvIORW.atvElemSettingsByLevel = nil
	}
}

func (atvIORW *ActiveIORW) SetActive(key string, value interface{}) {
	if atvIORW.activeMap == nil {
		atvIORW.activeMap = make(map[string]interface{})
	}
	atvIORW.activeMap[key] = nil
	atvIORW.activeMap[key] = value
}

func (atvIORW *ActiveIORW) GetActive(key string) interface{} {
	if atvIORW.activeMap != nil {
		if activeVal, containsActiveVal := atvIORW.activeMap[key]; containsActiveVal {
			return activeVal
		}
	}
	return nil
}

func (atvIORW *ActiveIORW) RemoveActive(key string) {
	if atvIORW.activeMap != nil {
		if _, containsActiveVal := atvIORW.activeMap[key]; containsActiveVal {
			atvIORW.activeMap[key] = nil
			delete(atvIORW.activeMap, key)
		}
	}
}

func (atvIORW *ActiveIORW) cleanupActiveMap() {
	if atvIORW.activeMap != nil {
		for key := range atvIORW.activeMap {
			atvIORW.activeMap[key] = nil
			delete(atvIORW.activeMap, key)
		}
		atvIORW.activeMap = nil
	}
}

func (atvIORW *ActiveIORW) Interupt() {
	atvIORW.interupted = true
}

func (atvIORW *ActiveIORW) IsInterupted() bool {
	return atvIORW.interupted
}

type ActiveIORWElemMap map[string][]interface{}

type ActiveIORWElemSettings struct {
	ActiveIORWElemMap
	atvpsvLabel   [][]byte
	atvpsbPrevB   byte
	atvpsvLabelI  []int
	switches      []string
	atvCode       *IORW
	psvContent    *IORW
	parsedAtvCode *IORW
}

func (atvIORWElem *ActiveIORWElemSettings) parsedActiveCode() *IORW {
	if atvIORWElem.parsedAtvCode == nil {
		atvIORWElem.parsedAtvCode = &IORW{}
	}
	return atvIORWElem.parsedAtvCode
}

func (atvIORWElem *ActiveIORWElemSettings) passiveContent() *IORW {
	if atvIORWElem.psvContent == nil {
		atvIORWElem.psvContent = &IORW{}
	}
	return atvIORWElem.psvContent
}

func (atvIORWElem *ActiveIORWElemSettings) activeCode() *IORW {
	if atvIORWElem.atvCode == nil {
		atvIORWElem.atvCode = &IORW{}
	}
	return atvIORWElem.atvCode
}

func (atvIORWElem *ActiveIORWElemSettings) parseActiveIORWElem(elemLevel int) {
	if !atvIORWElem.empty() {
		for elemkey, elemValues := range atvIORWElem.ActiveIORWElemMap {
			for _, elemValue := range elemValues {
				if _, ok := elemValue.(string); ok {
					value, code, active := atvIORWElem.parseElemValue(elemValue)
					if active {
						atvIORWElem.parsedActiveCode().Print("_script.ElemSettings(", elemLevel, ").Set(\""+elemkey+"\",false,"+code+");")
					} else {
						atvIORWElem.parsedActiveCode().Print("_script.ElemSettings(", elemLevel, ").Set(\""+elemkey+"\",false,", value, ");")
					}
				} else {

				}
			}
		}
	}
}

func (atvIORWElem *ActiveIORWElemSettings) parseElemValue(nvalueToParse interface{}) (value interface{}, code string, active bool) {
	if atvIORWElem.atvCode != nil && !atvIORWElem.atvCode.Empty() {
		atvIORWElem.atvCode.CleanupIORW()
	}
	if atvIORWElem.psvContent != nil && !atvIORWElem.psvContent.Empty() {
		atvIORWElem.psvContent.CleanupIORW()
	}
	if len(atvIORWElem.atvpsvLabel) == 2 {
		atvIORWElem.atvpsvLabelI[0] = 0
		atvIORWElem.atvpsvLabelI[1] = 0
	}
	atvIORWElem.atvpsbPrevB = 0
	var parseElemVal func([]byte) (int, error) = func(p []byte) (n int, err error) {
		if n = len(p); n > 0 {
			for _, b := range p {
				if atvIORWElem.atvpsvLabelI[1] == 0 && atvIORWElem.atvpsvLabelI[0] < len(atvIORWElem.atvpsvLabel[0]) {
					if atvIORWElem.atvpsvLabelI[0] > 0 && atvIORWElem.atvpsvLabel[0][atvIORWElem.atvpsvLabelI[0]-1] == atvIORWElem.atvpsbPrevB && atvIORWElem.atvpsvLabel[0][atvIORWElem.atvpsvLabelI[0]] != b {
						atvIORWElem.passiveContent().Write(atvIORWElem.atvpsvLabel[0][0:atvIORWElem.atvpsvLabelI[0]])
						atvIORWElem.atvpsvLabelI[0] = 0
					}
					if atvIORWElem.atvpsvLabel[0][atvIORWElem.atvpsvLabelI[0]] == b {
						atvIORWElem.atvpsvLabelI[0]++
						if atvIORWElem.atvpsvLabelI[0] == len(atvIORWElem.atvpsvLabel[0]) {

						} else {
							atvIORWElem.atvpsbPrevB = b
						}
					} else {
						if atvIORWElem.atvpsvLabelI[0] > 0 {
							atvIORWElem.passiveContent().Write(atvIORWElem.atvpsvLabel[0][0:atvIORWElem.atvpsvLabelI[0]])
							atvIORWElem.atvpsvLabelI[0] = 0
						}
						atvIORWElem.passiveContent().WriteByte(b)
						atvIORWElem.atvpsbPrevB = b
					}
				} else if atvIORWElem.atvpsvLabelI[0] == len(atvIORWElem.atvpsvLabel[0]) && atvIORWElem.atvpsvLabelI[1] < len(atvIORWElem.atvpsvLabel[1]) {
					if atvIORWElem.atvpsvLabel[1][atvIORWElem.atvpsvLabelI[1]] == b {
						atvIORWElem.atvpsvLabelI[1]++
						if atvIORWElem.atvpsvLabelI[1] == len(atvIORWElem.atvpsvLabel[1]) {

							atvIORWElem.atvpsbPrevB = 0
							atvIORWElem.atvpsvLabelI[0] = 0
							atvIORWElem.atvpsvLabelI[1] = 0
						}
					} else {
						if atvIORWElem.psvContent != nil && !atvIORWElem.psvContent.Empty() {
							atvIORWElem.activeCode().Print("\"", atvIORWElem.psvContent, "\"+")
							atvIORWElem.psvContent.CleanupIORW()
						}
						if atvIORWElem.atvpsvLabelI[1] > 0 {
							atvIORWElem.activeCode().Write(atvIORWElem.atvpsvLabel[1][0:atvIORWElem.atvpsvLabelI[1]])
							atvIORWElem.atvpsvLabelI[1] = 0
						}
						atvIORWElem.activeCode().WriteByte(b)
					}
				}
			}
		}
		return n, err
	}
	if atvIORWElem.psvContent != nil && !atvIORWElem.psvContent.Empty() {
		atvIORWElem.psvContent.CleanupIORW()
	}
	if nsvalue, nsvalueok := nvalueToParse.(string); nsvalueok {
		if nsvalueok {
			if len(nsvalue) > (len(atvIORWElem.atvpsvLabel[0]) + len(atvIORWElem.atvpsvLabel[1])) {
				parseElemVal([]byte(nsvalue))
			} else {
				atvIORWElem.passiveContent().Print(nsvalue)
			}
			if atvIORWElem.psvContent != nil && !atvIORWElem.psvContent.Empty() {
				atvIORWElem.activeCode().Print("\"", atvIORWElem.psvContent, "\"")
				atvIORWElem.psvContent.CleanupIORW()
			}
		}
	} else if niorw, niorwok := nvalueToParse.(IORW); niorwok {
		niorw.ReadToHandler(parseElemVal)
		if atvIORWElem.psvContent != nil && !atvIORWElem.psvContent.Empty() {
			atvIORWElem.activeCode().Print("\"", atvIORWElem.psvContent, "\"")
			atvIORWElem.psvContent.CleanupIORW()
		}
	}

	if atvIORWElem.psvContent != nil && !atvIORWElem.psvContent.Empty() {
		if atvIORWElem.atvCode != nil && !atvIORWElem.atvCode.Empty() {
			atvIORWElem.activeCode().Print("\"", atvIORWElem.psvContent, "\"")
			atvIORWElem.psvContent.CleanupIORW()
		} else {
			value = atvIORWElem.psvContent.String()
		}
	}
	if atvIORWElem.atvCode != nil && !atvIORWElem.atvCode.Empty() {
		code = atvIORWElem.atvCode.String()
		atvIORWElem.atvCode.CleanupIORW()
		active = true
	}

	return value, code, active
}

func (atvIORWElem *ActiveIORWElemSettings) empty() bool {
	return len(atvIORWElem.ActiveIORWElemMap) == 0
}

func (atvIORWElem *ActiveIORWElemSettings) appendMap(clear bool, p map[string][]interface{}) {
	if p != nil && len(p) > 0 {
		for key, val := range p {
			atvIORWElem.Set(key, clear, val...)
		}
	}
}

func (atvIORWElem *ActiveIORWElemSettings) Set(name string, clear bool, a ...interface{}) {
	if clear && len(atvIORWElem.ActiveIORWElemMap[name]) > 0 {
		atvIORWElem.ActiveIORWElemMap[name] = nil
		delete(atvIORWElem.ActiveIORWElemMap, name)
	}
	atvIORWElem.ActiveIORWElemMap[name] = append(atvIORWElem.ActiveIORWElemMap[name], a[:]...)
}

func (atvIORWElem *ActiveIORWElemSettings) Get(name string) (a []interface{}) {
	if len(atvIORWElem.ActiveIORWElemMap) > 0 {
		a = atvIORWElem.ActiveIORWElemMap[name]
	}
	return a
}

func (atvIORWElem *ActiveIORWElemSettings) appendSwitch(clear bool, s ...string) {
	if clear && atvIORWElem.switches != nil && len(atvIORWElem.switches) > 0 {
		atvIORWElem.switches = nil
	}
	if atvIORWElem.switches == nil && len(s) > 0 {
		atvIORWElem.switches = []string{}
	}
	if len(s) > 0 {
		atvIORWElem.switches = append(atvIORWElem.switches, s[:]...)
	}
}

func (atvIORWElem *ActiveIORWElemSettings) CleanupActiveIORWElem() {
	if len(atvIORWElem.ActiveIORWElemMap) > 0 {
		for key, _ := range atvIORWElem.ActiveIORWElemMap {
			atvIORWElem.ActiveIORWElemMap[key] = nil
			delete(atvIORWElem.ActiveIORWElemMap, key)
		}
	}
	if atvIORWElem.atvpsvLabel != nil {
		atvIORWElem.atvpsvLabel = nil
	}
	if atvIORWElem.switches != nil {
		atvIORWElem.switches = nil
	}
	if atvIORWElem.atvCode != nil {
		atvIORWElem.atvCode.CleanupIORW()
		atvIORWElem.atvCode = nil
	}
	if atvIORWElem.psvContent != nil {
		atvIORWElem.psvContent.CleanupIORW()
		atvIORWElem.psvContent = nil
	}
	if atvIORWElem.parsedAtvCode != nil {
		atvIORWElem.parsedAtvCode.CleanupIORW()
		atvIORWElem.parsedAtvCode = nil
	}
}

func newActiveIORWElemSettings() *ActiveIORWElemSettings {
	return &ActiveIORWElemSettings{ActiveIORWElemMap: make(map[string][]interface{})}
}

type ActiveIORWToken struct {
	atvIORWRes           *ActiveIORWResource
	atvIORWTokenIndex    int
	elemResourcePath     string
	elemExt              string
	elemName             string
	atvpsvLabels         [][]byte
	atvpsvLabelsI        []int
	atvpsvPrevB          byte
	psvLabels            [][]byte
	psvLabelsI           []int
	psvPrevB             byte
	unValidatedPsvIORW   *IORW
	unValidIsValid       bool
	unValidFoundIndex    uint64
	unValidIsTested      bool
	psvIORW              *IORW
	atvCodeIORW          *IORW
	parkedElemLabel      string
	parkedElemLevel      int64
	parkedElemLabelLevel int64
	parkIORW             *IORW
	atvIORWElemSettings  *ActiveIORWElemSettings
	postUnparsedIORWs    []*IORW
}

func (atvIORWTkn *ActiveIORWToken) cleanupActiveIORWElemSettings() {
	if atvIORWTkn.atvIORWElemSettings != nil {
		atvIORWTkn.atvIORWElemSettings.CleanupActiveIORWElem()
		atvIORWTkn.atvIORWElemSettings = nil
	}
}

func (atvIORWTkn *ActiveIORWToken) elemSettings() *ActiveIORWElemSettings {
	if atvIORWTkn.atvIORWElemSettings == nil {
		atvIORWTkn.atvIORWElemSettings = newActiveIORWElemSettings()
	}
	return atvIORWTkn.atvIORWElemSettings
}

func (atvIORWTkn *ActiveIORWToken) parkedLabel() string {
	return atvIORWTkn.parkedElemLabel
}

func (atvIORWTkn *ActiveIORWToken) parked() bool {
	return len(atvIORWTkn.parkedElemLabel) > 0
}

func (atvIORWTkn *ActiveIORWToken) precedingActiveIORWToken() *ActiveIORWToken {
	if atvIORWTkn.atvIORWTokenIndex > 0 {
		return atvIORWTkn.atvIORWRes.atvIORWTokens[atvIORWTkn.atvIORWTokenIndex-1]
	}
	return nil
}

func (avIORWTkn *ActiveIORWToken) cleanupPostUnparsedIORWs() {
	if avIORWTkn.postUnparsedIORWs != nil {
		for !avIORWTkn.postUnparsedIORW().Empty() {
			avIORWTkn.postUnparsedIORW().CleanupIORW()
		}
		avIORWTkn.postUnparsedIORWs = nil
	}
}

func (atvIORWTkn *ActiveIORWToken) cleanupActiveIORWToken() {
	if atvIORWTkn.atvIORWRes != nil {
		removeActiveIORWTokenByIndex(atvIORWTkn.atvIORWRes, atvIORWTkn.atvIORWTokenIndex)
		atvIORWTkn.atvIORWRes = nil
	}
	if atvIORWTkn.atvpsvLabels != nil {
		atvIORWTkn.atvpsvLabels = nil
	}
	if atvIORWTkn.atvpsvLabelsI != nil {
		atvIORWTkn.atvpsvLabelsI = nil
	}
	if atvIORWTkn.psvLabels != nil {
		atvIORWTkn.psvLabels = nil
	}
	if atvIORWTkn.psvLabelsI != nil {
		atvIORWTkn.psvLabelsI = nil
	}
	atvIORWTkn.cleanupParkedIORW()
	atvIORWTkn.cleanupUnvalidatedPassiveIORW()
	atvIORWTkn.cleanupPassiveIORW()
	atvIORWTkn.cleanupActiveCodeIORW()
	atvIORWTkn.cleanupPostUnparsedIORWs()
	atvIORWTkn.cleanupActiveIORWElemSettings()
}

func (atvIORWTkn *ActiveIORWToken) parkedIORW() *IORW {
	if atvIORWTkn.parkIORW == nil {
		atvIORWTkn.parkIORW = &IORW{}
	}
	return atvIORWTkn.parkIORW
}

func (atvIORWTkn *ActiveIORWToken) cleanupParkedIORW() {
	if atvIORWTkn.parkIORW != nil {
		atvIORWTkn.parkIORW.CleanupIORW()
		atvIORWTkn.parkIORW = nil
	}
}

func (atvIORWTkn *ActiveIORWToken) PassiveIORW() *IORW {
	if atvIORWTkn.psvIORW == nil {
		atvIORWTkn.psvIORW = &IORW{}
	}
	return atvIORWTkn.psvIORW
}

func (atvIORWTkn *ActiveIORWToken) cleanupPassiveIORW() {
	if atvIORWTkn.psvIORW != nil {
		atvIORWTkn.psvIORW.CleanupIORW()
		atvIORWTkn.psvIORW = nil
	}
}

func (atvIORWTkn *ActiveIORWToken) ActiveCodeIORW() *IORW {
	if atvIORWTkn.atvCodeIORW == nil {
		atvIORWTkn.atvCodeIORW = &IORW{}
	}
	return atvIORWTkn.atvCodeIORW
}

func (atvIORWTkn *ActiveIORWToken) cleanupActiveCodeIORW() {
	if atvIORWTkn.atvCodeIORW != nil {
		atvIORWTkn.atvCodeIORW.CleanupIORW()
		atvIORWTkn.atvCodeIORW = nil
	}
}

func (atvIORWTkn *ActiveIORWToken) postUnparsedIORW() (postUnPIORW *IORW) {
	if len(atvIORWTkn.postUnparsedIORWs) == 0 {
		postUnPIORW = EmptyIO()
	} else {
		if atvIORWTkn.postUnparsedIORWs[len(atvIORWTkn.postUnparsedIORWs)-1].Empty() {
			atvIORWTkn.postUnparsedIORWs[len(atvIORWTkn.postUnparsedIORWs)-1].CleanupIORW()
			atvIORWTkn.postUnparsedIORWs[len(atvIORWTkn.postUnparsedIORWs)-1] = nil
			if len(atvIORWTkn.postUnparsedIORWs) > 1 {
				atvIORWTkn.postUnparsedIORWs = atvIORWTkn.postUnparsedIORWs[0 : len(atvIORWTkn.postUnparsedIORWs)-2]
			} else {
				atvIORWTkn.postUnparsedIORWs = nil
			}
		} else {
			postUnPIORW = atvIORWTkn.postUnparsedIORWs[len(atvIORWTkn.postUnparsedIORWs)-1]
		}
	}
	if postUnPIORW == nil {
		postUnPIORW = atvIORWTkn.postUnparsedIORW()
	}
	return postUnPIORW
}

func (atvIORWTkn *ActiveIORWToken) unValidatedPassiveIORW() *IORW {
	if atvIORWTkn.unValidatedPsvIORW == nil {
		atvIORWTkn.unValidatedPsvIORW = &IORW{}
	}
	return atvIORWTkn.unValidatedPsvIORW
}

func (atvIORWTkn *ActiveIORWToken) cleanupUnvalidatedPassiveIORW() {
	if atvIORWTkn.unValidatedPsvIORW != nil {
		atvIORWTkn.unValidatedPsvIORW.CleanupIORW()
		atvIORWTkn.unValidatedPsvIORW = nil
		atvIORWTkn.unValidIsValid = false
		atvIORWTkn.unValidIsTested = false
		atvIORWTkn.unValidFoundIndex = 0
	}
}

func (atvIORWRes *ActiveIORWResource) appendActiveElemSetting(atvElemsetting *ActiveIORWElemSettings, elemLevel int) {
	if atvElemsetting != nil {
		atvElemsetting.parseActiveIORWElem(elemLevel)
	}
}

func newAtciveIORWToken(
	atvIORWRes *ActiveIORWResource,
	elemResourcePath string,
	elemExt string,
	elemName string,
	atvpsvLabels [][]byte,
	atvpsvLabelsI []int,
	atvpsvPrevB byte,
	psvLabels [][]byte,
	psvLabelsI []int,
	psvPrevB byte, atvElemSettings *ActiveIORWElemSettings) (atvIORWTkn *ActiveIORWToken) {

	atvIORWTkn = &ActiveIORWToken{atvIORWRes: atvIORWRes, atvIORWTokenIndex: len(atvIORWRes.atvIORWTokens), elemResourcePath: elemResourcePath, elemExt: elemExt, elemName: elemName, atvpsvLabels: atvpsvLabels, atvpsvLabelsI: atvpsvLabelsI, atvpsvPrevB: atvpsvPrevB, psvLabels: psvLabels, psvLabelsI: psvLabelsI, psvPrevB: psvPrevB}

	if atvElemSettings != nil && !atvElemSettings.empty() {
		doneElemSettings := make(chan bool, 1)
		defer close(doneElemSettings)
		go func() {
			atvIORWRes.appendActiveElemSetting(atvElemSettings, atvIORWTkn.atvIORWTokenIndex)
			if atvElemSettings.parsedAtvCode != nil && !atvElemSettings.parsedAtvCode.Empty() {
				atvIORWTkn.ActiveCodeIORW().Print(atvElemSettings.parsedAtvCode)
			}
			doneElemSettings <- true
		}()
		<-doneElemSettings
	}

	return atvIORWTkn
}

var emptyAtvIORWElemSettings *ActiveIORWElemSettings

func emptyActiveIORWElemSettings() *ActiveIORWElemSettings {
	if emptyAtvIORWElemSettings == nil {
		emptyAtvIORWElemSettings = newActiveIORWElemSettings()
	}
	return emptyAtvIORWElemSettings
}

func (atvIORW *ActiveIORW) Element() *ActiveIORWElemSettings {
	if atvIORW.atvElemSettingsByLevel == nil {
		return emptyActiveIORWElemSettings()
	}
	if elemSettings := atvIORW.atvElemSettingsByLevel[atvIORW.lastElemSettingsLevel]; elemSettings != nil {
		return elemSettings
	}
	return emptyActiveIORWElemSettings()
}

func (atvIORW *ActiveIORW) ElemSettings(elemSettingsLevel int) *ActiveIORWElemSettings {
	atvIORW.lastElemSettingsLevel = elemSettingsLevel
	if atvIORW.atvElemSettingsByLevel == nil {
		atvIORW.atvElemSettingsByLevel = make(map[int]*ActiveIORWElemSettings)
	}

	atvElemSettings, exist := atvIORW.atvElemSettingsByLevel[elemSettingsLevel]
	if !exist {
		atvElemSettings = newActiveIORWElemSettings()
		atvIORW.atvElemSettingsByLevel[elemSettingsLevel] = atvElemSettings
	}
	return atvElemSettings
}

func (atvIORW *ActiveIORW) executeResource(resource *IOResource, session ISession) {
	if resource != nil {
		if atvIORW.path = resource.path; atvIORW.isActiveIO(resource) {
			atvIORW.session = session

			atvIORW.atvIORWResource = ActiveIORWResources().activeResource(resource, atvIORW)

			if !atvIORW.interupted {
				processActiveIORW(atvIORW)
			}
		} else {
			resource.ReadToWriter(atvIORW)
		}
	}
}

func removeActiveIORWTokenByIndex(atvIORWRes *ActiveIORWResource, tokeni int) {
	if atvIORWRes.atvIORWTokens != nil && tokeni >= 0 && tokeni < len(atvIORWRes.atvIORWTokens) {
		atvIORWRes.atvIORWTokens[tokeni] = nil
		if tokeni == 0 {
			if len(atvIORWRes.atvIORWTokens) > 1 {
				atvIORWRes.atvIORWTokens = atvIORWRes.atvIORWTokens[tokeni+1:]
			} else {
				atvIORWRes.atvIORWTokens = nil
			}
		} else if tokeni == len(atvIORWRes.atvIORWTokens)-1 {
			atvIORWRes.atvIORWTokens = atvIORWRes.atvIORWTokens[0:tokeni]
		} else {
			if len(atvIORWRes.atvIORWTokens) > 1 {
				atvIORWRes.atvIORWTokens = append(atvIORWRes.atvIORWTokens[0:tokeni], atvIORWRes.atvIORWTokens[tokeni+1:]...)
			}
		}
	}
}

func interprateParkedIORW(atvIORWRes *ActiveIORWResource, atvIORWTkn *ActiveIORWToken) bool {
	if atvIORWTkn.parkIORW != nil && !atvIORWTkn.parkIORW.Empty() {
		hasbuffer := (len(atvIORWTkn.parkIORW.rawbuffer) > 0)
		hasbytes := (atvIORWTkn.parkIORW.rawbytesi > 0)
		if hasbuffer || hasbytes {
			if hasbuffer {
				rawbuffers := atvIORWTkn.parkIORW.rawbuffer[:]
				if hasbytes {
					rawbuffers = append(rawbuffers, atvIORWTkn.parkIORW.rawbytes[0:atvIORWTkn.parkIORW.rawbytesi])
				}
				atvIORWTkn.cleanupParkedIORW()
				for _, parkBuf := range rawbuffers {
					parseAtvPsvIORWBytes(atvIORWRes, parkBuf)
				}
				rawbuffers = nil
			} else if hasbytes {
				rawbytes := atvIORWTkn.parkIORW.rawbytes[0:atvIORWTkn.parkIORW.rawbytesi]
				atvIORWTkn.cleanupParkedIORW()
				parseAtvPsvIORWBytes(atvIORWRes, rawbytes)
				rawbytes = nil
			}
		}
		return true
	}
	return false
}

func (atvIORW *ActiveIORW) PrintPsvCntByI(index int) {
	if atvIORW.atvIORWResource != nil && atvIORW.atvIORWResource.psvContents != nil && index < len(atvIORW.atvIORWResource.psvContents) && index >= 0 {
		atvIORW.atvIORWResource.psvContents[index].ReadToWriter(atvIORW)
	}
}

func activeIORWTokenExt(path string) string {
	return filepath.Ext(path)
}

func appendActiveIORWToken(atvIORWRes *ActiveIORWResource, elemName string, elemPath string, atvElemSettings *ActiveIORWElemSettings) {
	if atvIORWRes.atvIORWTokens == nil {
		atvIORWRes.atvIORWTokens = []*ActiveIORWToken{}
	}

	if elemPath != "" && elemName == "" {
		elemExt := filepath.Ext(elemPath)
		elemName = elemPath
		if elemExt != "" {
			elemName = elemName[0 : len(elemName)-len(elemExt)]
		}
		elemName = strings.Replace(elemName, "/", ":", -1)
	}
	atvpsvLabels := [][]byte{[]byte(atvIORWRes.atvpsvlabels[0]), []byte(atvIORWRes.atvpsvlabels[1])}
	atvpsvLabelsI := []int{0, 0}
	psvLabels := [][]byte{[]byte(atvIORWRes.atvpsvlabels[2]), []byte(atvIORWRes.atvpsvlabels[3])}
	psvLabelsI := []int{0, 0}
	atvIORWTkn := newAtciveIORWToken(atvIORWRes, elemPath, activeIORWTokenExt(elemPath), elemName, atvpsvLabels, atvpsvLabelsI, 0, psvLabels, psvLabelsI, 0, atvElemSettings)
	atvIORWRes.atvIORWTokens = append(atvIORWRes.atvIORWTokens, atvIORWTkn)
}

func (atvIORWRes *ActiveIORWResource) currentAtvIORWToken() *ActiveIORWToken {
	if len(atvIORWRes.atvIORWTokens) == 0 {
		return nil
	}
	return atvIORWRes.atvIORWTokens[len(atvIORWRes.atvIORWTokens)-1]
}

func listedResourceTemplates(res *IOResource) (resourcesTemplates []*IOResource) {
	resourcesTemplates = []*IOResource{}
	if ioResTemplates := res.resourceMap.CheckResource(res.path + "-template"); ioResTemplates != nil {
		resourcesTemplates = append(resourcesTemplates, res)
		for _, templatePaths := range strings.Split(ioResTemplates.String(), "\n") {
			for _, templatePath := range strings.Split(strings.TrimSpace(templatePaths), "->") {
				if nextRes := res.resourceMap.CheckResource(templatePath); nextRes != nil {
					if nextTemplateResFound := listedResourceTemplates(nextRes); nextTemplateResFound != nil && len(nextTemplateResFound) > 0 {
						resourcesTemplates = append(resourcesTemplates, nextTemplateResFound...)
					}
				}
			}
		}
		resourcesTemplates = append(resourcesTemplates, ioResTemplates)
	} else {
		resourcesTemplates = append(resourcesTemplates, res)
	}
	return resourcesTemplates
}

func parseActiveIORW(atvIORWRes *ActiveIORWResource, resource *IOResource, resourceName string, adhocIORW *IORW) {
	var atvElemSettings *ActiveIORWElemSettings
	if atvIORWRes.currentAtvIORWToken() != nil {
		atvElemSettings = atvIORWRes.currentAtvIORWToken().atvIORWElemSettings
	}

	if atvElemSettings != nil {
		atvIORWRes.currentAtvIORWToken().atvIORWElemSettings = nil
	}

	appendActiveIORWToken(atvIORWRes, resourceName, resource.path, atvElemSettings)

	resourcesToParse := []*IOResource{}

	if resourcesFound := listedResourceTemplates(resource); resourcesFound != nil {
		if len(resourcesFound) > 0 {
			resourcesToParse = append(resourcesToParse, resourcesFound...)
		}
		resourcesFound = nil
	}

	atvIORWTkn := atvIORWRes.currentAtvIORWToken()

	for len(resourcesToParse) > 0 {
		currentResource := resourcesToParse[len(resourcesToParse)-1]
		atvIORWRes.appendInternalResource(currentResource)
		currentResourcei := 0

		elemBndry := []byte{}
		elemBndry = append(elemBndry, atvIORWTkn.psvLabels[0]...)
		bname := resource.path
		if filepath.Ext(bname) != "" {
			bname = bname[0 : len(bname)-len(filepath.Ext(bname))]
		}
		bname = strings.Replace(bname, "/", ":", -1)
		if strings.LastIndex(bname, ":") > -1 {
			bname = bname[strings.LastIndex(bname, ":"):]
		} else {
			bname = ":" + bname
		}
		elemBndry = append(elemBndry, []byte("."+bname)...)
		elemBndry = append(elemBndry, []byte("/")...)
		elemBndry = append(elemBndry, atvIORWTkn.psvLabels[1]...)
		elemBndryi := 0
		elemBndryPrevB := byte(0)

		tmpPostIORW := &IORW{}

		var processAtvPsvHandle func([]byte) (int, error) = func(p []byte) (n int, err error) {
			n = len(p)
			if elemBndryi == len(elemBndry) {
				tmpPostIORW.Write(p)
			} else {
				for np, b := range p {
					if elemBndryi > 0 && elemBndry[elemBndryi-1] == elemBndryPrevB && elemBndry[elemBndryi] != b {
						parseAtvPsvIORWBytes(atvIORWRes, elemBndry[0:elemBndryi])
						elemBndryi = 0
						elemBndryPrevB = 0
					}
					if elemBndry[elemBndryi] == b {
						elemBndryi++
						if elemBndryi == len(elemBndry) {
							if len(resourcesToParse) == 1 && adhocIORW != nil && !adhocIORW.Empty() {
								parseAtvPsvIORWIORW(atvIORWRes, adhocIORW)
							}
							if np < len(p)-1 {
								tmpPostIORW.Write(p[np+1:])
								break
							}
							elemBndryPrevB = 0
						} else {
							elemBndryPrevB = b
						}
					} else {
						if elemBndryi > 0 {
							parseAtvPsvIORWBytes(atvIORWRes, elemBndry[0:elemBndryi])
							elemBndryi = 0
						}
						parseAtvPsvIORWByte(atvIORWRes, b, atvIORWRes.currentAtvIORWToken())
						elemBndryPrevB = b
					}
				}
			}
			return n, err
		}

		currentResource.ReadToHandler(processAtvPsvHandle)

		if tmpPostIORW != nil {
			if tmpPostIORW.Empty() {
				tmpPostIORW.CleanupIORW()
			} else {
				if atvIORWTkn.postUnparsedIORWs == nil {
					atvIORWTkn.postUnparsedIORWs = []*IORW{}
				}
				atvIORWTkn.postUnparsedIORWs = append(atvIORWTkn.postUnparsedIORWs, tmpPostIORW)
			}
			tmpPostIORW = nil
		}

		currentResource = nil
		resourcesToParse[currentResourcei] = nil
		if currentResourcei > 0 {
			if currentResourcei == len(resourcesToParse)-1 {
				resourcesToParse = resourcesToParse[0:currentResourcei]
			} else {
				resourcesToParse = append(resourcesToParse[0:currentResourcei], resourcesToParse[currentResourcei+1:]...)
			}
		} else if currentResourcei == 0 {
			if len(resourcesToParse) > 1 {
				resourcesToParse = resourcesToParse[currentResourcei+1:]
			} else {
				resourcesToParse = nil
			}
		}
	}

	if atvIORWTkn.postUnparsedIORWs != nil {
		for len(atvIORWTkn.postUnparsedIORWs) > 0 {
			parseAtvPsvIORWIORW(atvIORWRes, atvIORWTkn.postUnparsedIORWs[len(atvIORWTkn.postUnparsedIORWs)-1])
			atvIORWTkn.postUnparsedIORWs[len(atvIORWTkn.postUnparsedIORWs)-1].CleanupIORW()
			atvIORWTkn.postUnparsedIORWs[len(atvIORWTkn.postUnparsedIORWs)-1] = nil
			if len(atvIORWTkn.postUnparsedIORWs) > 1 {
				atvIORWTkn.postUnparsedIORWs = atvIORWTkn.postUnparsedIORWs[0 : len(atvIORWTkn.postUnparsedIORWs)-1]
			} else {
				atvIORWTkn.postUnparsedIORWs = nil
			}
		}
	}

	flushUnvalidatedPassiveIORW(atvIORWRes, atvIORWTkn)
	if atvIORWTkn.psvIORW != nil && !atvIORWTkn.psvIORW.Empty() {
		flushPassiveIORW(atvIORWRes, atvIORWTkn)
	}

	if atvIORWTkn.atvIORWTokenIndex == 0 {
		if atvIORWTkn.atvCodeIORW != nil && !atvIORWTkn.atvCodeIORW.Empty() {
			atvIORWRes.activeCode().InputReader(atvIORWTkn.atvCodeIORW)
			atvIORWTkn.cleanupActiveCodeIORW()
		}
	}

	atvIORWTkn.cleanupActiveIORWToken()
	atvIORWTkn = nil

	if atvElemSettings != nil {
		atvIORWTkn = atvIORWRes.currentAtvIORWToken()
		parseAtvIORWBytes(atvIORWRes, []byte(fmt.Sprint("_script.ElemSettings(", atvIORWTkn.atvIORWTokenIndex+1, ").CleanupActiveIORWElem();")))
		atvElemSettings.CleanupActiveIORWElem()
	}

	if len(atvIORWRes.atvIORWTokens) > 0 {
		interprateParkedIORW(atvIORWRes, atvIORWRes.currentAtvIORWToken())
	}
}

func processActiveIORW(atvIORW *ActiveIORW) {
	if atvIORW.atvIORWResource != nil {
		if atvIORW.atvIORWResource.psvContentsI > 0 {
			//doneNonEval := make(chan bool)
			//defer close(doneNonEval)
			//go func() {
			if atvIORW.atvIORWResource != nil && atvIORW.atvIORWResource.psvContents != nil && atvIORW.atvIORWResource.psvContentsI > 0 {
				for n, _ := range atvIORW.atvIORWResource.psvContents[0:atvIORW.atvIORWResource.psvContentsI] {
					if atvIORW.interupted {
						break
					}
					atvIORW.PrintPsvCntByI(n)
				}
			}
			//	doneNonEval <- true
			//}()
			//<-doneNonEval
		}
		if atvIORW.atvIORWResource.atvCode != nil && !atvIORW.interupted {
			//doneEval := make(chan bool, 1)
			//defer close(doneEval)
			//go func() {
			atvIORW.eval(atvIORW.atvIORWResource.atvCode.String())
			//	doneEval <- true
			//}()
			//<-doneEval
		} else {
		}
	}
}

func (atvIORWRes *ActiveIORW) isActiveIO(resource *IOResource) (contains bool) {
	contains, _ = ContainsActiveExtension(resource.path)
	return contains
}

func (atvIORWRes *ActiveIORWResource) cleanupActiveIORWTokens() {
	if atvIORWRes.atvIORWTokens != nil {
		for len(atvIORWRes.atvIORWTokens) > 0 {
			atvIORWTkn := atvIORWRes.atvIORWTokens[0]
			atvIORWTkn.cleanupActiveIORWToken()
			atvIORWTkn = nil
		}
		atvIORWRes.atvIORWTokens = nil
	}
}

func (atvIORW *ActiveIORW) cleanupActiveIORW() {
	atvIORW.IORW.CleanupIORW()
	if atvIORW.session != nil {
		atvIORW.session = nil
	}
	atvIORW.cleanupActiveMap()
	atvIORW.cleanupActiveElemSettings()
	if atvIORW.atvIORWResource != nil {
		atvIORW.atvIORWResource = nil
	}
}

var activeExtMap map[string][]string

func activeExtionsMap() map[string][]string {
	if activeExtMap == nil {
		activeExtMap = make(map[string][]string)
	}
	return activeExtMap
}

func ContainsActiveExtension(extension string) (contains bool, labels []string) {
	if activeExtMap != nil && extension != "" {
		labels, contains = activeExtMap[filepath.Ext(extension)]
	}
	return contains, labels
}

func RegisterActiveExt(activePreFix, activePostFix, activeParenthasis, passivePreFix, passivePostFix, passiveParenthasis string, extensions ...string) {
	for _, ext := range extensions {
		if ext = filepath.Ext(ext); ext != "" {
			if _, containsMappedExt := activeExtionsMap()[ext]; containsMappedExt {
				activeExtMap[ext] = nil
				activeExtMap[ext] = []string{activePreFix + activeParenthasis, activeParenthasis + activePostFix, passivePreFix + passiveParenthasis, passiveParenthasis + passivePostFix}
			} else {
				activeExtMap[ext] = []string{activePreFix + activeParenthasis, activeParenthasis + activePostFix, passivePreFix + passiveParenthasis, passiveParenthasis + passivePostFix}
			}
		}
	}
}

func RegisterWebActiveExtensions() {
	RegisterActiveExt("<", ">", "%", "<", ">", "", ".htm", ".html", ".xml", ".svg")
	RegisterActiveExt("{", "}", "%", "{", "}", "", ".js", ".json", ".css")
}

func postParseActiveIORWToken(atvIORWRes *ActiveIORWResource, atvIORWTkn *ActiveIORWToken) {
	for !atvIORWTkn.postUnparsedIORW().Empty() {
		parseAtvPsvIORWIORW(atvIORWRes, atvIORWTkn.postUnparsedIORW())
		atvIORWTkn.postUnparsedIORW().CleanupIORW()
	}
}

func parseAtvPsvIORWIORW(atvIORWRes *ActiveIORWResource, ioRW *IORW) {
	ioRW.ReadToHandler(func(p []byte) (n int, err error) {
		if n = len(p); n > 0 {
			parseAtvPsvIORWBytes(atvIORWRes, p)
		}
		return n, err
	})
}

func parseAtvPsvIORWBytes(atvIORWRes *ActiveIORWResource, p []byte) {
	if len(p) > 0 {
		for _, b := range p {
			parseAtvPsvIORWByte(atvIORWRes, b, atvIORWRes.currentAtvIORWToken())
		}
	}
}

func parseAtvPsvIORWByte(atvIORWRes *ActiveIORWResource, b byte, atvIORWTkn *ActiveIORWToken) {
	if atvIORWTkn.atvpsvLabelsI[1] == 0 && atvIORWTkn.atvpsvLabelsI[0] < len(atvIORWTkn.atvpsvLabels[0]) {
		if atvIORWTkn.atvpsvLabelsI[0] > 0 && atvIORWTkn.atvpsvLabels[0][atvIORWTkn.atvpsvLabelsI[0]-1] == atvIORWTkn.atvpsvPrevB && atvIORWTkn.atvpsvLabels[0][atvIORWTkn.atvpsvLabelsI[0]] != b {
			parsePsvIORWBytes(atvIORWRes, atvIORWTkn.atvpsvLabels[0][0:atvIORWTkn.atvpsvLabelsI[0]])
			atvIORWTkn.atvpsvLabelsI[0] = 0
			atvIORWTkn.atvpsvPrevB = 0
		}
		if atvIORWTkn.atvpsvLabels[0][atvIORWTkn.atvpsvLabelsI[0]] == b {
			atvIORWTkn.atvpsvLabelsI[0]++
			if atvIORWTkn.atvpsvLabelsI[0] == len(atvIORWTkn.atvpsvLabels[0]) {
				if atvIORWTkn.parked() {
					atvIORWTkn.parkedIORW().Write(atvIORWTkn.atvpsvLabels[0][0:atvIORWTkn.atvpsvLabelsI[0]])
				} else if atvIORWTkn.unValidIsTested && atvIORWTkn.unValidIsValid {
					atvIORWTkn.unValidatedPassiveIORW().Write(atvIORWTkn.atvpsvLabels[0][0:atvIORWTkn.atvpsvLabelsI[0]])
				}
				atvIORWTkn.atvpsvPrevB = 0
			} else {
				atvIORWTkn.atvpsvPrevB = b
			}
		} else {
			if atvIORWTkn.atvpsvLabelsI[0] > 0 {
				parsePsvIORWBytes(atvIORWRes, atvIORWTkn.atvpsvLabels[0][0:atvIORWTkn.atvpsvLabelsI[0]])
				atvIORWTkn.atvpsvLabelsI[0] = 0
			}
			parsePsvIORWByte(atvIORWRes, b, atvIORWRes.currentAtvIORWToken())
			atvIORWTkn.atvpsvPrevB = b
		}
	} else if atvIORWTkn.atvpsvLabelsI[0] == len(atvIORWTkn.atvpsvLabels[0]) && atvIORWTkn.atvpsvLabelsI[1] < len(atvIORWTkn.atvpsvLabels[1]) {
		if atvIORWTkn.atvpsvLabels[1][atvIORWTkn.atvpsvLabelsI[1]] == b {
			atvIORWTkn.atvpsvLabelsI[1]++
			if atvIORWTkn.atvpsvLabelsI[1] == len(atvIORWTkn.atvpsvLabels[1]) {
				if atvIORWTkn.parked() {
					atvIORWTkn.parkedIORW().Write(atvIORWTkn.atvpsvLabels[1][0:atvIORWTkn.atvpsvLabelsI[1]])
				} else {
					if atvIORWTkn.unValidIsTested && atvIORWTkn.unValidIsValid {
						atvIORWTkn.unValidatedPassiveIORW().Write(atvIORWTkn.atvpsvLabels[1][0:atvIORWTkn.atvpsvLabelsI[1]])
					} else {
						if atvIORWTkn.atvIORWTokenIndex > 0 {
							if atvIORWTkn.atvCodeIORW != nil && !atvIORWTkn.atvCodeIORW.Empty() {
								precAtvIORWTkn := atvIORWTkn.precedingActiveIORWToken()

								precAtvIORWTkn.parkedIORW().Write(precAtvIORWTkn.atvpsvLabels[0])
								precAtvIORWTkn.parkedIORW().WriteFromReader(atvIORWTkn.atvCodeIORW)
								precAtvIORWTkn.parkedIORW().Write(precAtvIORWTkn.atvpsvLabels[1])
								atvIORWTkn.cleanupActiveCodeIORW()
							}
						}
					}
				}
				atvIORWTkn.atvpsvLabelsI[0] = 0
				atvIORWTkn.atvpsvLabelsI[1] = 0
				atvIORWTkn.atvpsvPrevB = 0
			}
		} else {
			if atvIORWTkn.atvpsvLabelsI[1] > 0 {
				if atvIORWTkn.parked() {
					atvIORWTkn.parkedIORW().Write(atvIORWTkn.atvpsvLabels[1][0:atvIORWTkn.atvpsvLabelsI[1]])
				} else {
					parseAtvIORWBytes(atvIORWRes, atvIORWTkn.atvpsvLabels[1][0:atvIORWTkn.atvpsvLabelsI[1]])
				}
				atvIORWTkn.atvpsvLabelsI[1] = 0
			}
			if atvIORWTkn.parked() {
				atvIORWTkn.parkedIORW().WriteByte(b)
			} else {
				parseAtvIORWByte(atvIORWRes, b, atvIORWRes.currentAtvIORWToken())
			}
		}
	}
}

func parsePsvIORWBytes(atvIORWRes *ActiveIORWResource, p []byte) {
	if len(p) > 0 {
		for _, b := range p {
			parsePsvIORWByte(atvIORWRes, b, atvIORWRes.currentAtvIORWToken())
		}
	}
}

func parsePsvIORWByte(atvIORWRes *ActiveIORWResource, b byte, atvIORWTkn *ActiveIORWToken) {
	if atvIORWTkn.psvLabelsI[1] == 0 && atvIORWTkn.psvLabelsI[0] < len(atvIORWTkn.psvLabels[0]) {
		if atvIORWTkn.psvLabelsI[0] > 0 && atvIORWTkn.psvLabels[0][atvIORWTkn.psvLabelsI[0]-1] == atvIORWTkn.psvPrevB && atvIORWTkn.psvLabels[0][atvIORWTkn.psvLabelsI[0]] != b {
			capturePassiveBytes(atvIORWRes, atvIORWTkn.psvLabels[0][0:atvIORWTkn.psvLabelsI[0]], atvIORWTkn)
			atvIORWTkn.psvLabelsI[0] = 0
			atvIORWTkn.psvPrevB = 0
		}
		if atvIORWTkn.psvLabels[0][atvIORWTkn.psvLabelsI[0]] == b {
			atvIORWTkn.psvLabelsI[0]++
			if atvIORWTkn.psvLabelsI[0] == len(atvIORWTkn.psvLabels[0]) {

				atvIORWTkn.psvPrevB = b
			} else {
				atvIORWTkn.psvPrevB = b
			}
		} else {
			if atvIORWTkn.psvLabelsI[0] > 0 {
				capturePassiveBytes(atvIORWRes, atvIORWTkn.psvLabels[0][0:atvIORWTkn.psvLabelsI[0]], atvIORWTkn)
				atvIORWTkn.psvLabelsI[0] = 0
			}
			capturePassiveByte(atvIORWRes, b, atvIORWTkn)
			atvIORWTkn.psvPrevB = b
		}
	} else if atvIORWTkn.psvLabelsI[0] == len(atvIORWTkn.psvLabels[0]) && atvIORWTkn.psvLabelsI[1] < len(atvIORWTkn.psvLabels[1]) {
		if atvIORWTkn.psvLabels[1][atvIORWTkn.psvLabelsI[1]] == b {
			atvIORWTkn.psvLabelsI[1]++
			if atvIORWTkn.psvLabelsI[1] == len(atvIORWTkn.psvLabels[1]) {
				if check, elemComplexity, elemSwitches, elemProps, elemName, elemResourcePath := checkUnvalidatedIORW(atvIORWRes, atvIORWTkn, atvIORWTkn.parked(), atvIORWTkn.parkedLabel()); check {
					if !atvIORWTkn.parked() {
						atvIORWTkn.atvpsvLabelsI[0] = 0
						atvIORWTkn.atvpsvLabelsI[1] = 0
						atvIORWTkn.atvpsvPrevB = 0

						atvIORWTkn.psvLabelsI[0] = 0
						atvIORWTkn.psvLabelsI[1] = 0
						atvIORWTkn.psvPrevB = 0
						flushPassiveIORW(atvIORWRes, atvIORWTkn)
						if elemComplexity == startelemtype {
							var atvIORWTkn *ActiveIORWToken = atvIORWRes.currentAtvIORWToken()
							atvIORWTkn.parkedElemLevel = int64(atvIORWTkn.atvIORWTokenIndex) + 1
							atvIORWTkn.parkedElemLabelLevel = 1
							atvIORWTkn.parkedElemLabel = elemName
							if atvIORWTkn.atvIORWElemSettings != nil {
								atvIORWTkn.cleanupActiveIORWElemSettings()
							}
							if elemProps != nil && len(elemProps) > 0 {
								atvIORWTkn.elemSettings().appendMap(true, elemProps)
								atvIORWTkn.elemSettings().atvpsvLabel = atvIORWTkn.atvpsvLabels[:]
								atvIORWTkn.elemSettings().atvpsvLabelI = make([]int, len(atvIORWTkn.atvpsvLabels))
							}
							if len(elemSwitches) > 0 {
								atvIORWTkn.elemSettings().appendSwitch(true, elemSwitches...)
							}
							atvIORWTkn = nil
						} else if elemComplexity == singleelemtype {
							if atvIORWTkn.atvIORWElemSettings != nil {
								atvIORWTkn.cleanupActiveIORWElemSettings()
							}
							if elemProps != nil && len(elemProps) > 0 {
								atvIORWTkn.elemSettings().appendMap(true, elemProps)
								atvIORWTkn.elemSettings().atvpsvLabel = atvIORWTkn.atvpsvLabels[:]
								atvIORWTkn.elemSettings().atvpsvLabelI = make([]int, len(atvIORWTkn.atvpsvLabels))
							}
							if len(elemSwitches) > 0 {
								atvIORWTkn.elemSettings().appendSwitch(true, elemSwitches...)
							}
							parseActiveIORW(atvIORWRes, atvIORWRes.lastResourceMap.Resource(elemResourcePath), elemName, nil)
						}
					} else {
						if elemComplexity == startelemtype {
							if atvIORWTkn.parkedElemLevel > 0 {
								atvIORWTkn.parkedElemLabelLevel++
							}
						} else if elemComplexity == endelemtype {
							atvIORWTkn.parkedElemLabelLevel--

							if atvIORWTkn.parkedElemLabelLevel == 0 {
								atvIORWTkn.atvpsvLabelsI[0] = 0
								atvIORWTkn.atvpsvLabelsI[1] = 0
								atvIORWTkn.atvpsvPrevB = 0

								atvIORWTkn.psvLabelsI[0] = 0
								atvIORWTkn.psvLabelsI[1] = 0
								atvIORWTkn.psvPrevB = 0

								atvIORWTkn.parkedElemLabel = ""
								atvIORWTkn.parkedElemLabelLevel = 0
								atvIORWTkn.parkedElemLevel = 0
								atvIORWRes := atvIORWTkn.atvIORWRes

								if parkedIORW := atvIORWTkn.parkIORW; parkedIORW != nil && !parkedIORW.Empty() {
									clonedParkedIORW := &IORW{}
									clonedParkedIORW.InputReader(parkedIORW)

									atvIORWTkn.cleanupParkedIORW()
									parseActiveIORW(atvIORWRes, atvIORWRes.lastResourceMap.Resource(elemResourcePath), elemName, clonedParkedIORW)
									clonedParkedIORW.CleanupIORW()
									clonedParkedIORW = nil
								} else {
									parseActiveIORW(atvIORWRes, atvIORWRes.lastResourceMap.Resource(elemResourcePath), elemName, nil)
								}
							}
						}
					}
				} else {
					flushUnvalidatedPassiveIORW(atvIORWRes, atvIORWTkn)
				}
			}
		} else {
			if atvIORWTkn.psvLabelsI[1] > 0 {
				captureUnvalidatedIORWBytes(atvIORWRes, atvIORWTkn.psvLabels[1][0:atvIORWTkn.psvLabelsI[1]], atvIORWTkn)
				atvIORWTkn.psvLabelsI[1] = 0
			}
			captureUnvalidatedIORWByte(atvIORWRes, b, atvIORWTkn)
			atvIORWTkn.psvPrevB = 0
		}
	}
}

type elemcomplexitytype int

func (elemCmplxType elemcomplexitytype) String() string {
	switch elemCmplxType {
	case singleelemtype:
		return "SINGLE"
		break
	case startelemtype:
		return "COMPLEX-START"
		break
	case endelemtype:
		return "COMPLEX-END"
		break
	default:
		return ""
		break
	}
	return ""
}

const singleelemtype elemcomplexitytype = 0
const startelemtype elemcomplexitytype = 1
const endelemtype elemcomplexitytype = 2

func appendedPropValue(a ...interface{}) []interface{} {
	return a
}

func checkUnvalidatedIORW(atvIORWRes *ActiveIORWResource, atvIORWTkn *ActiveIORWToken, parked bool, parkedLabel string) (check bool, elemComplexity elemcomplexitytype, elemSwitches []string, elemProps map[string][]interface{}, elemName string, elemResourcePath string) {
	if atvIORWTkn.unValidIsTested && !atvIORWTkn.unValidIsValid {
		return check, elemComplexity, elemSwitches, elemProps, elemName, elemResourcePath
	} else if atvIORWTkn.unValidatedPsvIORW != nil && !atvIORWTkn.unValidatedPsvIORW.Empty() {
		var validatePsvIORW func() bool = func() (valid bool) {
			foundElemComplexity := false
			if singleElem := (!atvIORWTkn.unValidatedPsvIORW.StartsWith("/") && atvIORWTkn.unValidatedPsvIORW.EndsWith("/")); singleElem {
				elemComplexity = singleelemtype
				foundElemComplexity = true
			} else if complexStartElem := (!atvIORWTkn.unValidatedPsvIORW.StartsWith("/") && !atvIORWTkn.unValidatedPsvIORW.EndsWith("/")); complexStartElem {
				elemComplexity = startelemtype
				foundElemComplexity = true
			} else if complexEndElem := (atvIORWTkn.unValidatedPsvIORW.StartsWith("/") && !atvIORWTkn.unValidatedPsvIORW.EndsWith("/")); complexEndElem {
				elemComplexity = endelemtype
				foundElemComplexity = true
			}

			if foundElemComplexity {
				unvalLength := atvIORWTkn.unValidatedPsvIORW.Length()
				startLength := uint64(0)
				endLength := uint64(0)
				switch elemComplexity {
				case singleelemtype:
					endLength = uint64(len(atvIORWTkn.psvLabels[1]))
					if unvalLength > endLength {
						unvalLength -= endLength
					}
					break
				case endelemtype:
					startLength = uint64(len(atvIORWTkn.psvLabels[0]))
					unvalLength -= startLength
					break
				}

				unvalStage := unvalnamestage
				lastunvalstage := unvalnamestage
				unvalName := ""
				if atvIORWTkn.unValidIsTested && atvIORWTkn.unValidIsValid {
					unvalName = atvIORWTkn.unValidatedPsvIORW.SubString(startLength, (atvIORWTkn.unValidFoundIndex-1)-startLength)
					startLength += atvIORWTkn.unValidFoundIndex
					unvalLength -= atvIORWTkn.unValidFoundIndex
					unvalStage = unvalpropreadstage

				}
				unvalPropName := ""
				unvalPropValue := ""
				unvalPropTextPar := ""
				unvalPropTextPrev := ""
				unvalPropSwitch := ""

				interprateUnvalStage := func(uvb byte, end bool) bool {
					if unvalStage == unvalnamestage {
						if strings.TrimSpace(string(uvb)) == "" || end {
							if end && strings.TrimSpace(string(uvb)) != "" {
								unvalName += string(uvb)
							}
							if unvalName != "" && regexpElemName.MatchString(unvalName) {
								if (!parked) || (parked && (elemComplexity == endelemtype || elemComplexity == startelemtype) && unvalName == parkedLabel) {
									lastunvalstage = unvalStage
									if end {
										valid = true
									} else {
										unvalStage = unvalpropreadstage
									}
								} else {
									valid = false
									return true
								}
							} else {
								valid = false
								return true
							}
						} else {
							unvalName += string(uvb)
						}
					} else if unvalStage == unvalpropreadstage {
						if strings.TrimSpace(string(uvb)) != "" {
							if string(uvb) == "=" {
								if lastunvalstage == unvalpropnamestage {
									if unvalPropSwitch != "" && unvalPropName == "" {
										unvalPropName = unvalPropSwitch
										unvalPropSwitch = ""
									} else if unvalPropName == "" {
										valid = false
										return true
									}
									lastunvalstage = unvalStage
									unvalStage = unvalpropvalreadstage
								}
							} else {
								unvalPropName += string(uvb)
								lastunvalstage = unvalStage
								unvalStage = unvalpropnamestage
							}
						} else if strings.TrimSpace(string(uvb)) == "" {
							if unvalPropSwitch != "" && (lastunvalstage == unvalpropnamestage || end) {
								if elemSwitches == nil {
									elemSwitches = []string{}
								}
								elemSwitches = append(elemSwitches, unvalPropSwitch)
								unvalPropSwitch = ""
								if end {
									valid = true
								}
							}
						}
					} else if unvalStage == unvalpropnamestage {
						if unvalPropName != "" && ((strings.TrimSpace(string(uvb)) != "" && string(uvb) == "=") || strings.TrimSpace(string(uvb)) == "") {
							if regexpElemPropName.MatchString(unvalPropName) {
								lastunvalstage = unvalStage
								if strings.TrimSpace(string(uvb)) == "=" {
									if unvalPropName == "" {
										valid = false
										return true
									}
									unvalStage = unvalpropvalreadstage
								} else if strings.TrimSpace(string(uvb)) == "" {
									unvalPropSwitch = unvalPropName
									unvalPropName = ""
									unvalStage = unvalpropreadstage
								}
							} else {
								valid = false
								return true
							}
						} else if strings.TrimSpace(string(uvb)) != "" {
							unvalPropName += string(uvb)
						}
					} else if unvalStage == unvalpropvalreadstage {
						if strings.TrimSpace(string(uvb)) != "" {
							lastunvalstage = unvalStage
							if strings.TrimSpace(string(uvb)) == "\"" || strings.TrimSpace(string(uvb)) == "'" {
								unvalPropTextPrev = ""
								unvalPropTextPar = strings.TrimSpace(string(uvb))
								unvalStage = unvalpropvaltextstage
							} else {
								unvalPropValue += string(uvb)
								unvalStage = unvalpropvalstage
							}
						}
					} else if unvalStage == unvalpropvaltextstage {
						if string(uvb) == unvalPropTextPar {
							if unvalPropTextPrev == string(uvb) {
								unvalPropValue += string(uvb)
								if end {
									valid = false
									return true
								}
							} else {
								if elemProps == nil {
									elemProps = make(map[string][]interface{})
								}
								elemProps[unvalPropName] = append(elemProps[unvalPropName], unvalPropValue)
								if end {
									valid = true
								} else {
									unvalPropName = ""
									unvalPropValue = ""
									unvalPropSwitch = ""
									unvalPropTextPar = ""
									unvalPropTextPrev = ""
									lastunvalstage = unvalStage
									unvalStage = unvalpropreadstage
								}
							}
						} else {
							unvalPropValue += string(uvb)
							unvalPropTextPrev = string(uvb)
							if end {
								valid = false
							}
						}
					} else if unvalStage == unvalpropvalstage {
						if strings.TrimSpace(string(uvb)) != "" {
							unvalPropValue += string(uvb)
						}
						if strings.TrimSpace(string(uvb)) == "" || end {
							if regexpPropValNumeric.MatchString(unvalPropValue) {
								if elemProps == nil {
									elemProps = make(map[string][]interface{})
								}
								if strings.HasPrefix(unvalPropValue, "+") {
									unvalPropValue = unvalPropValue[1:]
								}
								if strings.Index(unvalPropValue, ".") > 0 {
									fltval, _ := strconv.ParseFloat(unvalPropValue, 64)
									elemProps[unvalPropName] = append(elemProps[unvalPropName], fltval)
								} else {
									intval, _ := strconv.ParseInt(unvalPropValue, 0, 64)
									elemProps[unvalPropName] = append(elemProps[unvalPropName], intval)
								}
							} else if regexpPropValBool.MatchString(unvalPropValue) {
								if elemProps == nil {
									elemProps = make(map[string][]interface{})
								}
								boolval, _ := strconv.ParseBool(unvalPropValue)
								elemProps[unvalPropName] = append(elemProps[unvalPropName], boolval)
							} else {
								valid = false
								return true
							}
							if end {
								valid = true
							} else {
								unvalPropName = ""
								unvalPropValue = ""
								unvalPropSwitch = ""
								lastunvalstage = unvalStage
								unvalStage = unvalpropreadstage
							}
						}
					}

					return end
				}

				atvIORWTkn.unValidatedPsvIORW.ReadToHandler(func(p []byte) (n int, err error) {
					if len(p) > 0 {
						for _, uvb := range p {
							n++
							if startLength > 0 {
								startLength--
							} else if unvalLength > 0 {
								unvalLength--
								if interprateUnvalStage(uvb, unvalLength == 0) {
									err = io.EOF
									break
								}
							} else if endLength > 0 {
								endLength--
							} else {
								err = io.EOF
								break
							}
						}
						if err == nil && startLength == 0 && endLength == 0 && unvalLength == 0 {
							err = io.EOF
						}
					} else {
						err = io.EOF
					}
					return n, err
				})

				if valid {
					if unvalName != "" {
						if strings.LastIndex(unvalName, ".") > 0 {
							elemResourcePath = strings.Replace(unvalName, ":", "/", -1)
							elemName = unvalName[:strings.LastIndex(unvalName, ".")]
						} else {
							elemResourcePath = strings.Replace(unvalName, ":", "/", -1) + atvIORWTkn.elemExt
							elemName = unvalName
						}
						if atvIORWRes.lastResourceMap.Resource(elemResourcePath) == nil {
							valid = false
						}
						unvalName = ""
					}
					if unvalPropSwitch != "" {
						unvalPropSwitch = ""
					}
				}

				if !valid {
					if elemName != "" {
						elemName = ""
					}
					if elemProps != nil {
						if len(elemProps) > 0 {
							for elemPName := range elemProps {
								delete(elemProps, elemPName)
							}
						}
						elemProps = nil
					}
					if elemResourcePath != "" {
						elemResourcePath = ""
					}
					if elemSwitches != nil {
						for len(elemSwitches) > 0 {
							elemSwitches[0] = ""
							if len(elemSwitches) > 1 {
								elemSwitches = elemSwitches[1:]
							} else {
								break
							}
						}
						elemSwitches = nil
					}
				} else {
					atvIORWTkn.cleanupUnvalidatedPassiveIORW()
				}
			}
			return valid
		}
		doneValidating := make(chan bool, 1)
		defer close(doneValidating)
		go func() {
			doneValidating <- validatePsvIORW()
		}()

		check = <-doneValidating
	}
	return check, elemComplexity, elemSwitches, elemProps, elemName, elemResourcePath
}

type unvalstagetype int

const unvalnamestage unvalstagetype = unvalstagetype(0)
const unvalpropreadstage unvalstagetype = unvalstagetype(1)
const unvalpropnamestage unvalstagetype = unvalstagetype(2)
const unvalpropvalstage unvalstagetype = unvalstagetype(3)
const unvalpropvalreadstage unvalstagetype = unvalstagetype(4)
const unvalpropvaltextstage unvalstagetype = unvalstagetype(5)

func captureUnvalidatedIORWBytes(atvIORWRes *ActiveIORWResource, p []byte, atvIORWTkn *ActiveIORWToken) {
	if len(p) > 0 {
		for _, b := range p {
			captureUnvalidatedIORWByte(atvIORWRes, b, atvIORWTkn)
		}
	}
}

func captureUnvalidatedIORWByte(atvIORWRes *ActiveIORWResource, b byte, atvIORWTkn *ActiveIORWToken) {
	if atvIORWTkn.unValidIsTested {
		atvIORWTkn.unValidatedPassiveIORW().WriteByte(b)
	} else {
		atvIORWTkn.unValidFoundIndex++
		if atvIORWTkn.unValidatedPsvIORW != nil && !atvIORWTkn.unValidatedPsvIORW.Empty() && strings.TrimSpace(string(b)) == "" {
			if regexpElemName.MatchString(atvIORWTkn.unValidatedPsvIORW.String()) {
				atvIORWTkn.unValidIsValid = true
			} else {
				atvIORWTkn.unValidIsValid = false
			}
			atvIORWTkn.unValidIsTested = true
			atvIORWTkn.unValidatedPassiveIORW().WriteByte(b)
		} else {
			atvIORWTkn.unValidatedPassiveIORW().WriteByte(b)
		}
	}
}

func capturePassiveIORW(atvIORWRes *ActiveIORWResource, ioRW *IORW, atvIORWTkn *ActiveIORWToken) {
	if !ioRW.Empty() {
		ioRW.ReadToHandler(func(p []byte) (n int, err error) {
			if n = len(p); n > 0 {
				capturePassiveBytes(atvIORWRes, p, atvIORWTkn)
			}
			return n, err
		})
	}
}

func capturePassiveBytes(atvIORWRes *ActiveIORWResource, p []byte, atvIORWTkn *ActiveIORWToken) {
	if len(p) > 0 {
		for _, b := range p {
			capturePassiveByte(atvIORWRes, b, atvIORWTkn)
		}
	}
}

func capturePassiveByte(atvIORWRes *ActiveIORWResource, b byte, atvIORWTkn *ActiveIORWToken) {
	if atvIORWTkn.parked() {
		atvIORWTkn.parkedIORW().WriteByte(b)
	} else {
		atvIORWTkn.PassiveIORW().WriteByte(b)
	}
}

func parseAtvIORWIORW(atvIORWRes *ActiveIORWResource, ioRW *IORW) {
	if ioRW.Empty() {
		return
	}
	ioRW.ReadToHandler(func(p []byte) (n int, err error) {
		if n = len(p); n > 0 {
			parseAtvIORWBytes(atvIORWRes, p)
		}
		return n, err
	})
}

func parseAtvIORWBytes(atvIORWRes *ActiveIORWResource, p []byte) {
	if len(p) > 0 {
		for _, b := range p {
			parseAtvIORWByte(atvIORWRes, b, atvIORWRes.currentAtvIORWToken())
		}
	}
}

func captureAtvIORWBytes(atvIORWRes *ActiveIORWResource, p []byte, atvIORWTkn *ActiveIORWToken) {
	if len(p) > 0 {
		for _, b := range p {
			parseAtvIORWByte(atvIORWRes, b, atvIORWTkn)
		}
	}
}

func captureAtvIORWByte(atvIORWRes *ActiveIORWResource, b byte, atvIORWTkn *ActiveIORWToken) {
	atvIORWTkn.ActiveCodeIORW().WriteByte(b)
}

func flushUnvalidatedPassiveIORW(atvIORWRes *ActiveIORWResource, atvIORWTkn *ActiveIORWToken) {
	if atvIORWTkn.psvLabelsI[0] > 0 {
		capturePassiveBytes(atvIORWRes, atvIORWTkn.psvLabels[0][0:atvIORWTkn.psvLabelsI[0]], atvIORWTkn)
		atvIORWTkn.psvLabelsI[0] = 0
	}

	if atvIORWTkn.unValidatedPsvIORW != nil && !atvIORWTkn.unValidatedPassiveIORW().Empty() {
		capturePassiveIORW(atvIORWRes, atvIORWTkn.unValidatedPassiveIORW(), atvIORWTkn)
		atvIORWTkn.cleanupUnvalidatedPassiveIORW()
	}

	if atvIORWTkn.psvLabelsI[1] > 0 {
		capturePassiveBytes(atvIORWRes, atvIORWTkn.psvLabels[1][0:atvIORWTkn.psvLabelsI[1]], atvIORWTkn)
		atvIORWTkn.psvLabelsI[1] = 0
	}
}

func flushPassiveIORW(atvIORWRes *ActiveIORWResource, atvIORWTkn *ActiveIORWToken) {
	if atvIORWRes != nil {
		if atvIORWTkn.psvIORW != nil && !atvIORWTkn.PassiveIORW().Empty() {
			newPsvIORW := &IORW{}
			newPsvIORW.WriteFromReader(atvIORWTkn.PassiveIORW())
			atvIORWTkn.cleanupPassiveIORW()
			if atvIORWTkn.precedingActiveIORWToken() == nil {
				if atvIORWRes.psvContents == nil {
					atvIORWRes.psvContents = []*IORW{}
				}
				atvIORWRes.psvContents = append(atvIORWRes.psvContents, newPsvIORW)
				if atvIORWTkn.atvCodeIORW != nil {
					parseAtvIORWBytes(atvIORWRes, []byte("_script.PrintPsvCntByI("+fmt.Sprint(int(len(atvIORWRes.psvContents)-1))+");"))
				} else {
					atvIORWRes.psvContentsI = int(len(atvIORWRes.psvContents))
				}
			} else {
				atvIORWTkn.precedingActiveIORWToken().parkedIORW().InputReader(newPsvIORW)
				newPsvIORW.CleanupIORW()
			}
			newPsvIORW = nil
		}
	}
}

func parseAtvIORWByte(atvIORWRes *ActiveIORWResource, b byte, atvIORWTkn *ActiveIORWToken) {
	if atvIORWTkn.unValidIsTested && atvIORWTkn.unValidIsValid {
		atvIORWTkn.unValidatedPassiveIORW().WriteByte(b)
	} else {
		if atvIORWTkn.unValidatedPsvIORW != nil && !atvIORWTkn.unValidatedPsvIORW.Empty() {
			flushUnvalidatedPassiveIORW(atvIORWRes, atvIORWTkn)
		}
		if atvIORWTkn.psvIORW != nil && !atvIORWTkn.PassiveIORW().Empty() {
			flushPassiveIORW(atvIORWRes, atvIORWTkn)
		}
		if len(atvIORWRes.atvIORWTokens) > 0 {
			if !atvIORWRes.currentAtvIORWToken().parked() {
				interprateParkedIORW(atvIORWRes, atvIORWRes.currentAtvIORWToken())
			}
		}
		captureAtvIORWByte(atvIORWRes, b, atvIORWTkn)
	}
}

var regexpElemName *regexp.Regexp
var regexpElemPropName *regexp.Regexp
var regexpPropValNumeric *regexp.Regexp
var regexpPropValBool *regexp.Regexp
var regexpPropValDateTime *regexp.Regexp

const (
	ElemNameMask     string = `^((([a-zA-Z]+[a-zA-Z0-9]*)|(\.+\:+[a-zA-Z]+[a-zA-Z0-9]*)))+(\:+[a-zA-Z]+[a-zA-Z0-9]*)+(\-?([a-zA-Z]+[a-zA-Z0-9]*))?(\.[a-zA-Z]+[a-zA-Z]*)?$`
	ElemPropNameMask string = `^((\-?\-?[a-zA-Z]+[a-zA-Z0-9]*)|([a-zA-Z]+[a-zA-Z0-9]*)|(\-\-?[a-zA-Z]+[a-zA-Z0-9]*))+(\-?[a-zA-Z]*([a-zA-Z][a-zA-Z0-9])*)([^(\-|\W)])$`

	//PROP VALUES MASKS
	ElemNumericMask string = `^[+-]?\d\d*(\.?\d\d*)*$`
	ElemBoolMask    string = `true|false`
)

func init() {
	var regExpErr error
	if regexpElemName == nil {
		regexpElemName, regExpErr = regexp.Compile(ElemNameMask)
		if regExpErr != nil {
			fmt.Println(regExpErr.Error())
		}
	}

	if regexpElemPropName == nil {
		regexpElemPropName, regExpErr = regexp.Compile(ElemPropNameMask)
		if regExpErr != nil {
			fmt.Println(regExpErr.Error())
		}
	}

	if regexpPropValBool == nil {
		regexpPropValBool, regExpErr = regexp.Compile(ElemBoolMask)
		if regExpErr != nil {
			fmt.Println(regExpErr.Error())
		}
	}

	if regexpPropValNumeric == nil {
		regexpPropValNumeric, regExpErr = regexp.Compile(ElemNumericMask)
		if regExpErr != nil {
			fmt.Println(regExpErr.Error())
		}
	}
}
