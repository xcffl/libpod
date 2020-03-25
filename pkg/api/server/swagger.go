package server

import (
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/pkg/api/handlers/utils"
	"github.com/containers/libpod/pkg/domain/entities"
)

// No such image
// swagger:response NoSuchImage
type swagErrNoSuchImage struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such container
// swagger:response NoSuchContainer
type swagErrNoSuchContainer struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such exec instance
// swagger:response NoSuchExecInstance
type swagErrNoSuchExecInstance struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such volume
// swagger:response NoSuchVolume
type swagErrNoSuchVolume struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such pod
// swagger:response NoSuchPod
type swagErrNoSuchPod struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such manifest
// swagger:response NoSuchManifest
type swagErrNoSuchManifest struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Internal server error
// swagger:response InternalError
type swagInternalError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Conflict error in operation
// swagger:response ConflictError
type swagConflictError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Bad parameter in request
// swagger:response BadParamError
type swagBadParamError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Container already started
// swagger:response ContainerAlreadyStartedError
type swagContainerAlreadyStartedError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Container already stopped
// swagger:response ContainerAlreadyStoppedError
type swagContainerAlreadyStopped struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Pod already started
// swagger:response PodAlreadyStartedError
type swagPodAlreadyStartedError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Pod already stopped
// swagger:response PodAlreadyStoppedError
type swagPodAlreadyStopped struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Image summary
// swagger:response DockerImageSummary
type swagImageSummary struct {
	// in:body
	Body []entities.ImageSummary
}

// List Containers
// swagger:response DocsListContainer
type swagListContainers struct {
	// in:body
	Body struct {
		// This causes go-swagger to crash
		// handlers.Container
	}
}

// Success
// swagger:response
type ok struct {
	// in:body
	Body struct {
		// example: OK
		ok string
	}
}

// Volume prune response
// swagger:response VolumePruneResponse
type swagVolumePruneResponse struct {
	// in:body
	Body []entities.VolumePruneReport
}

// Volume create response
// swagger:response VolumeCreateResponse
type swagVolumeCreateResponse struct {
	// in:body
	Body struct {
		entities.VolumeConfigResponse
	}
}

// Volume list
// swagger:response VolumeList
type swagVolumeListResponse struct {
	// in:body
	Body []libpod.Volume
}

// Healthcheck
// swagger:response HealthcheckRun
type swagHealthCheckRunResponse struct {
	// in:body
	Body struct {
		libpod.HealthCheckResults
	}
}
