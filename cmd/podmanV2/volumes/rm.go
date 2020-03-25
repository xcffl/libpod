package volumes

import (
	"context"
	"fmt"

	"github.com/containers/libpod/cmd/podmanV2/registry"
	"github.com/containers/libpod/cmd/podmanV2/utils"
	"github.com/containers/libpod/pkg/domain/entities"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	volumeRmDescription = `Remove one or more existing volumes.

  By default only volumes that are not being used by any containers will be removed. To remove the volumes anyways, use the --force flag.`
	rmCommand = &cobra.Command{
		Use:     "rm [flags] VOLUME [VOLUME...]",
		Aliases: []string{"remove"},
		Short:   "Remove one or more volumes",
		Long:    volumeRmDescription,
		RunE:    rm,
		Example: `podman volume rm myvol1 myvol2
  podman volume rm --all
  podman volume rm --force myvol`,
	}
)

var (
	rmOptions = entities.VolumeRmOptions{}
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode, entities.TunnelMode},
		Command: rmCommand,
		Parent:  volumeCmd,
	})
	flags := rmCommand.Flags()
	flags.BoolVarP(&rmOptions.All, "all", "a", false, "Remove all volumes")
	flags.BoolVarP(&rmOptions.Force, "force", "f", false, "Remove a volume by force, even if it is being used by a container")
}

func rm(cmd *cobra.Command, args []string) error {
	var (
		errs utils.OutputErrors
	)
	if (len(args) > 0 && rmOptions.All) || (len(args) < 1 && !rmOptions.All) {
		return errors.New("choose either one or more volumes or all")
	}
	responses, err := registry.ContainerEngine().VolumeRm(context.Background(), args, rmOptions)
	if err != nil {
		return err
	}
	for _, r := range responses {
		if r.Err == nil {
			fmt.Println(r.Id)
		} else {
			errs = append(errs, r.Err)
		}
	}
	return errs.PrintErrors()
}
