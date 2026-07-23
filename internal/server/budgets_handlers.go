package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	budgetssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/budgets"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	budgetsJSONContentType = "application/x-amz-json-1.1"
	budgetsEventSource     = "budgets.amazonaws.com"
)

func (s *Server) handleBudgets(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = budgetsAction(action)

	switch action {
	case catalog.ActionBudgetsCreateBudget:
		s.budgetsCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBudgetsDescribeBudget:
		s.budgetsDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBudgetsDescribeBudgets:
		s.budgetsDescribeMany(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBudgetsDeleteBudget:
		s.budgetsDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeBudgetsError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Budgets action is not implemented.", readOnly, eventID, verified)
	}
}

func budgetsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateBudget":
		return catalog.ActionBudgetsCreateBudget
	case "DescribeBudget":
		return catalog.ActionBudgetsDescribeBudget
	case "DescribeBudgets":
		return catalog.ActionBudgetsDescribeBudgets
	case "DeleteBudget":
		return catalog.ActionBudgetsDeleteBudget
	default:
		return action
	}
}

// budgetsBoundAccountID returns verified.AccountID and rejects foreign AccountId body overrides.
func budgetsBoundAccountID(verified *authn.Verified, params map[string]any) (string, error) {
	accountID := verified.AccountID
	if aid, ok := params["AccountId"].(string); ok && strings.TrimSpace(aid) != "" {
		if strings.TrimSpace(aid) != verified.AccountID {
			return "", errors.New("AccountId must match the caller account")
		}
	}
	return accountID, nil
}

func (s *Server) budgetsCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBudgetsCreateBudget, "*") {
		s.writeBudgetsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform budgets:CreateBudget.", readOnly, eventID, verified)
		return
	}
	accountID, err := budgetsBoundAccountID(verified, params)
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	name, budgetType, timeUnit, amount, unit := "", "COST", "MONTHLY", "", "USD"
	var notifications any
	if b, ok := params["Budget"].(map[string]any); ok {
		name, _ = b["BudgetName"].(string)
		if t, ok := b["BudgetType"].(string); ok && t != "" {
			budgetType = t
		}
		if tu, ok := b["TimeUnit"].(string); ok && tu != "" {
			timeUnit = tu
		}
		if lim, ok := b["BudgetLimit"].(map[string]any); ok {
			amount, _ = lim["Amount"].(string)
			if u, ok := lim["Unit"].(string); ok && u != "" {
				unit = u
			}
		}
	}
	if n, ok := params["NotificationsWithSubscribers"]; ok {
		notifications = n
	}
	_, err = s.store.CreateBudget(accountID, name, budgetType, timeUnit, amount, unit, notifications)
	if errors.Is(err, store.ErrBudgetExists) {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "DuplicateRecordException",
			"Budget already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBudgetBadRequest) {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to create budget.", readOnly, eventID, verified)
		return
	}
	payload, _ := budgetssvc.CreateBudgetJSON()
	s.writeBudgetsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, budgetsEventSource, "CreateBudget", readOnly)
}

func (s *Server) budgetsDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	accountID, err := budgetsBoundAccountID(verified, params)
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	name, _ := params["BudgetName"].(string)
	if !s.authorize(verified, catalog.ActionBudgetsDescribeBudget, "*") {
		s.writeBudgetsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform budgets:DescribeBudget.", readOnly, eventID, verified)
		return
	}
	b, err := s.store.DescribeBudget(accountID, name)
	if errors.Is(err, store.ErrBudgetNotFound) {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Budget not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to describe budget.", readOnly, eventID, verified)
		return
	}
	payload, _ := budgetssvc.DescribeBudgetJSON(b)
	s.writeBudgetsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, budgetsEventSource, "DescribeBudget", readOnly)
}

func (s *Server) budgetsDescribeMany(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	accountID, err := budgetsBoundAccountID(verified, params)
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionBudgetsDescribeBudgets, "*") {
		s.writeBudgetsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform budgets:DescribeBudgets.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.DescribeBudgets(accountID)
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to describe budgets.", readOnly, eventID, verified)
		return
	}
	payload, _ := budgetssvc.DescribeBudgetsJSON(list)
	s.writeBudgetsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, budgetsEventSource, "DescribeBudgets", readOnly)
}

func (s *Server) budgetsDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	accountID, err := budgetsBoundAccountID(verified, params)
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	name, _ := params["BudgetName"].(string)
	if !s.authorize(verified, catalog.ActionBudgetsDeleteBudget, "*") {
		s.writeBudgetsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform budgets:DeleteBudget.", readOnly, eventID, verified)
		return
	}
	err = s.store.DeleteBudget(accountID, name)
	if errors.Is(err, store.ErrBudgetNotFound) {
		s.writeBudgetsError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Budget not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBudgetsError(w, r, body, requestID, http.StatusInternalServerError, "InternalErrorException",
			"Unable to delete budget.", readOnly, eventID, verified)
		return
	}
	payload, _ := budgetssvc.DeleteBudgetJSON()
	s.writeBudgetsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, budgetsEventSource, "DeleteBudget", readOnly)
}

func (s *Server) writeBudgetsOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", budgetsJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeBudgetsError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", budgetsJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, budgetsEventSource, code, readOnly)
}
