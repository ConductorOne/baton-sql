package cel

import (
	"github.com/google/cel-go/cel"

	"github.com/conductorone/baton-sql/pkg/cel/functions"
)

type batonCel struct{}

func (batonCel) Variables() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Variable("input", cel.StringType),
	}
}

func (batonCel) CompileOptions() []cel.EnvOption {
	var opts []cel.EnvOption

	opts = append(opts, batonCel{}.Variables()...)
	opts = append(opts, functions.GetAllOptions()...)

	return opts
}

func (batonCel) ProgramOptions() []cel.ProgramOption {
	return []cel.ProgramOption{}
}
