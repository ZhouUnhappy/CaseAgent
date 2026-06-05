package workflow

import (
	"fmt"

	"caseagent/internal/db/models"
)

type TransitionEvent string

const (
	TransitionStart   TransitionEvent = "start"
	TransitionSucceed TransitionEvent = "succeed"
	TransitionFail    TransitionEvent = "fail"
	TransitionCancel  TransitionEvent = "cancel"
	TransitionReplay  TransitionEvent = "replay"
)

type Transition struct {
	From  string
	Event TransitionEvent
	To    string
}

func NextStatus(current string, event TransitionEvent) (Transition, error) {
	var ok bool
	current, ok = canonicalWorkflowStatus(current)
	if !ok {
		return Transition{}, fmt.Errorf("illegal workflow status: %s", current)
	}
	switch current {
	case models.WorkflowStatusPending:
		switch event {
		case TransitionStart:
			return Transition{From: current, Event: event, To: models.WorkflowStatusRunning}, nil
		case TransitionCancel:
			return Transition{From: current, Event: event, To: models.WorkflowStatusCanceled}, nil
		}
	case models.WorkflowStatusRunning:
		switch event {
		case TransitionSucceed:
			return Transition{From: current, Event: event, To: models.WorkflowStatusSucceeded}, nil
		case TransitionFail:
			return Transition{From: current, Event: event, To: models.WorkflowStatusFailed}, nil
		case TransitionCancel:
			return Transition{From: current, Event: event, To: models.WorkflowStatusCanceled}, nil
		}
	case models.WorkflowStatusFailed, models.WorkflowStatusCanceled:
		if event == TransitionReplay {
			return Transition{From: current, Event: event, To: models.WorkflowStatusPending}, nil
		}
	}
	return Transition{}, fmt.Errorf("illegal workflow transition: %s --%s-->", current, event)
}

func MustNextStatus(current string, event TransitionEvent) string {
	transition, err := NextStatus(current, event)
	if err != nil {
		panic(err)
	}
	return transition.To
}

func IsTerminal(status string) bool {
	status, ok := canonicalWorkflowStatus(status)
	if !ok {
		return false
	}
	switch status {
	case models.WorkflowStatusSucceeded, models.WorkflowStatusFailed, models.WorkflowStatusCanceled:
		return true
	default:
		return false
	}
}

func canonicalWorkflowStatus(status string) (string, bool) {
	switch status {
	case models.WorkflowStatusPending,
		models.WorkflowStatusRunning,
		models.WorkflowStatusSucceeded,
		models.WorkflowStatusFailed,
		models.WorkflowStatusCanceled:
		return status, true
	default:
		return status, false
	}
}
