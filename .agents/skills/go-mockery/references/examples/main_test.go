// THIS IS ONLY A EXTRACT OF CODE. USE IT ONLY AS EXAMPLE
package main_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/KaribuLab/kli/git"
	mgit "github.com/KaribuLab/kli/mocks/github.com/KaribuLab/kli/git"
	"github.com/KaribuLab/kli/semver"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSimpleFeature(t *testing.T) {
	assert := assert.New(t)
	cmd := mgit.NewMockCmd(t)
	cmd.
		EXPECT().
		GetLogs(mock.AnythingOfType("bool")).
		Return([]git.GitLog{
			{
				Commit:  "123",
				Author:  "John Doe",
				Message: "feat: new feature",
			},
		}, nil)
	semverCmd := semver.NewSemverCommand(cmd)
	output, err := runAndGetOutput(semverCmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(output)
	assert.Contains(output, "v0.1.0")
	assert.Nil(err)
}

func runAndGetOutput(cmd *cobra.Command) (string, error) {
	oldStdErr := os.Stderr
	oldStdOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stderr = w
	os.Stdout = w
	err = cmd.Execute()
	if err != nil {
		return "", err
	}
	w.Close()
	os.Stderr = oldStdErr
	os.Stdout = oldStdOut
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
