package config

import (
	"testing"

	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/conductorone/baton-sdk/pkg/test"
	"github.com/conductorone/baton-sdk/pkg/ustrings"
)

func TestConfigs(t *testing.T) {
	test.ExerciseTestCasesFromExpressions(
		t,
		field.NewConfiguration(ConfigurationFields),
		nil,
		ustrings.ParseFlags,
		[]test.TestCaseFromExpression{
			{
				"",
				false,
				"empty",
			},
			{
				"--config-path ./examples/wordpress.yml",
				true,
				"all",
			},
		},
	)
}
