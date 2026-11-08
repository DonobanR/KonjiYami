package core

import "sync"

var beaconKillList = struct {
	sync.RWMutex
	ids map[string]bool
}{ids: make(map[string]bool)}

func AddToBeaconKillList(id string) {
	beaconKillList.Lock()
	defer beaconKillList.Unlock()
	beaconKillList.ids[id] = true
}

func IsBeaconKilled(id string) bool {
	beaconKillList.RLock()
	defer beaconKillList.RUnlock()
	return beaconKillList.ids[id]
}

func RemoveFromBeaconKillList(id string) {
	beaconKillList.Lock()
	defer beaconKillList.Unlock()
	delete(beaconKillList.ids, id)
}
