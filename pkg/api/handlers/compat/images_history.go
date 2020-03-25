package compat

import (
	"net/http"

	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/pkg/api/handlers"
	"github.com/containers/libpod/pkg/api/handlers/utils"
	"github.com/pkg/errors"
)

func HistoryImage(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	name := utils.GetName(r)
	var allHistory []handlers.HistoryResponse

	newImage, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return

	}
	history, err := newImage.History(r.Context())
	if err != nil {
		utils.InternalServerError(w, err)
		return
	}
	for _, h := range history {
		l := handlers.HistoryResponse{
			ID:        h.ID,
			Created:   h.Created.Unix(),
			CreatedBy: h.CreatedBy,
			Tags:      h.Tags,
			Size:      h.Size,
			Comment:   h.Comment,
		}
		allHistory = append(allHistory, l)
	}
	utils.WriteResponse(w, http.StatusOK, allHistory)
}
