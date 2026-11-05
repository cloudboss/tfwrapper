package root

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

type Module struct {
	Directory   string
	Files       map[string]*hcl.File
	Constraints version.Constraints
}

func Load(dir string) (*Module, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read root module %s: %w", dir, err)
	}
	var primary, overrides []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tf.json") {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimSuffix(name, ".json"), ".tf")
		if stem == "override" || strings.HasSuffix(stem, "_override") {
			overrides = append(overrides, name)
		} else {
			primary = append(primary, name)
		}
	}
	parser := hclparse.NewParser()
	module := &Module{Directory: dir}
	found := false
	for i, name := range append(primary, overrides...) {
		path := filepath.Join(dir, name)
		var file *hcl.File
		var diagnostics hcl.Diagnostics
		if strings.HasSuffix(name, ".json") {
			file, diagnostics = parser.ParseJSONFile(path)
		} else {
			file, diagnostics = parser.ParseHCLFile(path)
		}
		if diagnostics.HasErrors() {
			return nil, diagnostics
		}
		constraints, hasBlock, err := fileRequirements(file)
		if err != nil {
			return nil, err
		}
		found = found || hasBlock
		if i >= len(primary) && len(constraints) > 0 {
			module.Constraints = nil
		}
		module.Constraints = append(module.Constraints, constraints...)
	}
	if !found {
		return nil, fmt.Errorf("root module %s must contain a terraform block", dir)
	}
	if len(module.Constraints) == 0 {
		return nil, fmt.Errorf("root module %s must set terraform.required_version", dir)
	}
	module.Files = parser.Files()
	return module, nil
}

func Directory(cwd string, args []string) (string, error) {
	dir := cwd
	found := false
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			break
		}
		if arg == "-chdir" || arg == "-chdir=" {
			return "", fmt.Errorf("use -chdir=path before the Terraform command")
		}
		value, ok := strings.CutPrefix(arg, "-chdir=")
		if !ok {
			continue
		}
		if found {
			return "", fmt.Errorf("specify -chdir only once")
		}
		found = true
		if filepath.IsAbs(value) {
			dir = value
		} else {
			dir = filepath.Join(cwd, value)
		}
	}
	return filepath.Clean(dir), nil
}

func fileRequirements(file *hcl.File) (version.Constraints, bool, error) {
	content, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}},
	})
	if diagnostics.HasErrors() {
		return nil, false, diagnostics
	}
	var constraints version.Constraints
	for _, block := range content.Blocks {
		body, _, diagnostics := block.Body.PartialContent(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{{Name: "required_version"}},
		})
		if diagnostics.HasErrors() {
			return nil, false, diagnostics
		}
		attr, ok := body.Attributes["required_version"]
		if !ok {
			continue
		}
		value, diagnostics := attr.Expr.Value(nil)
		if diagnostics.HasErrors() {
			return nil, false, diagnostics
		}
		if value.IsNull() || value.Type() != cty.String || strings.TrimSpace(value.AsString()) == "" {
			return nil, false, fmt.Errorf(
				"%s: required_version must be a nonempty literal string", attr.Range,
			)
		}
		required, err := version.NewConstraint(value.AsString())
		if err != nil {
			return nil, false, fmt.Errorf("%s: invalid required_version: %w", attr.Range, err)
		}
		constraints = append(constraints, required...)
	}
	return constraints, len(content.Blocks) > 0, nil
}
