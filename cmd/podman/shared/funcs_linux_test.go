package shared

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCommand(t *testing.T) {
	inputCommand := "docker run -it --name NAME -e NAME=NAME -e IMAGE=IMAGE IMAGE echo \"hello world\""
	correctCommand := "/proc/self/exe run -it --name bar -e NAME=bar -e IMAGE=foo foo echo hello world"
	newCommand, err := GenerateCommand(inputCommand, "foo", "bar", "")
	assert.Nil(t, err)
	assert.Equal(t, "hello world", newCommand[11])
	assert.Equal(t, correctCommand, strings.Join(newCommand, " "))
}

func TestGenerateCommandCheckSubstitution(t *testing.T) {
	type subsTest struct {
		input      string
		expected   string
		shouldFail bool
	}

	absTmpFile, err := ioutil.TempFile("", "podmanRunlabelTestAbsolutePath")
	assert.Nil(t, err, "error creating tempfile")
	defer os.Remove(absTmpFile.Name())

	relTmpFile, err := ioutil.TempFile("./", "podmanRunlabelTestRelativePath")
	assert.Nil(t, err, "error creating tempfile")
	defer os.Remove(relTmpFile.Name())
	relTmpCmd, err := filepath.Abs(relTmpFile.Name())
	assert.Nil(t, err, "error getting absolute path for relative tmpfile")

	// this has a (low) potential of race conditions but no other way
	removedTmpFile, err := ioutil.TempFile("", "podmanRunlabelTestRemove")
	assert.Nil(t, err, "error creating tempfile")
	os.Remove(removedTmpFile.Name())

	absTmpCmd := fmt.Sprintf("%s --flag1 --flag2 --args=foo", absTmpFile.Name())
	tests := []subsTest{
		{
			input:      "docker run -it alpine:latest",
			expected:   "/proc/self/exe run -it alpine:latest",
			shouldFail: false,
		},
		{
			input:      "podman run -it alpine:latest",
			expected:   "/proc/self/exe run -it alpine:latest",
			shouldFail: false,
		},
		{
			input:      absTmpCmd,
			expected:   absTmpCmd,
			shouldFail: false,
		},
		{
			input:      "./" + relTmpFile.Name(),
			expected:   relTmpCmd,
			shouldFail: false,
		},
		{
			input:      "ls -la",
			expected:   "ls -la",
			shouldFail: false,
		},
		{
			input:      removedTmpFile.Name(),
			expected:   "",
			shouldFail: true,
		},
	}

	for _, test := range tests {
		newCommand, err := GenerateCommand(test.input, "foo", "bar", "")
		if test.shouldFail {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
		assert.Equal(t, test.expected, strings.Join(newCommand, " "))
	}
}

func TestGenerateCommandPath(t *testing.T) {
	inputCommand := "docker run -it --name NAME -e NAME=NAME -e IMAGE=IMAGE IMAGE echo install"
	correctCommand := "/proc/self/exe run -it --name bar -e NAME=bar -e IMAGE=foo foo echo install"
	newCommand, _ := GenerateCommand(inputCommand, "foo", "bar", "")
	assert.Equal(t, correctCommand, strings.Join(newCommand, " "))
}

func TestGenerateCommandNoSetName(t *testing.T) {
	inputCommand := "docker run -it --name NAME -e NAME=NAME -e IMAGE=IMAGE IMAGE echo install"
	correctCommand := "/proc/self/exe run -it --name foo -e NAME=foo -e IMAGE=foo foo echo install"
	newCommand, err := GenerateCommand(inputCommand, "foo", "", "")
	assert.Nil(t, err)
	assert.Equal(t, correctCommand, strings.Join(newCommand, " "))
}

func TestGenerateCommandNoName(t *testing.T) {
	inputCommand := "docker run -it -e IMAGE=IMAGE IMAGE echo install"
	correctCommand := "/proc/self/exe run -it -e IMAGE=foo foo echo install"
	newCommand, err := GenerateCommand(inputCommand, "foo", "", "")
	assert.Nil(t, err)
	assert.Equal(t, correctCommand, strings.Join(newCommand, " "))
}

func TestGenerateCommandAlreadyPodman(t *testing.T) {
	inputCommand := "podman run -it --name NAME -e NAME=NAME -e IMAGE=IMAGE IMAGE echo install"
	correctCommand := "/proc/self/exe run -it --name bar -e NAME=bar -e IMAGE=foo foo echo install"
	newCommand, err := GenerateCommand(inputCommand, "foo", "bar", "")
	assert.Nil(t, err)
	assert.Equal(t, correctCommand, strings.Join(newCommand, " "))
}
