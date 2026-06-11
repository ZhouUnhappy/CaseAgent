package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseTrustedOperatorDefaults(t *testing.T) {
	c := &gin.Context{Request: &http.Request{Header: http.Header{}}}

	operator := parseTrustedOperator(c)

	if operator.ID != "local-demo" || operator.Name != "local-demo" {
		t.Fatalf("operator = %#v, want local-demo defaults", operator)
	}
}

func TestParseTrustedOperatorHeaders(t *testing.T) {
	header := http.Header{}
	header.Set(operatorIDHeader, "qa-1")
	header.Set(operatorNameHeader, "QA Lead")
	c := &gin.Context{Request: &http.Request{Header: header}}

	operator := parseTrustedOperator(c)

	if operator.ID != "qa-1" || operator.Name != "QA Lead" {
		t.Fatalf("operator = %#v, want header values", operator)
	}
}

func TestTrustedOperatorCancelMessage(t *testing.T) {
	operator := trustedOperator{Name: "QA Lead", Reason: "bad payload"}

	got := operator.CancelMessage()
	want := "canceled by QA Lead: bad payload"
	if got != want {
		t.Fatalf("CancelMessage() = %q, want %q", got, want)
	}
}

func TestTrustedOperatorMetadata(t *testing.T) {
	operator := trustedOperator{ID: "qa-1", Name: "QA Lead", Reason: "rerun after fix"}

	metadata := operator.Metadata()

	if metadata["operator_id"] != "qa-1" || metadata["operator_name"] != "QA Lead" || metadata["reason"] != "rerun after fix" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
