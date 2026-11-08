package handlers

/*
	KONJIYAMI Implant Framework
	Copyright (C) 2021  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
	------------------------------------------------------------------------

	WARNING: These functions can be invoked by remote implants without user interaction

*/

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	consts "konjiyami/client/constants"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/core"
	"konjiyami/server/db"
	"konjiyami/server/db/models"
	"konjiyami/server/log"
	"github.com/gofrs/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

var (
	beaconHandlerLog    = log.NamedLogger("handlers", "beacons")
	expireThrottleMu    sync.Mutex
	lastExpireByBeacon  = map[string]time.Time{}
)

func beaconRegisterHandler(implantConn *core.ImplantConnection, data []byte) *konjiyamipb.Envelope {
	beaconReg := &konjiyamipb.BeaconRegister{}
	err := proto.Unmarshal(data, beaconReg)
	if err != nil {
		beaconHandlerLog.Errorf("Error decoding beacon registration message: %s", err)
		return nil
	}
	if core.IsBeaconKilled(beaconReg.ID) {
		beaconHandlerLog.Warnf("Beacon %s is on kill list, ignoring registration", beaconReg.ID)
		return nil
	}
	beaconHandlerLog.Infof("Beacon registration from %s", beaconReg.ID)
	beacon, err := db.BeaconByID(beaconReg.ID)
	beaconHandlerLog.Debugf("Found %v err = %s", beacon, err)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		beaconHandlerLog.Errorf("Database query error %s", err)
		return nil
	}
	isNew := errors.Is(err, gorm.ErrRecordNotFound)
	if isNew {
		beacon = &models.Beacon{
			ID: uuid.FromStringOrNil(beaconReg.ID),
		}
	}
	if isNew || beacon.Name == "" {
		beacon.Name = beaconReg.Register.Name
	}
	beacon.Hostname = beaconReg.Register.Hostname
	beacon.UUID = uuid.FromStringOrNil(beaconReg.Register.Uuid)
	beacon.Username = beaconReg.Register.Username
	beacon.UID = beaconReg.Register.Uid
	beacon.GID = beaconReg.Register.Gid
	beacon.OS = beaconReg.Register.Os
	beacon.Arch = beaconReg.Register.Arch
	beacon.Transport = implantConn.Transport
	beacon.RemoteAddress = implantConn.RemoteAddress
	beacon.PID = beaconReg.Register.Pid
	beacon.Filename = beaconReg.Register.Filename
	beacon.LastCheckin = implantConn.GetLastMessage()
	beacon.Version = beaconReg.Register.Version
	beacon.ReconnectInterval = beaconReg.Register.ReconnectInterval
	beacon.ActiveC2 = beaconReg.Register.ActiveC2
	beacon.ProxyURL = beaconReg.Register.ProxyURL
	// beacon.ConfigID = uuid.FromStringOrNil(beaconReg.Register.ConfigID)
	beacon.Locale = beaconReg.Register.Locale

	beacon.Interval = beaconReg.Interval
	beacon.Jitter = beaconReg.Jitter
	beacon.NextCheckin = time.Now().Unix() + beaconReg.NextCheckin

	err = db.Session().Save(beacon).Error
	if err != nil {
		beaconHandlerLog.Errorf("Database write %s", err)
	}

	eventData, _ := proto.Marshal(beacon.ToProtobuf())
	core.EventBroker.Publish(core.Event{
		EventType: consts.BeaconRegisteredEvent,
		Data:      eventData,
		Beacon:    beacon,
	})

	go auditLogBeacon(beacon, beaconReg.Register)
	return nil
}

type auditLogNewBeaconMsg struct {
	Beacon   *clientpb.Beacon
	Register *konjiyamipb.Register
}

func auditLogBeacon(beacon *models.Beacon, register *konjiyamipb.Register) {
	msg, err := json.Marshal(auditLogNewBeaconMsg{
		Beacon:   beacon.ToProtobuf(),
		Register: register,
	})
	if err != nil {
		beaconHandlerLog.Errorf("Failed to log new beacon to audit log: %s", err)
	} else {
		log.AuditLogger.Warn(string(msg))
	}
}

func beaconTasksHandler(implantConn *core.ImplantConnection, data []byte) *konjiyamipb.Envelope {
	beaconTasks := &konjiyamipb.BeaconTasks{}
	err := proto.Unmarshal(data, beaconTasks)
	if err != nil {
		beaconHandlerLog.Errorf("Error decoding beacon tasks message: %s", err)
		return nil
	}
	if core.IsBeaconKilled(beaconTasks.ID) {
		beaconHandlerLog.Warnf("Beacon %s is on kill list, ignoring tasks", beaconTasks.ID)
		return nil
	}
	go func() {
		err := db.UpdateBeaconCheckinByID(beaconTasks.ID, beaconTasks.NextCheckin)
		if err != nil {
			beaconHandlerLog.Errorf("failed to update checkin: %s", err)
		}
	}()

	// If the message contains tasks then process it as results
	// otherwise send the beacon any pending tasks. Currently we
	// don't receive results and send pending tasks at the same
	// time. We only send pending tasks if the request is empty.
	// If we send the Beacon 0 tasks it should not respond at all.
	if 0 < len(beaconTasks.Tasks) {
		fmt.Printf("[BEACON-RESULTS] beacon=%s returned %d task result(s)\n", beaconTasks.ID, len(beaconTasks.Tasks))
		beaconHandlerLog.Infof("Beacon %s returned %d task result(s)", beaconTasks.ID, len(beaconTasks.Tasks))
		go beaconTaskResults(beaconTasks.ID, beaconTasks.Tasks)
		return nil
	}

	beaconHandlerLog.Infof("Beacon %s requested pending task(s)", beaconTasks.ID)

	// Expire tasks stuck in SENT state (throttled: at most once per minute per beacon)
	expireThrottleMu.Lock()
	last := lastExpireByBeacon[beaconTasks.ID]
	if time.Since(last) > time.Minute {
		lastExpireByBeacon[beaconTasks.ID] = time.Now()
		expireThrottleMu.Unlock()
		go expireStaleTasks(beaconTasks.ID)
	} else {
		expireThrottleMu.Unlock()
	}

	// Pending tasks are ordered by their creation time.
	pendingTasks, err := db.PendingBeaconTasksByBeaconID(beaconTasks.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		beaconHandlerLog.Errorf("Beacon task database error: %s", err)
		return nil
	}
	tasks := []*konjiyamipb.Envelope{}
	for _, pendingTask := range pendingTasks {
		envelope := &konjiyamipb.Envelope{}
		err = proto.Unmarshal(pendingTask.Request, envelope)
		if err != nil {
			beaconHandlerLog.Errorf("Error decoding pending task: %s", err)
			continue
		}
		envelope.ID = pendingTask.EnvelopeID
		tasks = append(tasks, envelope)
		err = db.Session().Model(&models.BeaconTask{}).
			Where("id = ?", pendingTask.ID.String()).
			Updates(map[string]interface{}{
				"state":   models.SENT,
				"sent_at": time.Now(),
			}).Error
		if err != nil {
			beaconHandlerLog.Errorf("Database error: %s", err)
		}
	}
	taskData, err := proto.Marshal(&konjiyamipb.BeaconTasks{Tasks: tasks})
	if err != nil {
		beaconHandlerLog.Errorf("Error marshaling beacon tasks message: %s", err)
		return nil
	}
	beaconHandlerLog.Infof("Sending %d task(s) to beacon %s", len(pendingTasks), beaconTasks.ID)
	return &konjiyamipb.Envelope{
		Type: konjiyamipb.MsgBeaconTasks,
		Data: taskData,
	}
}

const staleTaskTimeout = 5 * time.Minute

func expireStaleTasks(beaconID string) {
	id := uuid.FromStringOrNil(beaconID)
	if id == uuid.Nil {
		return
	}
	cutoff := time.Now().Add(-staleTaskTimeout)
	result := db.Session().Model(&models.BeaconTask{}).
		Where("beacon_id = ? AND state = ? AND sent_at < ?", id.String(), models.SENT, cutoff).
		Updates(map[string]interface{}{
			"state":        models.CANCELED,
			"completed_at": time.Now(),
			"description":  gorm.Expr("description || ' [expired: no result after 5m]'"),
		})
	if result.Error != nil {
		beaconHandlerLog.Errorf("expire stale tasks: %s", result.Error)
	} else if result.RowsAffected > 0 {
		beaconHandlerLog.Warnf("Expired %d stale task(s) for beacon %s", result.RowsAffected, beaconID)
	}
}

func beaconTaskResults(beaconID string, taskEnvelopes []*konjiyamipb.Envelope) *konjiyamipb.Envelope {
	fmt.Printf("[BEACON-RESULTS] processing %d result(s) for beacon %s\n", len(taskEnvelopes), beaconID)
	beaconHandlerLog.Infof("[telemetry] processing %d result(s) for beacon %s", len(taskEnvelopes), beaconID)
	for i, envelope := range taskEnvelopes {
		fmt.Printf("[BEACON-RESULTS] result %d/%d: envelopeID=%d dataLen=%d\n",
			i+1, len(taskEnvelopes), envelope.ID, len(envelope.Data))
		beaconHandlerLog.Infof("[telemetry] result %d/%d: envelopeID=%d dataLen=%d",
			i+1, len(taskEnvelopes), envelope.ID, len(envelope.Data))
		dbTask, err := db.BeaconTaskByEnvelopeID(beaconID, envelope.ID)
		if err != nil {
			fmt.Printf("[BEACON-RESULTS] FAILED to find db task for envelopeID %d: %s\n", envelope.ID, err)
			beaconHandlerLog.Errorf("[telemetry] FAILED to find db task for envelopeID %d: %s", envelope.ID, err)
			continue
		}
		if dbTask == nil {
			beaconHandlerLog.Errorf("[telemetry] nil db task for envelopeID %d!", envelope.ID)
			continue
		}
		now := time.Now()
		err = db.Session().Model(&models.BeaconTask{}).
			Where("id = ?", dbTask.ID.String()).
			Updates(map[string]interface{}{
				"state":        models.COMPLETED,
				"completed_at": now,
				"response":     envelope.Data,
			}).Error
		if err != nil {
			beaconHandlerLog.Errorf("[telemetry] FAILED to update task %s → COMPLETED: %s", dbTask.ID, err)
			continue
		}
		fmt.Printf("[BEACON-RESULTS] task %s → COMPLETED (envelopeID=%d)\n", dbTask.ID, envelope.ID)
		beaconHandlerLog.Infof("[telemetry] task %s → COMPLETED (envelopeID=%d)", dbTask.ID, envelope.ID)
		dbTask.State = models.COMPLETED
		dbTask.CompletedAt = now
		dbTask.Response = envelope.Data
		eventData, _ := proto.Marshal(dbTask.ToProtobuf(false))
		core.EventBroker.Publish(core.Event{
			EventType: consts.BeaconTaskResultEvent,
			Data:      eventData,
		})
	}
	return nil
}

