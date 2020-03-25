package entities

import (
	"context"
)

type ContainerEngine interface {
	ContainerExists(ctx context.Context, nameOrId string) (*BoolReport, error)
	ContainerKill(ctx context.Context, namesOrIds []string, options KillOptions) ([]*KillReport, error)
	ContainerPause(ctx context.Context, namesOrIds []string, options PauseUnPauseOptions) ([]*PauseUnpauseReport, error)
	ContainerRestart(ctx context.Context, namesOrIds []string, options RestartOptions) ([]*RestartReport, error)
	ContainerRm(ctx context.Context, namesOrIds []string, options RmOptions) ([]*RmReport, error)
	ContainerUnpause(ctx context.Context, namesOrIds []string, options PauseUnPauseOptions) ([]*PauseUnpauseReport, error)
	ContainerStop(ctx context.Context, namesOrIds []string, options StopOptions) ([]*StopReport, error)
	ContainerWait(ctx context.Context, namesOrIds []string, options WaitOptions) ([]WaitReport, error)
	PodExists(ctx context.Context, nameOrId string) (*BoolReport, error)
	VolumeCreate(ctx context.Context, opts VolumeCreateOptions) (*IdOrNameResponse, error)
	VolumeInspect(ctx context.Context, namesOrIds []string, opts VolumeInspectOptions) ([]*VolumeInspectReport, error)
	VolumeRm(ctx context.Context, namesOrIds []string, opts VolumeRmOptions) ([]*VolumeRmReport, error)
	VolumePrune(ctx context.Context, opts VolumePruneOptions) ([]*VolumePruneReport, error)
	VolumeList(ctx context.Context, opts VolumeListOptions) ([]*VolumeListReport, error)
}
