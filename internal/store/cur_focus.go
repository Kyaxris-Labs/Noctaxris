package store

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UsageLine is one synthesized cost-and-usage row before FOCUS projection.
// Shape mirrors Floci's UsageLine (CE / CUR shared model).
type UsageLine struct {
	PeriodStart     time.Time
	PeriodEnd       time.Time
	Service         string
	Region          string
	UsageType       string
	Operation       string
	RecordType      string
	LinkedAccountID string
	ResourceID      string
	Tags            map[string]string
	Quantity        float64
	UsageUnit       string
}

// Usage record types (subset used by the lab projector).
const (
	UsageRecordTypeUsage  = "Usage"
	UsageRecordTypeCredit = "Credit"
	UsageRecordTypeTax    = "Tax"
	UsageRecordTypeRefund = "Refund"
)

// ResourceUsageEnumerator enumerates lab resources into UsageLine rows for CUR emission.
type ResourceUsageEnumerator interface {
	Enumerate(accountID, region string, periodStart, periodEnd time.Time) ([]UsageLine, error)
}

// FocusRow is one FOCUS 1.2 / CUR 2.0 column row (lab subset).
type FocusRow struct {
	BillingPeriodStart string            `json:"BillingPeriodStart"`
	BillingPeriodEnd   string            `json:"BillingPeriodEnd"`
	ChargePeriodStart  string            `json:"ChargePeriodStart"`
	ChargePeriodEnd    string            `json:"ChargePeriodEnd"`
	BillingAccountId   string            `json:"BillingAccountId"`
	SubAccountId       string            `json:"SubAccountId"`
	ServiceCategory    string            `json:"ServiceCategory"`
	ServiceName        string            `json:"ServiceName"`
	Region             string            `json:"Region"`
	ResourceId         string            `json:"ResourceId,omitempty"`
	ResourceName       string            `json:"ResourceName,omitempty"`
	ResourceType       string            `json:"ResourceType,omitempty"`
	ChargeCategory     string            `json:"ChargeCategory"`
	ChargeClass        string            `json:"ChargeClass"`
	ChargeDescription  string            `json:"ChargeDescription"`
	ChargeFrequency    string            `json:"ChargeFrequency"`
	ChargeSubcategory  string            `json:"ChargeSubcategory"`
	PricingQuantity    float64           `json:"PricingQuantity"`
	PricingUnit        string            `json:"PricingUnit"`
	UsageQuantity      float64           `json:"UsageQuantity"`
	UsageUnit          string            `json:"UsageUnit"`
	BilledCost         float64           `json:"BilledCost"`
	EffectiveCost      float64           `json:"EffectiveCost"`
	ListCost           float64           `json:"ListCost"`
	ContractedCost     float64           `json:"ContractedCost"`
	BillingCurrency    string            `json:"BillingCurrency"`
	Tags               map[string]string `json:"Tags,omitempty"`
}

// focusCSVHeader is the fixed CSV column order for FOCUS emits.
var focusCSVHeader = []string{
	"BillingPeriodStart", "BillingPeriodEnd",
	"ChargePeriodStart", "ChargePeriodEnd",
	"BillingAccountId", "SubAccountId",
	"ServiceCategory", "ServiceName",
	"Region",
	"ResourceId", "ResourceName", "ResourceType",
	"ChargeCategory", "ChargeClass", "ChargeDescription",
	"ChargeFrequency", "ChargeSubcategory",
	"PricingQuantity", "PricingUnit",
	"UsageQuantity", "UsageUnit",
	"BilledCost", "EffectiveCost", "ListCost", "ContractedCost",
	"BillingCurrency",
}

// SetCURUsageEnumerators replaces extra enumerators. Built-in S3 and Lambda
// enumerators always run; extras append after them.
func (s *Store) SetCURUsageEnumerators(enums ...ResourceUsageEnumerator) {
	s.curEnumMu.Lock()
	defer s.curEnumMu.Unlock()
	s.curExtraEnumerators = append([]ResourceUsageEnumerator(nil), enums...)
}

func (s *Store) curUsageEnumerators() []ResourceUsageEnumerator {
	out := []ResourceUsageEnumerator{
		&s3CURUsageEnumerator{store: s},
		&lambdaCURUsageEnumerator{store: s},
	}
	s.curEnumMu.Lock()
	defer s.curEnumMu.Unlock()
	if len(s.curExtraEnumerators) > 0 {
		out = append(out, s.curExtraEnumerators...)
	}
	return out
}

// CollectCURUsageLines runs registered enumerators for the current UTC month.
func (s *Store) CollectCURUsageLines(accountID, region string) []UsageLine {
	now := time.Now().UTC()
	start, end := curBillingMonthBounds(now)
	if region == "" {
		region = "us-east-1"
	}
	var out []UsageLine
	for _, e := range s.curUsageEnumerators() {
		lines, err := e.Enumerate(accountID, region, start, end)
		if err != nil {
			continue
		}
		out = append(out, lines...)
	}
	return out
}

// ProjectFOCUSRows maps UsageLine rows to FOCUS columns (pure; no I/O).
func ProjectFOCUSRows(lines []UsageLine) []FocusRow {
	rows := make([]FocusRow, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, projectFOCUSOne(line))
	}
	return rows
}

func projectFOCUSOne(line UsageLine) FocusRow {
	cost := curLabCostFor(line)
	tags := line.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	billingStart, billingEnd := curBillingMonthBounds(line.PeriodStart)
	if line.PeriodStart.IsZero() {
		billingStart, billingEnd = curBillingMonthBounds(time.Now().UTC())
	}
	chargeStart := line.PeriodStart
	chargeEnd := line.PeriodEnd
	if chargeStart.IsZero() {
		chargeStart = billingStart
	}
	if chargeEnd.IsZero() {
		chargeEnd = billingEnd
	}
	return FocusRow{
		BillingPeriodStart: billingStart.Format(time.RFC3339),
		BillingPeriodEnd:   billingEnd.Format(time.RFC3339),
		ChargePeriodStart:  chargeStart.Format(time.RFC3339),
		ChargePeriodEnd:    chargeEnd.Format(time.RFC3339),
		BillingAccountId:   line.LinkedAccountID,
		SubAccountId:       line.LinkedAccountID,
		ServiceCategory:    curServiceCategory(line.Service),
		ServiceName:        line.Service,
		Region:             line.Region,
		ResourceId:         line.ResourceID,
		ResourceName:       line.ResourceID,
		ResourceType:       curResourceType(line.Service),
		ChargeCategory:     curChargeCategory(line.RecordType),
		ChargeClass:        "Standard",
		ChargeDescription:  "Lab usage line",
		ChargeFrequency:    "Usage-Based",
		ChargeSubcategory:  line.UsageType,
		PricingQuantity:    line.Quantity,
		PricingUnit:        line.UsageUnit,
		UsageQuantity:      line.Quantity,
		UsageUnit:          line.UsageUnit,
		BilledCost:         cost,
		EffectiveCost:      cost,
		ListCost:           cost,
		ContractedCost:     cost,
		BillingCurrency:    "USD",
		Tags:               tags,
	}
}

func curLabCostFor(line UsageLine) float64 {
	switch line.RecordType {
	case UsageRecordTypeCredit, UsageRecordTypeTax, UsageRecordTypeRefund:
		return line.Quantity
	}
	return line.Quantity * curLabUnitPrice(line.Service, line.UsageType)
}

func curLabUnitPrice(service, usageType string) float64 {
	switch service {
	case "AmazonS3":
		return 0.023
	case "AWSLambda":
		return 0.0000002
	default:
		_ = usageType
		return 0
	}
}

func curBillingMonthBounds(t time.Time) (start, end time.Time) {
	t = t.UTC()
	start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0)
	return start, end
}

func curServiceCategory(serviceCode string) string {
	switch serviceCode {
	case "AmazonEC2", "AWSLambda", "AmazonECS", "AmazonEKS", "AWSBatch":
		return "Compute"
	case "AmazonS3", "AmazonEBS":
		return "Storage"
	case "AmazonRDS", "AmazonDynamoDB":
		return "Databases"
	default:
		return "Other"
	}
}

func curResourceType(serviceCode string) string {
	switch serviceCode {
	case "AmazonS3":
		return "Bucket"
	case "AWSLambda":
		return "Function"
	case "AmazonEC2":
		return "Instance"
	default:
		return serviceCode
	}
}

func curChargeCategory(recordType string) string {
	switch recordType {
	case UsageRecordTypeCredit:
		return "Credit"
	case UsageRecordTypeTax:
		return "Tax"
	case UsageRecordTypeRefund:
		return "Refund"
	default:
		return "Usage"
	}
}

// curRequestsFOCUS is true when Format is Parquet (Floci-style FOCUS emit) or
// AdditionalSchemaElements includes FOCUS (CSV FOCUS opt-in).
func curRequestsFOCUS(d *CURReportDefinition) bool {
	if d == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(d.Format), "Parquet") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(d.Format), "FOCUS") {
		return true
	}
	for _, e := range d.AdditionalSchemaElements {
		if strings.EqualFold(strings.TrimSpace(e), "FOCUS") {
			return true
		}
	}
	return false
}

func encodeFOCUSCSV(rows []FocusRow) ([]byte, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(focusCSVHeader); err != nil {
		return nil, err
	}
	for _, row := range rows {
		rec := []string{
			row.BillingPeriodStart, row.BillingPeriodEnd,
			row.ChargePeriodStart, row.ChargePeriodEnd,
			row.BillingAccountId, row.SubAccountId,
			row.ServiceCategory, row.ServiceName,
			row.Region,
			row.ResourceId, row.ResourceName, row.ResourceType,
			row.ChargeCategory, row.ChargeClass, row.ChargeDescription,
			row.ChargeFrequency, row.ChargeSubcategory,
			fmt.Sprintf("%g", row.PricingQuantity), row.PricingUnit,
			fmt.Sprintf("%g", row.UsageQuantity), row.UsageUnit,
			fmt.Sprintf("%g", row.BilledCost), fmt.Sprintf("%g", row.EffectiveCost),
			fmt.Sprintf("%g", row.ListCost), fmt.Sprintf("%g", row.ContractedCost),
			row.BillingCurrency,
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func encodeFOCUSNDJSON(rows []FocusRow) ([]byte, error) {
	var b strings.Builder
	for _, row := range rows {
		line, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// --- built-in enumerators ----------------------------------------------------

type s3CURUsageEnumerator struct{ store *Store }

func (e *s3CURUsageEnumerator) Enumerate(accountID, region string, periodStart, periodEnd time.Time) ([]UsageLine, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("s3 cur enumerator: store is nil")
	}
	if !periodEnd.After(periodStart) {
		return nil, nil
	}
	buckets, err := e.store.ListBuckets(accountID)
	if err != nil {
		return nil, err
	}
	const service = "AmazonS3"
	const usageType = "TimedStorage-Standard"
	out := make([]UsageLine, 0, len(buckets)+1)
	// Catalog / count line: quantity is the number of buckets in the account.
	out = append(out, UsageLine{
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		Service:         service,
		Region:          region,
		UsageType:       usageType,
		Operation:       "StandardStorage",
		RecordType:      UsageRecordTypeUsage,
		LinkedAccountID: accountID,
		Quantity:        float64(len(buckets)),
		UsageUnit:       "Count",
	})
	for _, b := range buckets {
		out = append(out, UsageLine{
			PeriodStart:     periodStart,
			PeriodEnd:       periodEnd,
			Service:         service,
			Region:          region,
			UsageType:       usageType,
			Operation:       "StandardStorage",
			RecordType:      UsageRecordTypeUsage,
			LinkedAccountID: accountID,
			ResourceID:      "arn:aws:s3:::" + b.Name,
			Tags:            map[string]string{},
			Quantity:        1,
			UsageUnit:       "Count",
		})
	}
	return out, nil
}

type lambdaCURUsageEnumerator struct{ store *Store }

func (e *lambdaCURUsageEnumerator) Enumerate(accountID, region string, periodStart, periodEnd time.Time) ([]UsageLine, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("lambda cur enumerator: store is nil")
	}
	if !periodEnd.After(periodStart) {
		return nil, nil
	}
	fns, err := e.store.ListFunctions(accountID)
	if err != nil {
		return nil, err
	}
	const service = "AWSLambda"
	const usageType = "AWS-Lambda-Requests"
	out := make([]UsageLine, 0, len(fns)+1)
	out = append(out, UsageLine{
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		Service:         service,
		Region:          region,
		UsageType:       usageType,
		Operation:       "Invoke",
		RecordType:      UsageRecordTypeUsage,
		LinkedAccountID: accountID,
		Quantity:        float64(len(fns)),
		UsageUnit:       "Count",
	})
	for _, fn := range fns {
		out = append(out, UsageLine{
			PeriodStart:     periodStart,
			PeriodEnd:       periodEnd,
			Service:         service,
			Region:          region,
			UsageType:       usageType,
			Operation:       "Invoke",
			RecordType:      UsageRecordTypeUsage,
			LinkedAccountID: accountID,
			ResourceID:      fn.FunctionARN,
			Tags:            map[string]string{},
			Quantity:        1,
			UsageUnit:       "Count",
		})
	}
	return out, nil
}
