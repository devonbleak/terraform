package config

import (
	"strings"

	gohcl2 "github.com/hashicorp/hcl2/gohcl"
	hcl2 "github.com/hashicorp/hcl2/hcl"
	hcl2parse "github.com/hashicorp/hcl2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// zclConfigurable is an implementation of configurable that knows
// how to turn a zcl Body into a *Config object.
type hcl2Configurable struct {
	SourceFilename string
	Body           hcl2.Body
}

// zclLoader is a wrapper around a zcl parser that provides a fileLoaderFunc.
type hcl2Loader struct {
	Parser *hcl2parse.Parser
}

// For the moment we'll just have a global loader since we don't have anywhere
// better to stash this.
// TODO: refactor the loader API so that it uses some sort of object we can
// stash the parser inside.
var globalHCL2Loader = newHCL2Loader()

// newZclLoader creates a new zclLoader containing a new zcl Parser.
//
// zcl parsers retain information about files that are loaded to aid in
// producing diagnostic messages, so all files within a single configuration
// should be loaded with the same parser to ensure the availability of
// full diagnostic information.
func newHCL2Loader() hcl2Loader {
	return hcl2Loader{
		Parser: hcl2parse.NewParser(),
	}
}

// loadFile is a fileLoaderFunc that knows how to read a HCL2
// files and turn it into a hcl2Configurable.
func (l hcl2Loader) loadFile(filename string) (configurable, []string, error) {
	var f *hcl2.File
	var diags hcl2.Diagnostics
	if strings.HasSuffix(filename, ".json") {
		f, diags = l.Parser.ParseJSONFile(filename)
	} else {
		f, diags = l.Parser.ParseHCLFile(filename)
	}
	if diags.HasErrors() {
		// Return diagnostics as an error; callers may type-assert this to
		// recover the original diagnostics, if it doesn't end up wrapped
		// in another error.
		return nil, nil, diags
	}

	return &hcl2Configurable{
		SourceFilename: filename,
		Body:           f.Body,
	}, nil, nil
}

func (t *hcl2Configurable) Config() (*Config, error) {
	config := &Config{}

	// these structs are used only for the initial shallow decoding; we'll
	// expand this into the main, public-facing config structs afterwards.
	type atlas struct {
		Name    string    `hcl:"name"`
		Include *[]string `hcl:"include"`
		Exclude *[]string `hcl:"exclude"`
	}
	type module struct {
		Name   string    `hcl:"name,label"`
		Source string    `hcl:"source,attr"`
		Config hcl2.Body `hcl:",remain"`
	}
	type provider struct {
		Name    string    `hcl:"name,label"`
		Alias   *string   `hcl:"alias,attr"`
		Version *string   `hcl:"version,attr"`
		Config  hcl2.Body `hcl:",remain"`
	}
	type resourceLifecycle struct {
		CreateBeforeDestroy *bool     `hcl:"create_before_destroy,attr"`
		PreventDestroy      *bool     `hcl:"prevent_destroy,attr"`
		IgnoreChanges       *[]string `hcl:"ignore_changes,attr"`
	}
	type connection struct {
		Config hcl2.Body `hcl:",remain"`
	}
	type provisioner struct {
		Type string `hcl:"type,label"`

		When      *string `hcl:"when,attr"`
		OnFailure *string `hcl:"on_failure,attr"`

		Connection *connection `hcl:"connection,block"`
		Config     hcl2.Body   `hcl:",remain"`
	}
	type resource struct {
		Type string `hcl:"type,label"`
		Name string `hcl:"name,label"`

		CountExpr hcl2.Expression `hcl:"count,attr"`
		Provider  *string         `hcl:"provider,attr"`
		DependsOn *[]string       `hcl:"depends_on,attr"`

		Lifecycle    *resourceLifecycle `hcl:"lifecycle,block"`
		Provisioners []provisioner      `hcl:"provisioner,block"`

		Config hcl2.Body `hcl:",remain"`
	}
	type variable struct {
		Name string `hcl:"name,label"`

		DeclaredType *string    `hcl:"type,attr"`
		Default      *cty.Value `hcl:"default,attr"`
		Description  *string    `hcl:"description,attr"`
		Sensitive    *bool      `hcl:"sensitive,attr"`
	}
	type output struct {
		Name string `hcl:"name,label"`

		Value       hcl2.Expression `hcl:"value,attr"`
		DependsOn   *[]string       `hcl:"depends_on,attr"`
		Description *string         `hcl:"description,attr"`
		Sensitive   *bool           `hcl:"sensitive,attr"`
	}
	type locals struct {
		Definitions hcl2.Attributes `hcl:",remain"`
	}
	type backend struct {
		Type   string    `hcl:"type,label"`
		Config hcl2.Body `hcl:",remain"`
	}
	type terraform struct {
		RequiredVersion *string  `hcl:"required_version,attr"`
		Backend         *backend `hcl:"backend,block"`
	}
	type topLevel struct {
		Atlas     *atlas     `hcl:"atlas,block"`
		Datas     []resource `hcl:"data,block"`
		Modules   []module   `hcl:"module,block"`
		Outputs   []output   `hcl:"output,block"`
		Providers []provider `hcl:"provider,block"`
		Resources []resource `hcl:"resource,block"`
		Terraform *terraform `hcl:"terraform,block"`
		Variables []variable `hcl:"variable,block"`
		Locals    []*locals  `hcl:"locals,block"`
	}

	var raw topLevel
	diags := gohcl2.DecodeBody(t.Body, nil, &raw)
	if diags.HasErrors() {
		// Do some minimal decoding to see if we can at least get the
		// required Terraform version, which might help explain why we
		// couldn't parse the rest.
		if raw.Terraform != nil && raw.Terraform.RequiredVersion != nil {
			config.Terraform = &Terraform{
				RequiredVersion: *raw.Terraform.RequiredVersion,
			}
		}

		// We return the diags as an implementation of error, which the
		// caller than then type-assert if desired to recover the individual
		// diagnostics.
		// FIXME: The current API gives us no way to return warnings in the
		// absense of any errors.
		return config, diags
	}

	for _, rawV := range raw.Variables {
		v := &Variable{
			Name: rawV.Name,
		}
		if rawV.DeclaredType != nil {
			v.DeclaredType = *rawV.DeclaredType
		}
		if rawV.Default != nil {
			// TODO: decode this to a raw interface like the rest of Terraform
			// is expecting, using some shared "turn cty value into what
			// Terraform expects" function.
		}
		if rawV.Description != nil {
			v.Description = *rawV.Description
		}

		config.Variables = append(config.Variables, v)
	}

	for _, rawR := range raw.Resources {
		r := &Resource{
			Mode: ManagedResourceMode,
			Type: rawR.Type,
			Name: rawR.Name,
		}
		if rawR.Lifecycle != nil {
			l := &ResourceLifecycle{}
			if rawR.Lifecycle.CreateBeforeDestroy != nil {
				l.CreateBeforeDestroy = *rawR.Lifecycle.CreateBeforeDestroy
			}
			if rawR.Lifecycle.PreventDestroy != nil {
				l.PreventDestroy = *rawR.Lifecycle.PreventDestroy
			}
			if rawR.Lifecycle.IgnoreChanges != nil {
				l.IgnoreChanges = *rawR.Lifecycle.IgnoreChanges
			}
		}

		// TODO: provider, provisioners, depends_on, count, and the config itself

		config.Resources = append(config.Resources, r)

	}

	return config, nil
}
