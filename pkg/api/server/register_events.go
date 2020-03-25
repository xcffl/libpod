package server

import (
	"net/http"

	"github.com/containers/libpod/pkg/api/handlers/compat"
	"github.com/gorilla/mux"
)

func (s *APIServer) registerEventsHandlers(r *mux.Router) error {
	// swagger:operation GET /events system getEvents
	// ---
	// tags:
	//   - system (compat)
	// summary: Get events
	// description: Returns events filtered on query parameters
	// produces:
	// - application/json
	// parameters:
	// - name: since
	//   type: string
	//   in: query
	//   description: start streaming events from this time
	// - name: until
	//   type: string
	//   in: query
	//   description: stop streaming events later than this
	// - name: filters
	//   type: string
	//   in: query
	//   description: JSON encoded map[string][]string of constraints
	// responses:
	//   200:
	//     description: returns a string of json data describing an event
	//   500:
	//     "$ref": "#/responses/InternalError"
	r.Handle(VersionedPath("/events"), s.APIHandler(compat.GetEvents)).Methods(http.MethodGet)
	// Added non version path to URI to support docker non versioned paths
	r.Handle("/events", s.APIHandler(compat.GetEvents)).Methods(http.MethodGet)
	// swagger:operation GET /libpod/events system libpodGetEvents
	// ---
	// tags:
	//   - system
	// summary: Get events
	// description: Returns events filtered on query parameters
	// produces:
	// - application/json
	// parameters:
	// - name: since
	//   type: string
	//   in: query
	//   description: start streaming events from this time
	// - name: until
	//   type: string
	//   in: query
	//   description: stop streaming events later than this
	// - name: filters
	//   type: string
	//   in: query
	//   description: JSON encoded map[string][]string of constraints
	// responses:
	//   200:
	//     description: returns a string of json data describing an event
	//   500:
	//     "$ref": "#/responses/InternalError"
	r.Handle(VersionedPath("/libpod/events"), s.APIHandler(compat.GetEvents)).Methods(http.MethodGet)
	return nil
}
