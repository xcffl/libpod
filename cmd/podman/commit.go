package main

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/pkg/adapter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	commitCommand     cliconfig.CommitValues
	commitDescription = `Create an image from a container's changes. Optionally tag the image created, set the author with the --author flag, set the commit message with the --message flag, and make changes to the instructions with the --change flag.`

	_commitCommand = &cobra.Command{
		Use:   "commit [flags] CONTAINER [IMAGE]",
		Short: "Create new image based on the changed container",
		Long:  commitDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			commitCommand.InputArgs = args
			commitCommand.GlobalFlags = MainGlobalOpts
			commitCommand.Remote = remoteclient
			return commitCmd(&commitCommand)
		},
		Example: `podman commit -q --message "committing container to image" reverent_golick image-committed
  podman commit -q --author "firstName lastName" reverent_golick image-committed
  podman commit -q --pause=false containerID image-committed
  podman commit containerID`,
	}

	// ChangeCmds is the list of valid Changes commands to passed to the Commit call
	ChangeCmds = []string{"CMD", "ENTRYPOINT", "ENV", "EXPOSE", "LABEL", "ONBUILD", "STOPSIGNAL", "USER", "VOLUME", "WORKDIR"}
)

func init() {
	commitCommand.Command = _commitCommand
	commitCommand.SetHelpTemplate(HelpTemplate())
	commitCommand.SetUsageTemplate(UsageTemplate())
	flags := commitCommand.Flags()
	flags.StringArrayVarP(&commitCommand.Change, "change", "c", []string{}, fmt.Sprintf("Apply the following possible instructions to the created image (default []): %s", strings.Join(ChangeCmds, " | ")))
	flags.StringVarP(&commitCommand.Format, "format", "f", "oci", "`Format` of the image manifest and metadata")
	flags.StringVarP(&commitCommand.ImageIDFile, "iidfile", "", "", "`file` to write the image ID to")
	flags.StringVarP(&commitCommand.Message, "message", "m", "", "Set commit message for imported image")
	flags.StringVarP(&commitCommand.Author, "author", "a", "", "Set the author for the image committed")
	flags.BoolVarP(&commitCommand.Pause, "pause", "p", false, "Pause container during commit")
	flags.BoolVarP(&commitCommand.Quiet, "quiet", "q", false, "Suppress output")
	flags.BoolVar(&commitCommand.IncludeVolumes, "include-volumes", false, "Include container volumes as image volumes")
}

func commitCmd(c *cliconfig.CommitValues) error {
	runtime, err := adapter.GetRuntime(getContext(), &c.PodmanCommand)
	if err != nil {
		return errors.Wrapf(err, "could not get runtime")
	}
	defer runtime.DeferredShutdown(false)

	args := c.InputArgs
	if len(args) < 1 {
		return errors.Errorf("you must provide a container name or ID and optionally a target image name")
	}

	container := args[0]
	reference := ""
	if len(args) > 1 {
		reference = args[1]
	}

	iid, err := runtime.Commit(getContext(), c, container, reference)
	if err != nil {
		return err
	}
	if c.ImageIDFile != "" {
		if err = ioutil.WriteFile(c.ImageIDFile, []byte(iid), 0644); err != nil {
			return errors.Wrapf(err, "failed to write image ID to file %q", c.ImageIDFile)
		}
	}
	fmt.Println(iid)
	return nil
}
