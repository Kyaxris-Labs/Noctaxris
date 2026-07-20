package scheduler

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateScheduleJSON builds a CreateSchedule response.
func CreateScheduleJSON(sch store.Schedule) ([]byte, error) {
	return json.Marshal(map[string]any{"ScheduleArn": sch.ScheduleARN})
}

// GetScheduleJSON builds a GetSchedule response.
func GetScheduleJSON(sch store.Schedule) ([]byte, error) {
	out := map[string]any{
		"Arn":                sch.ScheduleARN,
		"Name":               sch.Name,
		"GroupName":          sch.GroupName,
		"ScheduleExpression": sch.Expression,
		"State":              sch.State,
		"Target": map[string]any{
			"Arn":     sch.TargetARN,
			"RoleArn": sch.RoleARN,
			"Input":   sch.Input,
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		"CreationDate":       sch.CreationDate,
		"LastModificationDate": sch.LastModificationDate,
	}
	return json.Marshal(out)
}

// UpdateScheduleJSON builds an UpdateSchedule response.
func UpdateScheduleJSON(sch store.Schedule) ([]byte, error) {
	return json.Marshal(map[string]any{"ScheduleArn": sch.ScheduleARN})
}

// DeleteScheduleJSON is an empty OK body.
func DeleteScheduleJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// ListSchedulesJSON builds a ListSchedules response.
func ListSchedulesJSON(schedules []store.Schedule) ([]byte, error) {
	entries := make([]map[string]any, 0, len(schedules))
	for _, sch := range schedules {
		entries = append(entries, map[string]any{
			"Arn":       sch.ScheduleARN,
			"Name":      sch.Name,
			"GroupName": sch.GroupName,
			"State":     sch.State,
		})
	}
	return json.Marshal(map[string]any{"Schedules": entries})
}
