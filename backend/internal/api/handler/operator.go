package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	operatorIDHeader   = "X-Operator-ID"
	operatorNameHeader = "X-Operator-Name"
)

type trustedOperator struct {
	ID     string `json:"operator_id"`
	Name   string `json:"operator_name"`
	Reason string `json:"reason"`
}

type interventionRequest struct {
	Reason string `json:"reason"`
}

func parseTrustedOperator(c *gin.Context) trustedOperator {
	id := strings.TrimSpace(c.GetHeader(operatorIDHeader))
	name := strings.TrimSpace(c.GetHeader(operatorNameHeader))
	if id == "" {
		id = "local-demo"
	}
	if name == "" {
		name = id
	}
	return trustedOperator{ID: id, Name: name}
}

func parseInterventionRequest(c *gin.Context) (trustedOperator, bool) {
	operator := parseTrustedOperator(c)
	var req interventionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return operator, false
	}
	operator.Reason = strings.TrimSpace(req.Reason)
	if operator.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return operator, false
	}
	return operator, true
}

func (o trustedOperator) Metadata() map[string]any {
	return map[string]any{
		"operator_id":   o.ID,
		"operator_name": o.Name,
		"reason":        o.Reason,
	}
}

func (o trustedOperator) CancelMessage() string {
	if o.Name == "" {
		return "canceled by operator"
	}
	if o.Reason == "" {
		return fmt.Sprintf("canceled by %s", o.Name)
	}
	return fmt.Sprintf("canceled by %s: %s", o.Name, o.Reason)
}
