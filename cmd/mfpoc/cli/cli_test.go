package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionCmd_Output(t *testing.T) {
	root := Root()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})
	require.NoError(t, root.Execute())
	require.Contains(t, out.String(), `"version"`)
	require.Contains(t, out.String(), `"commit"`)
}

func TestConfigValidate_FailsOnMissingFile(t *testing.T) {
	root := Root()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"config-validate", "--config", "/nonexistent/mfpoc.yaml"})
	require.Error(t, root.Execute())
}
