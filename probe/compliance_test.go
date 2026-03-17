package probe

import (
	"testing"
)

func TestJurisdictionFail_WhenAbsent(t *testing.T) {
	result := checkJurisdiction("")
	if result.Severity != SeverityFail {
		t.Errorf("expected FAIL, got %s", result.Severity)
	}
	if result.ID != "compliance.jurisdiction.declared" {
		t.Errorf("unexpected ID: %s", result.ID)
	}
}

func TestJurisdictionPass_WhenPresent(t *testing.T) {
	result := checkJurisdiction("ZA")
	if result.Severity != SeverityPass {
		t.Errorf("expected PASS, got %s", result.Severity)
	}
	if result.Value != "ZA" {
		t.Errorf("expected value ZA, got %v", result.Value)
	}
	if result.Metadata["declared_by"] != "operator_flag" {
		t.Errorf("expected declared_by=operator_flag, got %v", result.Metadata["declared_by"])
	}
}

func TestJurisdictionMessage_ContainsNotVerifiedText(t *testing.T) {
	result := checkJurisdiction("DE")
	expected := "Jurisdiction declared by operator. Value not independently verified."
	if result.Message != expected {
		t.Errorf("message = %q, want %q", result.Message, expected)
	}
}

func TestJurisdictionNormalizesCase(t *testing.T) {
	result := checkJurisdiction("za")
	if result.Value != "ZA" {
		t.Errorf("expected uppercase ZA, got %v", result.Value)
	}
}
