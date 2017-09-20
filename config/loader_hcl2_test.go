package config

import (
	"reflect"
	"testing"

	hcl2 "github.com/hashicorp/hcl2/hcl"
)

func TestHCL2ConfigurableConfigurable(t *testing.T) {
	var _ configurable = new(hcl2Configurable)
}

func TestHCL2Basic(t *testing.T) {
	loader := globalHCL2Loader
	cbl, _, err := loader.loadFile("test-fixtures/basic-hcl2.tf")
	if err != nil {
		if diags, isDiags := err.(hcl2.Diagnostics); isDiags {
			for _, diag := range diags {
				t.Logf("- %s", diag.Error())
			}
			t.Fatalf("unexpected diagnostics in load")
		} else {
			t.Fatalf("unexpected error in load: %s", err)
		}
	}

	got, err := cbl.Config()
	if err != nil {
		if diags, isDiags := err.(hcl2.Diagnostics); isDiags {
			for _, diag := range diags {
				t.Logf("- %s", diag.Error())
			}
			t.Fatalf("unexpected diagnostics in decode")
		} else {
			t.Fatalf("unexpected error in decode: %s", err)
		}
	}

	want := &Config{}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, want)
	}
}
