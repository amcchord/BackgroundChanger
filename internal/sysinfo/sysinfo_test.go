package sysinfo

import "testing"

func TestSupportsMachineLockScreenPolicy(t *testing.T) {
	tests := map[string]bool{
		"Enterprise":     true,
		"EnterpriseS":    true,
		"Education":      true,
		"IoTEnterprise":  true,
		"ServerStandard": true,
		"Professional":   false,
		"Core":           false,
	}
	for edition, want := range tests {
		if got := SupportsMachineLockScreenPolicy(edition); got != want {
			t.Errorf("SupportsMachineLockScreenPolicy(%q) = %v, want %v", edition, got, want)
		}
	}
}

func TestIsProfessionalEdition(t *testing.T) {
	tests := map[string]bool{
		"Professional":            true,
		"ProfessionalN":           true,
		"ProfessionalWorkstation": true,
		"ProfessionalEducation":   false,
		"Enterprise":              false,
		"Core":                    false,
	}
	for edition, want := range tests {
		if got := IsProfessionalEdition(edition); got != want {
			t.Errorf("IsProfessionalEdition(%q) = %v, want %v", edition, got, want)
		}
	}
}

func TestCompact(t *testing.T) {
	if got := compact("  a   b  ", 10); got != "a b" {
		t.Fatalf("compact whitespace = %q", got)
	}
	if got := compact("abcdefgh", 5); got != "abcd…" {
		t.Fatalf("compact length = %q", got)
	}
}
