package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBudgetsDescribeManyAndErrors(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	empty := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.DescribeBudgets", "budgets", map[string]any{
		"AccountId": testAccountID,
	}, now)
	if empty.Code != http.StatusOK {
		t.Fatalf("DescribeBudgets empty status=%d body=%q", empty.Code, empty.Body.String())
	}

	create := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.CreateBudget", "budgets", map[string]any{
		"AccountId": testAccountID,
		"Budget": map[string]any{
			"BudgetName": "cov-budget",
			"BudgetType": "COST",
			"TimeUnit":   "MONTHLY",
			"BudgetLimit": map[string]string{
				"Amount": "10.0",
				"Unit":   "USD",
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateBudget status=%d body=%q", create.Code, create.Body.String())
	}

	many := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.DescribeBudgets", "budgets", map[string]any{
		"AccountId": testAccountID,
	}, now)
	if many.Code != http.StatusOK || !strings.Contains(many.Body.String(), "cov-budget") {
		t.Fatalf("DescribeBudgets status=%d body=%q", many.Code, many.Body.String())
	}

	foreign := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.DescribeBudgets", "budgets", map[string]any{
		"AccountId": "999999999999",
	}, now)
	if foreign.Code != http.StatusBadRequest || !strings.Contains(foreign.Body.String(), "InvalidParameterException") {
		t.Fatalf("DescribeBudgets foreign AccountId want InvalidParameter status=%d body=%q", foreign.Code, foreign.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.UpdateBudget", "budgets", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented || !strings.Contains(unknown.Body.String(), "InvalidAction") {
		t.Fatalf("unknown Budgets action want InvalidAction status=%d body=%q", unknown.Code, unknown.Body.String())
	}

	missing := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.DescribeBudget", "budgets", map[string]any{
		"AccountId":  testAccountID,
		"BudgetName": "no-such",
	}, now)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "NotFoundException") {
		t.Fatalf("DescribeBudget missing want NotFound status=%d body=%q", missing.Code, missing.Body.String())
	}
}

func TestBCMGetDeleteExportAndErrors(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.CreateExport", "bcm-data-exports", map[string]any{
		"Export": map[string]any{
			"Name":        "cov-cur",
			"Description": "lab",
			"DestinationConfigurations": map[string]any{
				"S3Destination": map[string]any{
					"S3OutputConfigurations": map[string]any{"Format": "CSV"},
				},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateExport status=%d body=%q", create.Code, create.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &resp)
	arn, _ := resp["ExportArn"].(string)
	if arn == "" {
		t.Fatalf("missing ExportArn: %s", create.Body.String())
	}

	get := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.GetExport", "bcm-data-exports", map[string]any{
		"ExportArn": arn,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "cov-cur") {
		t.Fatalf("GetExport status=%d body=%q", get.Code, get.Body.String())
	}

	missing := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.GetExport", "bcm-data-exports", map[string]any{
		"ExportArn": "arn:aws:bcm-data-exports:us-east-1:000000000001:export/missing",
	}, now)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("GetExport missing want ResourceNotFound status=%d body=%q", missing.Code, missing.Body.String())
	}

	del := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.DeleteExport", "bcm-data-exports", map[string]any{
		"ExportArn": arn,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteExport status=%d body=%q", del.Code, del.Body.String())
	}

	delGone := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.DeleteExport", "bcm-data-exports", map[string]any{
		"ExportArn": arn,
	}, now)
	if delGone.Code != http.StatusBadRequest || !strings.Contains(delGone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DeleteExport missing want ResourceNotFound status=%d body=%q", delGone.Code, delGone.Body.String())
	}

	badCreate := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.CreateExport", "bcm-data-exports", map[string]any{
		"Export": map[string]any{"Name": ""},
	}, now)
	if badCreate.Code != http.StatusBadRequest || !strings.Contains(badCreate.Body.String(), "ValidationException") {
		t.Fatalf("CreateExport empty name want ValidationException status=%d body=%q", badCreate.Code, badCreate.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.UpdateExport", "bcm-data-exports", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented || !strings.Contains(unknown.Body.String(), "InvalidAction") {
		t.Fatalf("unknown BCM action want InvalidAction status=%d body=%q", unknown.Code, unknown.Body.String())
	}
}
