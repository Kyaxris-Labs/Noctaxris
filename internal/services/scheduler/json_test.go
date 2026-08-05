package scheduler_test

import (
	"encoding/json"
	"testing"

	schedulersvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/scheduler"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSchedulerJSON(t *testing.T) {
	sch := store.Schedule{
		ScheduleARN: "arn:aws:scheduler:us-east-1:1:schedule/default/lab",
		Name: "lab", GroupName: "default", Expression: "rate(1 hour)",
		State: "ENABLED", TargetARN: "arn:aws:lambda:us-east-1:1:function:f",
		RoleARN: "arn:aws:iam::1:role/sch", Input: `{}`,
		CreationDate: "2024-01-01T00:00:00Z", LastModificationDate: "2024-01-02T00:00:00Z",
	}
	createRaw, err := schedulersvc.CreateScheduleJSON(sch)
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]string
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	if createOut["ScheduleArn"] != sch.ScheduleARN {
		t.Fatalf("create=%v", createOut)
	}

	getRaw, err := schedulersvc.GetScheduleJSON(sch)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	target, _ := getOut["Target"].(map[string]any)
	if target["Arn"] != sch.TargetARN {
		t.Fatalf("get=%v", getOut)
	}

	updRaw, err := schedulersvc.UpdateScheduleJSON(sch)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(updRaw, &createOut); err != nil {
		t.Fatal(err)
	}

	delRaw, err := schedulersvc.DeleteScheduleJSON()
	if err != nil || string(delRaw) != "{}" {
		t.Fatalf("delete=%s err=%v", delRaw, err)
	}

	listRaw, err := schedulersvc.ListSchedulesJSON([]store.Schedule{sch})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	entries, _ := listOut["Schedules"].([]any)
	if len(entries) != 1 {
		t.Fatalf("list=%v", listOut)
	}
}
