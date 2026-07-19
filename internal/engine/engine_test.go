package engine

import (
	"testing"

	"github.com/amcchord/WallpaperIdentity/v4/internal/loginscreen"
)

func TestEffectivePolicyRequiresEditionAppropriateProof(t *testing.T) {
	tests := []struct {
		name                                      string
		native, professional, compatibility, want bool
		apply                                     loginscreen.ApplyResult
	}{
		{name: "enterprise group policy", native: true, want: true, apply: loginscreen.ApplyResult{GroupPolicyApplied: true}},
		{name: "enterprise verified MDM", native: true, want: true, apply: loginscreen.ApplyResult{MDMBridgeApplied: true}},
		{name: "pro registry write is not enough", professional: true, compatibility: true, apply: loginscreen.ApplyResult{GroupPolicyApplied: true}},
		{name: "pro SetEdu without verified image", professional: true, compatibility: true, apply: loginscreen.ApplyResult{ProCompatibilityApplied: true}},
		{name: "pro verified compatibility path", professional: true, compatibility: true, want: true, apply: loginscreen.ApplyResult{ProCompatibilityApplied: true, MDMBridgeApplied: true}},
		{name: "pro opt out", professional: true, apply: loginscreen.ApplyResult{ProCompatibilityApplied: true, MDMBridgeApplied: true}},
		{name: "home remains unsupported", apply: loginscreen.ApplyResult{GroupPolicyApplied: true, MDMBridgeApplied: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectivePolicyApplied(tc.native, tc.professional, tc.compatibility, tc.apply); got != tc.want {
				t.Fatalf("effectivePolicyApplied() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPreLoginRefreshWaitsForBootSettledNetworkState(t *testing.T) {
	if shouldRefreshPreLogin("service-start", true) {
		t.Fatal("service-start can run before DHCP replaces a previous lease")
	}
	if !shouldRefreshPreLogin("boot-settled", true) {
		t.Fatal("boot-settled should refresh the empty login session")
	}
	if shouldRefreshPreLogin("boot-settled", false) {
		t.Fatal("explicit boot login-screen refresh opt-out was ignored")
	}
}
