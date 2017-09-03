package reply

import (
	"encoding/json"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/moira-alert/moira-alert"
	"github.com/moira-alert/moira-alert/database"
	"strconv"
)

// Duty hack for moira.Trigger TTL int64 and stored trigger TTL string compatibility
type triggerStorageElement struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Desc            *string             `json:"desc,omitempty"`
	Targets         []string            `json:"targets"`
	WarnValue       *float64            `json:"warn_value"`
	ErrorValue      *float64            `json:"error_value"`
	Tags            []string            `json:"tags"`
	TTLState        *string             `json:"ttl_state,omitempty"`
	Schedule        *moira.ScheduleData `json:"sched,omitempty"`
	Expression      *string             `json:"expr,omitempty"`
	Patterns        []string            `json:"patterns"`
	IsSimpleTrigger bool                `json:"is_simple_trigger"`
	TTL             *string             `json:"ttl"`
}

func (storageElement *triggerStorageElement) toTrigger() moira.Trigger {
	return moira.Trigger{
		ID:              storageElement.ID,
		Name:            storageElement.Name,
		Desc:            storageElement.Desc,
		Targets:         storageElement.Targets,
		WarnValue:       storageElement.WarnValue,
		ErrorValue:      storageElement.ErrorValue,
		Tags:            storageElement.Tags,
		TTLState:        storageElement.TTLState,
		Schedule:        storageElement.Schedule,
		Expression:      storageElement.Expression,
		Patterns:        storageElement.Patterns,
		IsSimpleTrigger: storageElement.IsSimpleTrigger,
		TTL:             getTriggerTTL(storageElement.TTL),
	}
}

func toTriggerStorageElement(trigger *moira.Trigger, triggerID string) *triggerStorageElement {
	return &triggerStorageElement{
		ID:              triggerID,
		Name:            trigger.Name,
		Desc:            trigger.Desc,
		Targets:         trigger.Targets,
		WarnValue:       trigger.WarnValue,
		ErrorValue:      trigger.ErrorValue,
		Tags:            trigger.Tags,
		TTLState:        trigger.TTLState,
		Schedule:        trigger.Schedule,
		Expression:      trigger.Expression,
		Patterns:        trigger.Patterns,
		IsSimpleTrigger: trigger.IsSimpleTrigger,
		TTL:             getTriggerTTLString(trigger.TTL),
	}
}

func getTriggerTTL(ttlString *string) *int64 {
	if ttlString == nil {
		return nil
	}
	ttl, _ := strconv.ParseInt(*ttlString, 10, 64)
	return &ttl
}

func getTriggerTTLString(ttl *int64) *string {
	if ttl == nil {
		return nil
	}
	ttlString := fmt.Sprintf("%v", *ttl)
	return &ttlString
}

// Trigger converts redis DB reply to moira.Trigger object
func Trigger(rep interface{}, err error) (moira.Trigger, error) {
	bytes, err := redis.Bytes(rep, err)
	if err != nil {
		if err == redis.ErrNil {
			return moira.Trigger{}, database.ErrNil
		}
		return moira.Trigger{}, fmt.Errorf("Failed to read trigger: %s", err.Error())
	}
	triggerSE := &triggerStorageElement{}
	err = json.Unmarshal(bytes, triggerSE)
	if err != nil {
		return moira.Trigger{}, fmt.Errorf("Failed to parse trigger json %s: %s", string(bytes), err.Error())
	}

	return triggerSE.toTrigger(), nil
}

// GetTriggerBytes marshal moira.Trigger to bytes array
func GetTriggerBytes(triggerID string, trigger *moira.Trigger) ([]byte, error) {
	triggerSE := toTriggerStorageElement(trigger, triggerID)
	bytes, err := json.Marshal(triggerSE)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal trigger: %s", err.Error())
	}
	return bytes, nil
}
